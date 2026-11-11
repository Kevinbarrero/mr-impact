// Package pipeline wires diff, symbols, load, callgraph, coverage and risk
// into one report.Model. It does no network I/O.
package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/scanner"
	"go/types"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Kevinbarrero/mr-impact/internal/arch"
	"github.com/Kevinbarrero/mr-impact/internal/callgraph"
	"github.com/Kevinbarrero/mr-impact/internal/config"
	"github.com/Kevinbarrero/mr-impact/internal/configfields"
	"github.com/Kevinbarrero/mr-impact/internal/coverage"
	"github.com/Kevinbarrero/mr-impact/internal/diff"
	"github.com/Kevinbarrero/mr-impact/internal/load"
	"github.com/Kevinbarrero/mr-impact/internal/registrations"
	"github.com/Kevinbarrero/mr-impact/internal/report"
	"github.com/Kevinbarrero/mr-impact/internal/risk"
	"github.com/Kevinbarrero/mr-impact/internal/sensitive"
	"github.com/Kevinbarrero/mr-impact/internal/symbols"
)

const defaultDepth = 2

type Config struct {
	Version       string           // shown in the report footer as "mrimpact <Version>"
	Dir           string           // repo root, checked out at Head; the module root
	Base, Head    string           // Head default "HEAD"
	Depth         int              // caller depth; <= 0 means 2
	CoverProfiles []string         // globs; relative ones are resolved against Dir
	Tags          []string         // build tags for loading
	Timeout       time.Duration    // call graph budget; 0 means none
	Now           func() time.Time // nil → time.Now; e2e pins it so Duration renders "0.0 s"
	Sensitive     *sensitive.Table // nil → the default table (sensitive.NewTable(nil, nil))
	Critical      []string         // package globs whose functions are sensitive
	// File is the effective config file (read at head), for the note's
	// config line; nil: no line. FileName is how the line names it; FileRel,
	// when set, is its repo-relative path, read at the base to tell what the
	// MR changed.
	File              *config.Config
	FileName, FileRel string
	// Arch is the architecture model the import graph is judged against;
	// nil → arch.Inferred() (advisory hints).
	Arch *arch.Model
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
	m.Version = cfg.Version
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
	// Diff against merge-base(base, head) so changes the target branch made
	// after head branched off are excluded. In CI the base already is the
	// merge-base; there a shallow clone may hide the common ancestor, so with
	// no merge-base in local history the base itself is used.
	switch mb, err := diff.MergeBase(ctx, cfg.Dir, baseSHA, head); {
	case err == nil:
		baseSHA = mb
	case !errors.Is(err, diff.ErrNoMergeBase):
		return report.Model{}, err
	}
	m := report.Model{BaseSHA: baseSHA, Coverage: "static", Config: configNote(ctx, cfg, baseSHA)}

	// 2. Diff.
	changes, err := diff.Run(ctx, cfg.Dir, baseSHA, head)
	if err != nil {
		return report.Model{}, err
	}
	if len(changes) == 0 { // diff.Run keeps only non-excluded .go files
		m.NoGoChanges = true
		return m, nil
	}

	// 3. Symbols.
	headSrc := func(p string) ([]byte, error) { return os.ReadFile(filepath.Join(cfg.Dir, filepath.FromSlash(p))) }
	baseSrc := func(p string) ([]byte, error) { return diff.ShowFile(ctx, cfg.Dir, baseSHA, p) }
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
	m.DeletedFiles = syms.DeletedFiles
	for _, d := range syms.Decls {
		m.Decls = append(m.Decls, report.Decl{What: d.What, File: d.File})
	}
	var live []symbols.ChangedFunc
	for _, f := range syms.Funcs {
		if f.Kind == symbols.Deleted {
			m.Deleted = append(m.Deleted, report.DeletedFunc{Display: f.Display, File: f.File})
		} else {
			live = append(live, f)
		}
	}

	// 4. Load HEAD; the BASE revision is exported and loaded alongside, for
	// comparisons that need both (registrations).
	baseCh := make(chan baseResult, 1)
	bctx, cancelBase := context.WithCancel(ctx)
	deletedLines := map[string]map[int]bool{}
	for _, c := range changes {
		if c.IsTest || c.OldPath == "" || len(c.DeletedLines) == 0 {
			continue
		}
		set := map[int]bool{}
		for _, l := range c.DeletedLines {
			set[l] = true
		}
		deletedLines[c.OldPath] = set
	}
	go func() { baseCh <- loadBase(bctx, cfg, baseSHA, deletedLines) }()
	baseDone := false
	defer func() { // early returns: stop the base load and let it clean up
		cancelBase()
		if !baseDone {
			<-baseCh
		}
	}()
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

	// A critical glob that matches no loaded package is most likely a typo.
	for _, g := range cfg.Critical {
		matched := false
		for _, p := range prog.Packages {
			if _, ok := sensitive.Critical(p.PkgPath, prog.ModulePath, []string{g}); ok {
				matched = true
				break
			}
		}
		if !matched {
			m.Warnings = append(m.Warnings, fmt.Sprintf("critical glob %q matches no package", g))
		}
	}

	table := cfg.Sensitive
	if table == nil {
		t, err := sensitive.NewTable(nil, nil)
		if err != nil {
			return report.Model{}, err
		}
		table = &t
	}

	// 6 + 8. Impact and risk per function.
	var rated []ratedFunc
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
		pkgName, _, _ := strings.Cut(f.Display, ".")
		in := risk.Input{
			Kind: f.Kind, Exported: f.Exported,
			NonTestCallers: len(im.Callers), TestReached: im.TestReached,
			Importable:      importable(pkgPathOf(prog.ModulePath, f.File), pkgName),
			BranchesRemoved: f.BranchesRemoved && f.MovedBranches < f.RemovedBranches,
			AddedLines:      len(f.AddedLines),
			ReachUnknown:    graph == nil,
		}
		pkgPath := pkgPathOf(prog.ModulePath, f.File)
		var cross []string
		var uncovered []int
		if prof != nil {
			in.HasProfile = true
			in.Covered, in.Total = prof.Lines(f.File, f.AddedLines)
			uncovered = prof.Uncovered(f.File, f.AddedLines)
			if in.Total > 0 && in.Covered == 0 {
				cross = crossPkgTests(pkgPath, im.TestPkgs)
			}
			if len(cross) > 0 {
				in.CrossPkgTestReached = true
				m.CrossPkgOnly++
				// The cover cell says "reached by tests in …": those tests
				// may cover the lines a same-package profile marks 0.
				uncovered = nil
			}
		}
		in.Wiring = f.Wiring
		// Sensitive: a table API in its body, or a critical package. A
		// declaration the program does not hold is not sensitive.
		var apis []string
		var sites []sensitive.Site
		critical := ""
		if fd, info, declPkg, ok := prog.FuncDecl(f.File, f.StartLine); ok {
			in.Root = fd.Recv == nil && (fd.Name.Name == "init" || (fd.Name.Name == "main" && pkgName == "main"))
			sites = table.Sites(fd, info, prog.Fset)
			if g, ok := sensitive.Critical(declPkg, prog.ModulePath, cfg.Critical); ok {
				critical = g
			}
		}
		apis = touchedAPIs(sites, f.AddedLines, critical)
		in.Sensitive = len(apis) > 0 || critical != ""
		cover := coverText(prof != nil, in.Covered, in.Total, len(f.AddedLines), graph != nil, im.TestReached, cross, prog.ModulePath)
		var entries []callgraph.Entry
		if graph != nil {
			entries = graph.EntryPoints(f.File, f.StartLine, entryRoot(prog))
		}
		var callers1 []string
		for _, c := range im.Callers {
			if c.Depth == 1 { // calls, dispatch and registrations all count
				callers1 = append(callers1, c.Display+"\x00"+c.Pkg)
			}
		}
		r := risk.Rate(in)
		level := r.Level
		if in.Sensitive {
			// Its own row, also when folded into a group later.
			m.Sensitive = append(m.Sensitive, report.SensitiveRow{
				Level: level, Display: f.Display, APIs: apis, Critical: critical,
				CoverText: cover, Untested: in.SensitiveUntested(),
			})
		}
		rated = append(rated, ratedFunc{
			Row: report.FuncRow{
				Level: level, Reason: r.Reason, Display: f.Display, Kind: kindText(f, in.Sensitive),
				Callers: im.Callers, CoverText: cover, Registered: im.Registered, Entries: entries, File: f.File, Uncovered: uncovered,
			},
			Key: f.Display + "\x00" + pkgPath, New: f.Kind == symbols.New, In: in,
			Callers1: callers1, CrossPkgs: cross,
		})
	}
	if graph != nil {
		m.Funcs, m.Folded, m.Groups = group(rated, prog.ModulePath)
	} else {
		// Callers unknown: nothing can be shown to be called only by new code.
		for _, r := range rated {
			m.Funcs = append(m.Funcs, r.Row)
		}
	}

	// 9. Removed case values that still exist: those values now reach
	// default, or no case at all.
	m.RemovedCases = stillDeclared(prog, syms.RemovedCases)

	// 10. Registrations removed, rewired, changed and added.
	b := <-baseCh
	baseDone = true
	if b.err != nil {
		m.Warnings = append(m.Warnings, "Registrations not compared: the base revision could not be loaded.")
	} else {
		head := registrations.Find(prog.Packages, prog.ModulePath, prog.Rel)
		m.RegRewired, m.RegChanged, m.RegRemoved, m.RegAdded = registrations.Compare(b.regs, head)
	}

	// Architecture: the head import graph judged against the model, and
	// against the base graph when the base loaded.
	model := arch.Inferred()
	if cfg.Arch != nil {
		model = *cfg.Arch
	}
	res := arch.Analyze(model, b.graph, arch.FromPackages(prog.Packages, prog.ModulePath, prog.Rel), touchedPkgs(prog, changes))
	m.Arch = &res

	// What the removed code used: resolved with the BASE types when the base
	// loaded, else guessed from syntax (by package and name).
	names := map[string]bool{}
	for r := range syms.DeletedRefs {
		names[r.Name] = true
	}
	usedField := func(pkgPath, name string) bool { return names[name] }
	usedFunc := func(f callgraph.FuncInfo) bool {
		if prog.Generated[f.File] {
			return names[f.Name] // x.Query(): the receiver's package is unknown
		}
		return syms.DeletedRefs[symbols.Ref{Import: f.Pkg, Name: f.Name}] ||
			syms.DeletedRefs[symbols.Ref{Dir: dirOf(prog.ModulePath, f.Pkg), Name: f.Name}]
	}
	if b.err == nil {
		usedField = func(pkgPath, name string) bool { return b.uses[use{Pkg: pkgPath, Name: name, Field: true}] }
		usedFunc = func(f callgraph.FuncInfo) bool {
			return b.uses[use{Pkg: f.Pkg, Recv: f.Recv, Name: f.Name}] ||
				(f.Method && b.uses[use{Recv: anyRecv, Name: f.Name}])
		}
	}

	// 11. Configuration fields the removed code used that nothing reads now.
	m.UnusedConfig = configfields.Unused(prog.Packages, prog.Rel, usedField)

	// 12. Functions the removed code used that nothing reachable uses any
	// more, and what only they reached.
	if graph != nil {
		required := interfaceMethods(prog)
		for _, u := range graph.Dead(apiRoot(prog), usedFunc) {
			switch {
			case isProtobuf(u.File):
			case required(u):
				// It implements a method an interface still declares (e.g. a
				// mock): removing it alone would not compile.
			case !prog.Generated[u.File]:
				m.Unused = append(m.Unused, u)
			case !u.TestOnly: // mocks are generated and used by tests only
				m.UnusedGenerated = append(m.UnusedGenerated, u)
			}
		}
	}
	return m, nil
}

