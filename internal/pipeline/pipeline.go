// Package pipeline wires diff, symbols, load, callgraph, coverage and risk
// into one report.Model. It does no network I/O.
package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/scanner"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/wh1t3hat/mrimpact/internal/callgraph"
	"gitlab.com/wh1t3hat/mrimpact/internal/coverage"
	"gitlab.com/wh1t3hat/mrimpact/internal/diff"
	"gitlab.com/wh1t3hat/mrimpact/internal/load"
	"gitlab.com/wh1t3hat/mrimpact/internal/report"
	"gitlab.com/wh1t3hat/mrimpact/internal/risk"
	"gitlab.com/wh1t3hat/mrimpact/internal/symbols"
)

// Version is shown in the report footer as "mrimpact v<Version>".
const Version = "0.1.0"

const defaultDepth = 2

type Config struct {
	Dir           string           // repo root, checked out at Head; the module root
	Base, Head    string           // Head default "HEAD"
	Depth         int              // caller depth; <= 0 means 2
	CoverProfiles []string         // globs; relative ones are resolved against Dir
	Tags          []string         // build tags for loading
	Timeout       time.Duration    // call graph budget; 0 means none
	Now           func() time.Time // nil → time.Now; e2e pins it so Duration renders "0.0 s"
}

// Run analyzes Base..Head in cfg.Dir. err is non-nil only for internal
// failures (git missing, ErrShallow, no package loaded, I/O errors); in that
// case Model.Failure is also set to a fixed, single-line message (never the
// error text, which may hold git/go stderr with credentialed URLs) so the CLI
// can still post, and err is returned unchanged for the job log. Build errors
// in the changed code are an analysis outcome: Model.Failure is set, err is nil.
func Run(ctx context.Context, cfg Config) (report.Model, error) {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	start := now()
	m, err := run(ctx, cfg)
	if err != nil {
		var le loadError
		if errors.As(err, &le) {
			err = le.err
		}
		m = report.Model{Failure: failureText(err, le.err != nil)}
	}
	m.Version = Version
	m.Duration = now().Sub(start)
	return m, err
}

