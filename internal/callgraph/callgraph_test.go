package callgraph

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/load"
)

const fixture = "testdata/iface"

var (
	fixtureOnce  sync.Once
	fixtureProg  *load.Program
	fixtureGraph *Graph
	fixtureErr   error
)

// loadFixture loads and builds the iface fixture once for all tests.
func loadFixture(t *testing.T) (*load.Program, *Graph) {
	t.Helper()
	fixtureOnce.Do(func() {
		fixtureProg, fixtureErr = load.Load(context.Background(), fixture, nil)
		if fixtureErr == nil {
			fixtureGraph, fixtureErr = Build(context.Background(), fixtureProg)
		}
	})
	if fixtureErr != nil {
		t.Fatal(fixtureErr)
	}
	return fixtureProg, fixtureGraph
}

// lineOf returns the 1-based line of the first line in dir/file containing s.
func lineOf(t *testing.T, dir, file, s string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(string(data), "\n") {
		if strings.Contains(l, s) {
			return i + 1
		}
	}
	t.Fatalf("%q not found in %s", s, file)
	return 0
}

func callerAt(im Impact, display string) (Caller, bool) {
	for _, c := range im.Callers {
		if c.Display == display {
			return c, true
		}
	}
	return Caller{}, false
}

func TestInterfaceCallerDetected(t *testing.T) {
	_, g := loadFixture(t)
	im := g.Impact("store/mem.go", lineOf(t, fixture, "store/mem.go", "func (s *memStore) Get"), 2)
	if !im.Found {
		t.Fatal("memStore.Get not found")
	}
	want := []Caller{
		{Display: "svc.Lookup", Pkg: "example.com/iface/svc", Depth: 1, Via: "store.(*memStore).Get"},
		{Display: "api.Handle", Pkg: "example.com/iface/api", Depth: 2, Via: "svc.Lookup"},
	}
	if len(im.Callers) != len(want) {
		t.Fatalf("callers = %+v, want %+v", im.Callers, want)
	}
	for i := range want {
		if im.Callers[i] != want[i] {
			t.Errorf("caller[%d] = %+v, want %+v", i, im.Callers[i], want[i])
		}
	}
	if !im.TestReached || im.TestCallers != 1 {
		t.Errorf("TestReached=%v TestCallers=%d, want true, 1", im.TestReached, im.TestCallers)
	}
}

func TestDepthLimit(t *testing.T) {
	_, g := loadFixture(t)
	im := g.Impact("store/mem.go", lineOf(t, fixture, "store/mem.go", "func (s *memStore) Get"), 1)
	if _, ok := callerAt(im, "api.Handle"); ok {
		t.Errorf("api.Handle present at depth 1: %+v", im.Callers)
	}
	if _, ok := callerAt(im, "svc.Lookup"); !ok {
		t.Errorf("svc.Lookup missing at depth 1: %+v", im.Callers)
	}
	if !im.TestReached || im.TestCallers != 0 {
		t.Errorf("TestReached=%v TestCallers=%d, want true, 0", im.TestReached, im.TestCallers)
	}
}

func TestNoCallers(t *testing.T) {
	_, g := loadFixture(t)
	im := g.Impact("unused.go", lineOf(t, fixture, "unused.go", "func Orphan"), 2)
	if !im.Found || len(im.Callers) != 0 || im.TestReached {
		t.Errorf("Orphan impact = %+v, want found, no callers, not test-reached", im)
	}
}

func TestBuildTagExcluded(t *testing.T) {
	p, g := loadFixture(t)
	im := g.Impact("tagged.go", lineOf(t, fixture, "tagged.go", "func Special"), 2)
	if im.Found {
		t.Error("Special found despite build tag")
	}
	if p.Files["tagged.go"] {
		t.Error(`Files["tagged.go"] = true`)
	}
	if !p.Files["unused.go"] || !p.Files["svc/svc_test.go"] {
		t.Errorf("Files missing compiled files: %v", p.Files)
	}
}