// configNote states the effective config when it removes defaults or names
// critical globs, or when the MR changed the file (head against base; a base
// without the file is the empty config). The base file never fails the run:
// one that cannot be read or parsed is reported as unreadable.
func configNote(ctx context.Context, cfg Config, baseSHA string) *report.ConfigNote {
	if cfg.File == nil {
		return nil
	}
	head := *cfg.File
	n := &report.ConfigNote{File: cfg.FileName, Critical: head.Critical, Removes: head.SensitiveRemove, Adds: head.SensitiveAdd}
	if head.Architecture.Set {
		n.Architecture = head.Architecture.Name()
	}
	if cfg.FileRel != "" {
		var base config.Config
		src, err := diff.ShowFile(ctx, cfg.Dir, baseSHA, cfg.FileRel)
		if err == nil {
			base, err = config.Parse(baseSHA[:min(7, len(baseSHA))]+":"+cfg.FileRel, bytes.NewReader(src))
		} else if errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
		if err != nil {
			n.Changed, n.BaseUnreadable = true, true
		} else if n.Changes = config.Changes(base, head); len(n.Changes) > 0 {
			n.Changed = true
		}
	}
	if !n.Changed && len(head.Critical) == 0 && len(head.SensitiveRemove) == 0 && !head.Architecture.Set {
		return nil
	}
	return n
}

