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
	"strconv"
	"strings"
	"time"

	"gitlab.com/wh1t3hat/mrimpact/internal/gitlab"
	"gitlab.com/wh1t3hat/mrimpact/internal/pipeline"
	"gitlab.com/wh1t3hat/mrimpact/internal/report"
	"gitlab.com/wh1t3hat/mrimpact/internal/risk"
)

// newClient builds the GitLab client; tests replace it to inject a no-op Sleep.
var newClient = func(baseURL, token string) *gitlab.Client {
	return &gitlab.Client{BaseURL: baseURL, Token: token, HTTP: http.DefaultClient}
}

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

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
	failOn := fs.String("fail-on", "", `exit 1 when set to "red" and a red function exists`)
	strict := fs.Bool("strict", false, "exit 2 on internal or post failure")
	skipEmpty := fs.Bool("skip-empty", false, "do not post when there are no Go changes")
	noPost := fs.Bool("no-post", false, "shadow mode: never post to GitLab")
	out := fs.String("out", "mrimpact-report.md", "write the note to this file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	usage := func(format string, a ...any) int {
		warnf(format, a...)
		fs.Usage()
		return 2
	}
	switch {
	case fs.NArg() > 0:
		return usage("unexpected argument %q", fs.Arg(0))
	case *base == "":
		return usage("--base is required (or CI_MERGE_REQUEST_DIFF_BASE_SHA)")
	case *failOn != "" && *failOn != "red":
		return usage(`--fail-on must be "" or "red", got %q`, *failOn)
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

	cfg := pipeline.Config{
		Dir: *dir, Base: *base, Head: *head, Depth: *depth,
		CoverProfiles: profiles, Timeout: *timeout,
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
	if *failOn == "red" {
		for _, f := range m.Funcs {
			if f.Level == risk.Red {
				return 1
			}
		}
	}
	return 0
}
