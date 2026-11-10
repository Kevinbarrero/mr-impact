// Command mrimpact analyzes a GitLab merge request's changed Go functions and
// posts one risk-ranked note.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Kevinbarrero/mr-impact/internal/arch"
	"github.com/Kevinbarrero/mr-impact/internal/config"
	"github.com/Kevinbarrero/mr-impact/internal/gitlab"
	"github.com/Kevinbarrero/mr-impact/internal/pipeline"
	"github.com/Kevinbarrero/mr-impact/internal/report"
	"github.com/Kevinbarrero/mr-impact/internal/risk"
	"github.com/Kevinbarrero/mr-impact/internal/sensitive"
)

// newClient builds the GitLab client; tests replace it to inject a no-op Sleep.
var newClient = func(baseURL, token string) *gitlab.Client {
	return &gitlab.Client{BaseURL: baseURL, Token: token, HTTP: http.DefaultClient}
}

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// version is set at release time with -X main.version=vX.Y.Z.
var version = "dev"

// resolveVersion returns v unless it is "dev", in which case a module version
// recorded by "go install" is used when there is one.
func resolveVersion(v string, info func() (*debug.BuildInfo, bool)) string {
	if v != "dev" {
		return v
	}
	if bi, ok := info(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return v
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, env func(string) string, stdout, stderr io.Writer) int {
	warnf := func(format string, a ...any) { fmt.Fprintf(stderr, "mrimpact: "+format+"\n", a...) }

	fs := flag.NewFlagSet("mrimpact", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var profiles listFlag
	dir := fs.String("dir", ".", "repository root (module root), checked out at --head")
	base := fs.String("base", env("CI_MERGE_REQUEST_DIFF_BASE_SHA"), "base revision (env CI_MERGE_REQUEST_DIFF_BASE_SHA)")
	head := fs.String("head", "HEAD", "head revision")
	depth := fs.Int("depth", 2, "caller depth (>= 1)")
	fs.Var(&profiles, "coverprofile", "coverage profile glob (repeatable)")
	tags := fs.String("tags", "", "comma-separated build tags")
	timeout := fs.Duration("timeout", 5*time.Minute, "call graph time budget")
	failOn := fs.String("fail-on", "", `comma list of gates that exit 1: "red" (a red function exists), "arch" (a new violation of the declared architecture)`)
	strict := fs.Bool("strict", false, "exit 2 on internal or post failure")
	skipEmpty := fs.Bool("skip-empty", false, "do not post when there are no Go changes")
	showVersion := fs.Bool("version", false, "print the version and exit")
	noPost := fs.Bool("no-post", false, "shadow mode: never post to GitLab")
	out := fs.String("out", "mrimpact-report.md", "write the note to this file")
	configPath := fs.String("config", "", "config file (default: .mrimpact.yml in --dir, optional)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "mrimpact %s\n", resolveVersion(version, debug.ReadBuildInfo))
		return 0
	}
	usage := func(format string, a ...any) int {
		warnf(format, a...)
		fs.Usage()
		return 2
	}
	gates, failErr := parseFailOn(*failOn)
	switch {
	case fs.NArg() > 0:
		return usage("unexpected argument %q", fs.Arg(0))
	case *base == "":
		return usage("--base is required (or CI_MERGE_REQUEST_DIFF_BASE_SHA)")
	case failErr != nil:
		return usage("%v", failErr)
	case *depth < 1:
		return usage("--depth must be >= 1")
	case *timeout <= 0:
		return usage("--timeout must be > 0")
	}

	iid, projectID, apiURL := env("CI_MERGE_REQUEST_IID"), env("CI_PROJECT_ID"), env("CI_API_V4_URL")
	inMR := iid != "" && projectID != "" && apiURL != ""
	mrIID := 0
	if inMR && !*noPost {
		n, err := strconv.Atoi(iid)
		if err != nil || n < 1 {
			return usage("CI_MERGE_REQUEST_IID must be a positive integer, got %q", iid)
		}
		mrIID = n
	}

	// Config errors stop the run before any note is written or posted.
	fileCfg, err := config.Load(*dir, *configPath)
	var table sensitive.Table
	if err == nil {
		table, err = sensitive.NewTable(fileCfg.SensitiveAdd, fileCfg.SensitiveRemove)
	}
	var model arch.Model
	if err == nil {
		model, err = arch.FromConfig(fileCfg.Architecture)
	}
	if err != nil {
		msg := strings.TrimPrefix(err.Error(), config.ErrConfig.Error()+": ")
		warnf("config: %s", strings.Join(strings.Fields(msg), " "))
		return 2
	}

	cfg := pipeline.Config{
		Version: resolveVersion(version, debug.ReadBuildInfo),
		Dir:     *dir, Base: *base, Head: *head, Depth: *depth,
		CoverProfiles: profiles, Timeout: *timeout,
		Sensitive: &table, Critical: fileCfg.Critical,
		File: &fileCfg, FileName: ".mrimpact.yml", FileRel: ".mrimpact.yml",
		Arch: &model,
	}
	if *configPath != "" {
		cfg.FileName, cfg.FileRel = *configPath, repoRel(*dir, *configPath)
	}
	for _, t := range strings.Split(*tags, ",") {
		if t = strings.TrimSpace(t); t != "" {
			cfg.Tags = append(cfg.Tags, t)
		}
	}

	internalFailure := false
	m, err := pipeline.Run(ctx, cfg)
	if err != nil {
		warnf("analysis failed: %v", err)
		internalFailure = true
	}
	note := report.Render(m)

	if err := os.WriteFile(*out, []byte(note), 0o644); err != nil {
		warnf("cannot write %s: %v", *out, err)
		internalFailure = true
	}
	fmt.Fprint(stdout, note)

	if inMR && !*noPost && !(m.NoGoChanges && *skipEmpty) {
		// No token: keep the artifact and stdout, warn, and skip posting; this
		// is a configuration gap, not an internal failure (spec §7).
		if token := env("MRIMPACT_TOKEN"); token == "" {
			warnf("set MRIMPACT_TOKEN (Project Access Token, api scope, Reporter role) as a masked CI/CD variable; note not posted")
		} else if err := newClient(apiURL, token).Upsert(ctx, projectID, mrIID, report.Marker, note); err != nil {
			warnf("posting note failed: %v", err)
			internalFailure = true
		}
	}

	if *strict && internalFailure {
		return 2
	}
	if gates["red"] {
		for _, f := range m.Funcs {
			if f.Level == risk.Red {
				return 1
			}
		}
	}
	// Only a declared architecture gates, and only against a readable base:
	// without a base nothing is "new".
	if gates["arch"] && m.Arch != nil && !m.Arch.Inferred && !m.Arch.BaseMissing && len(m.Arch.New) > 0 {
		return 1
	}
	return 0
}

// parseFailOn splits the --fail-on comma list; items are trimmed, empty ones
// ignored, anything but red or arch is an error naming it.
func parseFailOn(v string) (map[string]bool, error) {
	gates := map[string]bool{}
	for _, it := range strings.Split(v, ",") {
		switch it = strings.TrimSpace(it); it {
		case "":
		case "red", "arch":
			gates[it] = true
		default:
			return nil, fmt.Errorf(`--fail-on: unknown gate %q (want "red" and/or "arch", comma-separated)`, it)
		}
	}
	return gates, nil
}

// repoRel returns p relative to the repository root dir, slash-separated, or
// "" when p lies outside it (it then has no base version to compare).
func repoRel(dir, p string) string {
	absDir, err1 := filepath.Abs(dir)
	absP, err2 := filepath.Abs(p)
	if err1 != nil || err2 != nil {
		return ""
	}
	rel, err := filepath.Rel(absDir, absP)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}