// interfaceMethods returns a predicate reporting whether a dead method
// implements a method of an interface declared in the module, so it is
// required by that interface.
func interfaceMethods(prog *load.Program) func(callgraph.Unused) bool {
	byPath := map[string]*types.Package{}
	var ifaces []*types.Interface
	for _, p := range prog.Packages {
		if p.Types == nil || byPath[p.PkgPath] != nil {
			continue
		}
		byPath[p.PkgPath] = p.Types
		for _, name := range p.Types.Scope().Names() {
			if tn, ok := p.Types.Scope().Lookup(name).(*types.TypeName); ok {
				// A generated interface (sqlc's Querier) lists everything
				// generated beside it: it requires nothing.
				if f, ok := prog.Rel(prog.Fset.Position(tn.Pos()).Filename); ok && prog.Generated[f] {
					continue
				}
				if it, ok := tn.Type().Underlying().(*types.Interface); ok && it.NumMethods() > 0 {
					ifaces = append(ifaces, it)
				}
			}
		}
	}
	return func(u callgraph.Unused) bool {
		// Display "pkg.(*T).M" or "pkg.T.M": a method has two dots.
		rest, ok := strings.CutPrefix(u.Display, strings.SplitN(u.Display, ".", 2)[0]+".")
		if !ok || !strings.Contains(rest, ".") {
			return false
		}
		i := strings.LastIndex(rest, ".")
		recv, method := strings.Trim(rest[:i], "(*)"), rest[i+1:]
		if j := strings.Index(recv, "["); j >= 0 {
			recv = recv[:j]
		}
		pkg := byPath[u.Pkg]
		if pkg == nil {
			return false
		}
		tn, ok := pkg.Scope().Lookup(recv).(*types.TypeName)
		if !ok {
			return false
		}
		for _, it := range ifaces {
			if !hasMethod(it, method) {
				continue
			}
			if types.Implements(tn.Type(), it) || types.Implements(types.NewPointer(tn.Type()), it) {
				return true
			}
		}
		return false
	}
}

