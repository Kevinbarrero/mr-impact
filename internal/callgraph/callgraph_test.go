package callgraph

import (
	"context"
	"errors"
	"go/ast"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"gitlab.com/wh1t3hat/mrimpact/internal/load"
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