func run(ctx context.Context, cfg Config) (report.Model, error) {
	head := cfg.Head
	if head == "" {
		head = "HEAD"
	}
	depth := cfg.Depth
	if depth <= 0 {
		depth = defaultDepth
	}

	// 1. Base commit.
	if err := diff.EnsureCommit(ctx, cfg.Dir, cfg.Base); err != nil {
		return report.Model{}, err
	}
	baseSHA, err := revParse(ctx, cfg.Dir, cfg.Base)
	if err != nil {
		return report.Model{}, err
	}
	m := report.Model{BaseSHA: baseSHA, Coverage: "static"}

	// 2. Diff.
	changes, err := diff.Run(ctx, cfg.Dir, cfg.Base, head)
	if err != nil {
		return report.Model{}, err
	}
	if len(changes) == 0 { // diff.Run keeps only non-excluded .go files
		m.NoGoChanges = true
		return m, nil
	}

	// 3. Symbols.
	headSrc := func(p string) ([]byte, error) { return os.ReadFile(filepath.Join(cfg.Dir, filepath.FromSlash(p))) }
	baseSrc := func(p string) ([]byte, error) { return diff.ShowFile(ctx, cfg.Dir, cfg.Base, p) }
	syms, err := symbols.Find(changes, headSrc, baseSrc)
	if err != nil {
		var list scanner.ErrorList
		if errors.As(err, &list) && len(list) > 0 {
			m.Failure = "build errors in `" + filepath.ToSlash(list[0].Pos.Filename) + "`"
			return m, nil
		}
		return report.Model{}, err
	}
	m.TestsTouched = syms.TestsTouched
	for _, d := range syms.Decls {
		m.Decls = append(m.Decls, d.What+" in "+d.File)
	}
	var live []symbols.ChangedFunc
	for _, f := range syms.Funcs {
		if f.Kind == symbols.Deleted {
			m.Deleted = append(m.Deleted, f.Display)
		} else {
			live = append(live, f)
		}
	}

	// 4. Load.
	prog, err := load.Load(ctx, cfg.Dir, cfg.Tags)
	if err != nil {
		return report.Model{}, loadError{err}
	}
	changedFiles := map[string]bool{}
	for _, f := range live {
		changedFiles[f.File] = true
	}
	var failed []string
	for _, b := range prog.Broken {
		if containsAny(b.Files, changedFiles) {
			failed = append(failed, "`"+b.PkgPath+"`")
		} else {
			m.NotAnalyzed = append(m.NotAnalyzed, b.PkgPath+" (load error)")
		}
	}
	if len(failed) > 0 {
		return report.Model{BaseSHA: baseSHA, Failure: "build errors in " + strings.Join(failed, ", ")}, nil
	}
	for _, w := range prog.Warnings {
		m.Warnings = append(m.Warnings, w.PkgPath+": "+w.Msg)
	}

	// 5. Call graph.
	gctx, cancel := ctx, context.CancelFunc(func() {})
	if cfg.Timeout > 0 {
		gctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	}
	graph, err := callgraph.Build(gctx, prog)
	cancel()
	if err != nil {
		if ctx.Err() != nil || !errors.Is(err, context.DeadlineExceeded) {
			return report.Model{}, err
		}
		graph = nil
		m.Degraded = true
		m.Warnings = append(m.Warnings, fmt.Sprintf("Call graph exceeded the %s timeout: callers not analyzed.", cfg.Timeout))
	}

	// 7. Coverage (loaded before rating; step 6 is per function below).
	var prof *coverage.Profile
	if len(cfg.CoverProfiles) > 0 {
		patterns := make([]string, len(cfg.CoverProfiles))
		for i, p := range cfg.CoverProfiles {
			if !filepath.IsAbs(p) {
				p = filepath.Join(cfg.Dir, p)
			}
			patterns[i] = p
		}
		p, warning, err := coverage.Load(patterns, prog.ModulePath, lineCounter(cfg.Dir))
		switch {
		case err != nil:
			warning = "Coverprofile ignored: " + err.Error()
		case warning != "":
			warning = "Coverprofile ignored: " + warning
		default:
			prof = p
			m.Coverage = "coverprofile"
		}
		if warning != "" {
			m.Degraded = true
			m.Warnings = append(m.Warnings, warning)
		}
	}

	// 6 + 8. Impact and risk per function.
	for _, f := range live {
		if !prog.Files[f.File] {
			m.NotAnalyzed = append(m.NotAnalyzed, f.Display+" (build tags)")
			continue
		}
		var im callgraph.Impact
		if graph != nil {
			im = graph.Impact(f.File, f.StartLine, depth)
			if !im.Found {
				m.NotAnalyzed = append(m.NotAnalyzed, f.Display+" (not in call graph)")
				continue
			}
		}
		in := risk.Input{
			Kind: f.Kind, Exported: f.Exported,
			NonTestCallers: len(im.Callers), TestReached: im.TestReached,
		}
		var cover string
		switch {
		case prof != nil:
			in.HasProfile = true
			in.Covered, in.Total = prof.Lines(f.File, f.AddedLines)
			cover = "n/a"
			if in.Total > 0 {
				cover = fmt.Sprintf("%d %% (%d/%d)", in.Covered*100/in.Total, in.Covered, in.Total)
			}
		case graph == nil:
			cover = "n/a"
		case im.TestReached:
			cover = "reached by test"
		default:
			cover = "not reached"
		}
		m.Funcs = append(m.Funcs, report.FuncRow{
			Level: risk.Assess(in), Display: f.Display, Kind: kindText(f),
			Callers: im.Callers, CoverText: cover,
		})
	}
	return m, nil
}

// loadError marks a load.Load failure (nothing loaded) so Run can choose its
// public message; Run unwraps it before returning.
type loadError struct{ err error }

func (e loadError) Error() string { return e.err.Error() }

// failureText is the public, single-line Model.Failure for an internal error.
func failureText(err error, isLoad bool) string {
	switch {
	case errors.Is(err, diff.ErrShallow):
		return diff.ErrShallow.Error()
	case isLoad:
		return "no Go packages could be loaded (see job log)"
	default:
		return "internal error (see job log)"
	}
}

func kindText(f symbols.ChangedFunc) string {
	if f.Kind == symbols.Signature && f.Exported {
		return "signature (exported)"
	}
	return string(f.Kind)
}

func containsAny(files []string, set map[string]bool) bool {
	for _, f := range files {
		if set[f] {
			return true
		}
	}
	return false
}

// lineCounter reports the line count of repo-relative HEAD files under dir.
func lineCounter(dir string) coverage.FileInfo {
	return func(p string) (int, bool) {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return 0, false
		}
		n := bytes.Count(data, []byte("\n"))
		if len(data) > 0 && data[len(data)-1] != '\n' {
			n++
		}
		return n, true
	}
}

func revParse(ctx context.Context, dir, rev string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--verify", rev+"^{commit}")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w: %s", rev, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