func hasMethod(it *types.Interface, name string) bool {
	for i := range it.NumMethods() {
		if it.Method(i).Name() == name {
			return true
		}
	}
	return false
}

// importable reports whether another module can import the package: not a
// main package and not under an internal/ directory.
func importable(pkgPath, pkgName string) bool {
	if pkgName == "main" {
		return false
	}
	for _, seg := range strings.Split(pkgPath, "/") {
		if seg == "internal" {
			return false
		}
	}
	return true
}

// dirOf maps an import path of module mod to its repo-relative dir; "" for
// packages of other modules.
func dirOf(mod, pkgPath string) string {
	switch {
	case pkgPath == mod:
		return "."
	case strings.HasPrefix(pkgPath, mod+"/"):
		return pkgPath[len(mod)+1:]
	}
	return ""
}

// pkgPathOf returns the import path of the package holding repo-relative
// file, under module path mod.
func pkgPathOf(mod, file string) string {
	if dir := path.Dir(file); dir != "." {
		return mod + "/" + dir
	}
	return mod
}

// stillDeclared keeps the removed cases whose constant or type is still
// declared at HEAD.
func stillDeclared(prog *load.Program, cs []symbols.RemovedCase) []report.RemovedCase {
	byPath := map[string]*types.Package{}
	for _, p := range prog.Packages {
		if p.Types != nil && byPath[p.PkgPath] == nil {
			byPath[p.PkgPath] = p.Types
		}
	}
	var out []report.RemovedCase
	for _, c := range cs {
		pkgPath := c.Ref.Import
		if pkgPath == "" {
			pkgPath = pkgPathOf(prog.ModulePath, c.Ref.Dir+"/x.go")
		}
		pkg := byPath[pkgPath]
		if pkg == nil {
			continue
		}
		switch pkg.Scope().Lookup(c.Ref.Name).(type) {
		case *types.Const, *types.TypeName:
			out = append(out, report.RemovedCase{Func: c.Func, File: c.File, Line: c.Line, Case: c.Case, HasDefault: c.HasDefault})
		}
	}
	return out
}

type baseResult struct {
	regs  []registrations.Registration
	uses  map[use]bool // what the deleted lines name
	graph *arch.Graph  // the base import graph; nil when err is set
	err   error
}

// loadBase exports the base revision into a temporary directory (the
// repository is not touched), type-checks it and finds its registrations.
func loadBase(ctx context.Context, cfg Config, baseSHA string, deletedLines map[string]map[int]bool) baseResult {
	tmp, err := os.MkdirTemp("", "mrimpact-base-*")
	if err != nil {
		return baseResult{err: err}
	}
	defer os.RemoveAll(tmp)
	if err := diff.Export(ctx, cfg.Dir, baseSHA, tmp); err != nil {
		return baseResult{err: err}
	}
	pkgs, rel, err := load.Syntax(ctx, tmp, cfg.Tags)
	if err != nil {
		return baseResult{err: err}
	}
	mod := ""
	for _, p := range pkgs {
		if p.Module != nil {
			mod = p.Module.Path
			break
		}
	}
	g := arch.FromPackages(pkgs, mod, rel)
	return baseResult{regs: registrations.Find(pkgs, mod, rel), uses: deletedUses(pkgs, rel, deletedLines), graph: &g}
}