func TestBuildTagIncluded(t *testing.T) {
	p, err := load.Load(context.Background(), fixture, []string{"special"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := Build(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Files["tagged.go"] || !g.Impact("tagged.go", lineOf(t, fixture, "tagged.go", "func Special"), 2).Found {
		t.Error("Special not analyzed with -tags special")
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBrokenPackage(t *testing.T) {
	dir := t.TempDir()
	copyDir(t, fixture, dir)
	broken := "package api\n\nfunc Handle() string {\n\treturn 42\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "api", "api.go"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load.Load(context.Background(), dir, nil)
	if err != nil || p == nil {
		t.Fatalf("Load = %v, %v", p, err)
	}
	if len(p.Broken) != 1 || !strings.HasSuffix(p.Broken[0].PkgPath, "/api") {
		t.Fatalf("Broken = %+v, want one entry ending in /api", p.Broken)
	}
	if b := p.Broken[0]; len(b.Files) != 1 || b.Files[0] != "api/api.go" || b.Msg == "" {
		t.Errorf("Broken[0] = %+v", b)
	}
	g, err := Build(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	im := g.Impact("store/mem.go", lineOf(t, dir, "store/mem.go", "func (s *memStore) Get"), 2)
	if _, ok := callerAt(im, "svc.Lookup"); !ok || !im.TestReached {
		t.Errorf("svc not analyzable: %+v", im)
	}
	if _, ok := callerAt(im, "api.Handle"); ok {
		t.Errorf("broken api.Handle listed: %+v", im.Callers)
	}
}

// A //go:embed pattern with no match is a list error, not a type error: the
// package and its importers stay analyzable and are reported as warnings.
func TestEmbedMissingPackage(t *testing.T) {
	dir := t.TempDir()
	copyDir(t, fixture, dir)
	embed := "package store\n\nimport \"embed\"\n\n//go:embed missing/dir\nvar assets embed.FS\n"
	if err := os.WriteFile(filepath.Join(dir, "store", "assets.go"), []byte(embed), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load.Load(context.Background(), dir, nil)
	if err != nil || p == nil {
		t.Fatalf("Load = %v, %v", p, err)
	}
	if len(p.Broken) != 0 {
		t.Fatalf("Broken = %+v, want none", p.Broken)
	}
	if len(p.Warnings) != 1 || p.Warnings[0].PkgPath != "example.com/iface/store" {
		t.Fatalf("Warnings = %+v, want one entry for example.com/iface/store", p.Warnings)
	}
	if w := p.Warnings[0]; !strings.Contains(w.Msg, "pattern missing/dir") || strings.Contains(w.Msg, "\n") {
		t.Errorf("Warnings[0].Msg = %q, want single-line embed error", w.Msg)
	}
	g, err := Build(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	im := g.Impact("svc/svc.go", lineOf(t, dir, "svc/svc.go", "func Lookup"), 2)
	if _, ok := callerAt(im, "api.Handle"); !ok || !im.TestReached {
		t.Errorf("importer svc.Lookup not analyzable: %+v", im)
	}
	im = g.Impact("store/mem.go", lineOf(t, dir, "store/mem.go", "func (s *memStore) Get"), 2)
	if _, ok := callerAt(im, "svc.Lookup"); !ok {
		t.Errorf("embed package store not analyzable: %+v", im)
	}
}

// An import of a module that doesn't exist must still mark the importer
// Broken (it can't be type-checked completely).
func TestMissingImportBroken(t *testing.T) {
	dir := t.TempDir()
	copyDir(t, fixture, dir)
	src := "package api\n\nimport \"example.com/doesnotexist\"\n\nfunc Handle() string {\n\treturn doesnotexist.Name()\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "api", "api.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load.Load(context.Background(), dir, nil)
	if err != nil || p == nil {
		t.Fatalf("Load = %v, %v", p, err)
	}
	if len(p.Broken) != 1 || p.Broken[0].PkgPath != "example.com/iface/api" {
		t.Fatalf("Broken = %+v, want example.com/iface/api", p.Broken)
	}
	if len(p.Warnings) != 0 {
		t.Errorf("Warnings = %+v, want none", p.Warnings)
	}
}

func TestBuildTimeout(t *testing.T) {
	p, _ := loadFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, p); !errors.Is(err, context.Canceled) {
		t.Errorf("Build err = %v, want context.Canceled", err)
	}
}

func TestGenericClosureCaller(t *testing.T) {
	_, g := loadFixture(t)
	im := g.Impact("gen/gen.go", lineOf(t, fixture, "gen/gen.go", "func (l *List[T]) Push"), 2)
	want := []Caller{{Display: "gen.Use", Pkg: "example.com/iface/gen", Depth: 1, Via: "gen.(*List[T]).Push"}}
	if !im.Found || len(im.Callers) != 1 || im.Callers[0] != want[0] {
		t.Fatalf("impact = %+v, want callers %+v", im, want)
	}
	if !im.TestReached || im.TestCallers != 1 {
		t.Errorf("TestReached=%v TestCallers=%d, want true, 1", im.TestReached, im.TestCallers)
	}
}

func displays(cs []Caller) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Display)
	}
	return out
}

// Through an event-bus style dispatcher, only the code that registered the
// handler is a depth-2 caller, not everything else that subscribes.
func TestDispatcherNarrowedToRegistrant(t *testing.T) {
	_, g := loadFixture(t)
	im := g.Impact("wire/wire.go", lineOf(t, fixture, "wire/wire.go", "func (a *A) handle"), 3)
	got := strings.Join(displays(im.Callers), ", ")
	want := "bus.(*Bus).Subscribe, wire.NewA, wire.Main"
	if got != want {
		t.Errorf("callers = %s, want %s", got, want)
	}
	if len(im.Callers) == 3 && (im.Callers[0].Kind != Dispatch || im.Callers[1].Kind != Registers || im.Callers[2].Kind != Calls) {
		t.Errorf("kinds = %q %q %q, want dispatch, registers, calls", im.Callers[0].Kind, im.Callers[1].Kind, im.Callers[2].Kind)
	}
	if len(im.Registered) != 1 {
		t.Fatalf("Registered = %+v, want 1", im.Registered)
	}
	r := im.Registered[0]
	if r.File != "wire/wire.go" || r.Line != lineOf(t, fixture, "wire/wire.go", `b.Subscribe("topic.a"`) ||
		r.Call != `Subscribe("topic.a", …)` {
		t.Errorf("Registered[0] = %+v", r)
	}
}

// Callers of a closure are not callers of the function that created it.
func TestClosureCallersNotAttributedToCreator(t *testing.T) {
	_, g := loadFixture(t)
	im := g.Impact("wire/wire.go", lineOf(t, fixture, "wire/wire.go", "func NewSvc"), 2)
	if got := strings.Join(displays(im.Callers), ", "); got != "wire.Main" {
		t.Errorf("NewSvc callers = %s, want wire.Main", got)
	}
	im = g.Impact("wire/wire.go", lineOf(t, fixture, "wire/wire.go", "func Save"), 2)
	if got := strings.Join(displays(im.Callers), ", "); got != "wire.Main" {
		t.Errorf("Save callers = %s, want wire.Main (not bus.RunTx)", got)
	}
}

// Dead reports unreachable seeded functions, what only they reached
// (Via), and marks what tests still reach.
func TestDead(t *testing.T) {
	_, g := loadFixture(t)
	seeds := map[string]bool{"lost": true, "testOnly": true, "gone": true, "registered": true, "kept": true, "String": true}
	root := func(f FuncInfo) bool { return ast.IsExported(f.Name) }
	seed := func(f FuncInfo) bool { return f.Pkg == "example.com/iface/dead" && seeds[f.Name] }
	var got []string
	for _, u := range g.Dead(root, seed) {
		s := u.Display
		if u.TestOnly {
			s += " (tests)"
		}
		if u.Via != "" {
			s += " via " + u.Via
		}
		got = append(got, s)
	}
	want := "dead.lost, dead.helper via dead.lost, dead.testOnly (tests), dead.t.gone"
	if strings.Join(got, ", ") != want {
		t.Errorf("Dead = %s\nwant   %s", strings.Join(got, ", "), want)
	}
}

func TestImpact_TestPkgs(t *testing.T) {
	const dir = "testdata/testpkgs"
	p, err := load.Load(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	g, err := Build(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	im := g.Impact("a/a.go", lineOf(t, dir, "a/a.go", "func A()"), 2)
	want := []string{"example.com/tp/a", "example.com/tp/a_test", "example.com/tp/b", "example.com/tp/c"}
	if !im.TestReached || !reflect.DeepEqual(im.TestPkgs, want) {
		t.Errorf("TestPkgs = %v (reached %v), want %v", im.TestPkgs, im.TestReached, want)
	}
	im = g.Impact("a/a.go", lineOf(t, dir, "a/a.go", "func Lonely"), 2)
	if !im.Found || im.TestPkgs != nil {
		t.Errorf("Lonely: found=%v TestPkgs=%v, want found and nil", im.Found, im.TestPkgs)
	}
}

var (
	epOnce  sync.Once
	epGraph *Graph
	epErr   error
)

func loadEntries(t *testing.T) *Graph {
	t.Helper()
	epOnce.Do(func() {
		var p *load.Program
		p, epErr = load.Load(context.Background(), "testdata/entries", nil)
		if epErr == nil {
			epGraph, epErr = Build(context.Background(), p)
		}
	})
	if epErr != nil {
		t.Fatal(epErr)
	}
	return epGraph
}

// apiPred mirrors the pipeline's root predicate (generated code aside).
func apiPred(f FuncInfo) bool {
	if !ast.IsExported(f.Name) || f.PkgName == "main" {
		return false
	}
	if !f.Method {
		for _, seg := range strings.Split(f.Pkg, "/") {
			if seg == "internal" {
				return false
			}
		}
	}
	return true
}

func TestEntryPoints(t *testing.T) {
	g := loadEntries(t)
	const dir = "testdata/entries"
	got := g.EntryPoints("internal/svc/svc.go", lineOf(t, dir, "internal/svc/svc.go", "func helper"), apiPred)
	want := []Entry{
		{"registered", "…/x"}, // mux.HandleFunc("/x", handle) on a parameter: a partial route
		{"api", "api.Do"},
		{"main", "main.main"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EntryPoints = %v, want %v", got, want)
	}
	if got := g.EntryPoints("internal/svc/svc.go", lineOf(t, dir, "internal/svc/svc.go", "func onlyTested"), apiPred); got != nil {
		t.Errorf("test-only function: EntryPoints = %v, want nil", got)
	}
	if got := g.EntryPoints("internal/svc/svc.go", 1, apiPred); got != nil {
		t.Errorf("unknown line: EntryPoints = %v, want nil", got)
	}
}

func TestEntryPoints_Cap(t *testing.T) {
	g := loadEntries(t)
	got := g.EntryPoints("many/many.go", lineOf(t, "testdata/entries", "many/many.go", "func target"), apiPred)
	if len(got) != 20 {
		t.Errorf("len = %d, want 20 (%v)", len(got), got)
	}
	if len(got) > 0 && got[0] != (Entry{"api", "many.Call01"}) {
		t.Errorf("first = %v", got[0])
	}
}

// A registration that is not a call argument has no Call text: its entry
// falls back to the site's file:line, never an empty Display.
func TestEntryPoints_RegisteredWithoutCall(t *testing.T) {
	g := loadEntries(t)
	const dir = "testdata/entries"
	got := g.EntryPoints("internal/svc/table.go", lineOf(t, dir, "internal/svc/table.go", "func hidden"), apiPred)
	want := Entry{"registered", fmt.Sprintf("internal/svc/table.go:%d", lineOf(t, dir, "internal/svc/table.go", "var table"))}
	found := false
	for _, e := range got {
		if e.Display == "" {
			t.Errorf("empty entry in %v", got)
		}
		found = found || e == want
	}
	if !found {
		t.Errorf("EntryPoints = %v, want it to hold %v", got, want)
	}
}

func registeredAt(t *testing.T, fn string) []Site {
	t.Helper()
	g := loadEntries(t)
	const file = "internal/router/router.go"
	return g.Impact(file, lineOf(t, "testdata/entries", file, "func "+fn+"("), 1).Registered
}

// A handler passed through a variadic parameter (hs ...func()) sits in an
// SSA slice: its site is still the call, rendered as the route.
func TestSite_Variadic(t *testing.T) {
	got := registeredAt(t, "h")
	line := lineOf(t, "testdata/entries", "internal/router/router.go", `g.POST("/x"`)
	want := []Site{{File: "internal/router/router.go", Line: line, Call: "POST …/x"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Registered = %+v, want %+v", got, want)
	}
}

// Middleware and handler both from the module, in one variadic slice: both
// resolve to the same call text.
func TestSite_VariadicTwoModuleFuncs(t *testing.T) {
	mw, h := registeredAt(t, "mw"), registeredAt(t, "h")
	if len(mw) != 1 || len(h) != 1 || mw[0].Call != "POST …/x" || mw[0].Text() != h[0].Text() {
		t.Errorf("mw = %+v, h = %+v, want one equal non-empty call each", mw, h)
	}
}

// A direct (non-variadic) argument keeps today's call text.
func TestSite_DirectArgUnchanged(t *testing.T) {
	got := registeredAt(t, "direct")
	if len(got) != 1 || got[0].Call != `Subscribe("topic.a", …)` {
		t.Errorf("Registered = %+v, want Subscribe(\"topic.a\", …)", got)
	}
}

// A variadic call that is not a route keeps its call text, every argument as
// written: both module functions share it.
func TestSite_VariadicNotRoute(t *testing.T) {
	a, b := registeredAt(t, "u1"), registeredAt(t, "u2")
	if len(a) != 1 || a[0].Call != "Use(u1, u2)" || len(b) != 1 || b[0].Call != a[0].Call {
		t.Errorf("u1 = %+v, u2 = %+v, want Use(u1, u2) each", a, b)
	}
}

// A slice whose use is not exactly one real call (a range loop, len or
// append before the call, two calls) is not guessed at: the site keeps its
// file:line.
func TestSite_SliceNotOneCallKeepsFileLine(t *testing.T) {
	const dir, file = "testdata/entries", "internal/router/router.go"
	for _, tc := range []struct{ fn, line string }{
		{"r1", "range []func(){r1, r2}"},
		{"l1", "hs := []func(){l1}"},
		{"a1", "hs := []func(){a1}"},
		{"t1", "hs := []func(){t1}"},
	} {
		got := registeredAt(t, tc.fn)
		want := []Site{{File: file, Line: lineOf(t, dir, file, tc.line)}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: Registered = %+v, want %+v", tc.fn, got, want)
		}
	}
}

// An explicit spread keeps its "..." in the call text.
func TestSite_SpreadKeepsEllipsis(t *testing.T) {
	got := registeredAt(t, "s1")
	if len(got) != 1 || got[0].Call != "Use(mws...)" {
		t.Errorf("Registered = %+v, want Use(mws...)", got)
	}
	got = registeredAt(t, "s3")
	if len(got) != 1 || got[0].Call != "Use([]func(){…}...)" {
		t.Errorf("Registered = %+v, want Use([]func(){…}...) (ExprString elides literals)", got)
	}
}

const lcDir = "testdata/libcalled"

var (
	lcOnce  sync.Once
	lcGraph *Graph
	lcErr   error
)

// loadLibCalled loads and builds the libcalled fixture once.
func loadLibCalled(t *testing.T) *Graph {
	t.Helper()
	lcOnce.Do(func() {
		var p *load.Program
		p, lcErr = load.Load(context.Background(), lcDir, nil)
		if lcErr == nil {
			lcGraph, lcErr = Build(context.Background(), p)
		}
	})
	if lcErr != nil {
		t.Fatal(lcErr)
	}
	return lcGraph
}

func lcImpact(t *testing.T, file, decl string) Impact {
	t.Helper()
	im := loadLibCalled(t).Impact(file, lineOf(t, lcDir, file, decl), 2)
	if !im.Found {
		t.Fatalf("%s not found", decl)
	}
	return im
}

// errors.Is calls Unwrap inside the standard library, where VTA sees no
// body: the test that converts candidateErrors to error reaches it, but is
// not a caller.
func TestLibCalled_UnwrapViaErrorsIs(t *testing.T) {
	im := lcImpact(t, "errs.go", "func (candidateErrors) Unwrap")
	if !im.TestReached || len(im.Callers) != 0 {
		t.Errorf("TestReached=%v Callers=%+v, want true, none", im.TestReached, im.Callers)
	}
	if want := []string{"example.com/lc"}; !reflect.DeepEqual(im.TestPkgs, want) {
		t.Errorf("TestPkgs = %v, want %v", im.TestPkgs, want)
	}
}

func TestLibCalled_WrongSignature(t *testing.T) {
	if im := lcImpact(t, "str.go", "func (bad) Unwrap"); im.TestReached {
		t.Errorf("bad.Unwrap (func() string) test-reached: %+v", im)
	}
}

func TestLibCalled_Generic(t *testing.T) {
	if im := lcImpact(t, "gen.go", "func (E[T]) Unwrap"); !im.TestReached {
		t.Errorf("E.Unwrap not test-reached through E[int]: %+v", im)
	}
}

func TestLibCalled_ValueConversionNoPointerMethods(t *testing.T) {
	if im := lcImpact(t, "ptr.go", "func (*P) String"); im.TestReached {
		t.Errorf("(*P).String test-reached though only P{} is converted: %+v", im)
	}
}

func TestLibCalled_EntryPoints(t *testing.T) {
	g := loadLibCalled(t)
	got := g.EntryPoints("h.go", lineOf(t, lcDir, "h.go", "func (H) ServeHTTP"), apiPred)
	want := Entry{"main", "main.main"}
	found := false
	for _, e := range got {
		found = found || e == want
	}
	if !found {
		t.Errorf("EntryPoints = %v, want it to hold %v", got, want)
	}
}

func TestLibCalled_NotACaller(t *testing.T) {
	im := lcImpact(t, "errs.go", "func (candidateErrors) Error")
	if len(im.Callers) != 0 || im.TestCallers != 0 {
		t.Errorf("Callers=%+v TestCallers=%d, want none, 0", im.Callers, im.TestCallers)
	}
	if !im.TestReached {
		t.Errorf("candidateErrors.Error not test-reached: %+v", im)
	}
}

func TestLibraryCalledList(t *testing.T) {
	if len(libCalledList) != 22 {
		t.Errorf("list has %d entries, want 22", len(libCalledList))
	}
	sig := types.NewSignatureType(nil, nil, nil, nil, types.NewTuple(types.NewParam(0, nil, "", types.Typ[types.String])), false)
	if !libraryCalled(types.NewFunc(0, nil, "Error", sig)) {
		t.Error("Error() string not library-called")
	}
	if libraryCalled(types.NewFunc(0, nil, "Unwrap", sig)) {
		t.Error("Unwrap() string library-called")
	}
	// Parameter and result names are not part of the signature.
	errT := types.Universe.Lookup("error").Type()
	bytesT := types.NewSlice(types.Typ[types.Byte]) // prints []uint8: identical to []byte
	is := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewParam(0, nil, "target", errT)),
		types.NewTuple(types.NewParam(0, nil, "", types.Typ[types.Bool])), false)
	if !libraryCalled(types.NewFunc(0, nil, "Is", is)) {
		t.Error("Is(target error) bool not library-called")
	}
	mj := types.NewSignatureType(nil, nil, nil, nil,
		types.NewTuple(types.NewParam(0, nil, "b", bytesT), types.NewParam(0, nil, "err", errT)), false)
	if !libraryCalled(types.NewFunc(0, nil, "MarshalJSON", mj)) {
		t.Error("MarshalJSON() (b []byte, err error) not library-called")
	}
}