// touchedPkgs returns the import paths of the head packages a non-test .go
// change of the MR touches (sorted, unique): those holding an added, modified
// or renamed file at head, and, for a deleted file or a rename's old path, the
// head package whose non-test files live in that file's directory, if any.
func touchedPkgs(prog *load.Program, changes []diff.FileChange) []string {
	files, dirs := map[string]bool{}, map[string]bool{}
	for _, c := range changes {
		if c.IsTest {
			continue
		}
		if c.Path != "" {
			files[c.Path] = true
		}
		if c.OldPath != "" && (c.Status == diff.Deleted || c.Status == diff.Renamed) {
			dirs[path.Dir(c.OldPath)] = true
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range prog.Packages {
		for _, f := range append(slices.Clone(p.GoFiles), p.CompiledGoFiles...) {
			r, ok := prog.Rel(f)
			if !ok || seen[p.PkgPath] || strings.HasSuffix(r, "_test.go") {
				continue
			}
			if files[r] || dirs[path.Dir(r)] {
				seen[p.PkgPath] = true
				out = append(out, p.PkgPath)
			}
		}
	}
	slices.Sort(out)
	return out
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

func kindText(f symbols.ChangedFunc, isSensitive bool) string {
	k := string(f.Kind)
	if f.Kind == symbols.Signature && f.Exported {
		k = "signature (exported)"
	}
	if f.BranchesRemoved {
		k += branchText(f)
	}
	if f.Wiring {
		k += " · wiring"
	}
	if isSensitive {
		k += " · sensitive"
	}
	return k
}

// branchText renders the branch-removed suffix: moved branches name where
// they went (at most two destinations, then " +N").
func branchText(f symbols.ChangedFunc) string {
	if f.MovedBranches == 0 || len(f.MovedTo) == 0 {
		return " (branch removed)"
	}
	var names []string
	for i, d := range f.MovedTo {
		if i == 2 {
			break
		}
		names = append(names, "`"+d+"`")
	}
	to := strings.Join(names, ", ")
	if extra := len(f.MovedTo) - 2; extra > 0 {
		to += fmt.Sprintf(" +%d", extra)
	}
	if f.MovedBranches >= f.RemovedBranches {
		return " (branch moved to " + to + ")"
	}
	return fmt.Sprintf(" (branch removed · %d of %d moved to %s)", f.MovedBranches, f.RemovedBranches, to)
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

// apiRoot reports whether f is public API: exported methods may implement
// interfaces of dependencies (String, MarshalJSON, ServeHTTP); exported
// functions of an importable package may be called from another module. Not
// so for generated code: it is API only if something uses it.
func apiRoot(prog *load.Program) func(callgraph.FuncInfo) bool {
	return func(f callgraph.FuncInfo) bool {
		exported := ast.IsExported(f.Name)
		return exported && (f.Method || importable(f.Pkg, f.PkgName)) && !prog.Generated[f.File]
	}
}

// entryRoot is apiRoot narrowed for the Entry points column: exported
// functions of importable packages, and exported methods of exported types
// in importable packages; generated code never. An exported method of an
// unexported or internal type is not a way in that a reader would name.
func entryRoot(prog *load.Program) func(callgraph.FuncInfo) bool {
	return func(f callgraph.FuncInfo) bool {
		if !ast.IsExported(f.Name) || !importable(f.Pkg, f.PkgName) || prog.Generated[f.File] {
			return false
		}
		return !f.Method || ast.IsExported(f.Recv)
	}
}

// touchedAPIs is the distinct APIs (first-occurrence order) of the sites an
// added line falls in; with a critical glob, every API of the function. A
// function none of whose sensitive calls was touched has none.
func touchedAPIs(sites []sensitive.Site, added []int, critical string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range sites {
		if seen[s.API] {
			continue
		}
		hit := critical != ""
		for _, l := range added {
			if hit {
				break
			}
			hit = l >= s.From && l <= s.To
		}
		if hit {
			seen[s.API] = true
			out = append(out, s.API)
		}
	}
	return out
}
