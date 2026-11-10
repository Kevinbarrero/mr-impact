package symbols

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/diff"
)

type files map[string]string

func (f files) src(p string) ([]byte, error) {
	s, ok := f[p]
	if !ok {
		return nil, fmt.Errorf("%s: %w", p, fs.ErrNotExist)
	}
	return []byte(s), nil
}

func mod(path string, added, deleted []int) diff.FileChange {
	return diff.FileChange{Path: path, OldPath: path, Status: diff.Modified, AddedLines: added, DeletedLines: deleted}
}

func run(t *testing.T, changes []diff.FileChange, head, base files) Result {
	t.Helper()
	r, err := Find(changes, head.src, base.src)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

const pricingBase = `package pricing

type Calculator struct{}

func (c *Calculator) Total(a int) int {
	x := a
	return x
}

func other() {}
`

func TestBodyChange(t *testing.T) {
	head := strings.Replace(pricingBase, "x := a\n", "x := a + 1\n", 1)
	r := run(t, []diff.FileChange{mod("pricing/p.go", []int{6}, []int{6})},
		files{"pricing/p.go": head}, files{"pricing/p.go": pricingBase})
	if len(r.Funcs) != 1 {
		t.Fatalf("%+v", r.Funcs)
	}
	f := r.Funcs[0]
	if f.Kind != Body || f.Key != "pricing.Calculator.Total" || f.StartLine != 5 || f.File != "pricing/p.go" || len(f.AddedLines) != 1 || f.AddedLines[0] != 6 {
		t.Errorf("%+v", f)
	}
	if f.Display != "pricing.(*Calculator).Total" || !f.Exported {
		t.Errorf("%+v", f)
	}
}

func TestSignatureChange(t *testing.T) {
	head := strings.Replace(pricingBase, "Total(a int)", "Total(a, b int)", 1)
	r := run(t, []diff.FileChange{mod("pricing/p.go", []int{5}, []int{5})},
		files{"pricing/p.go": head}, files{"pricing/p.go": pricingBase})
	if len(r.Funcs) != 1 || r.Funcs[0].Kind != Signature {
		t.Fatalf("%+v", r.Funcs)
	}
}

// Parameter and result names are not part of the API: renaming them is a body
// change, while any change to types (params, results, variadic, receiver, type
// parameters) is a signature change.
func TestSignatureComparesTypesOnly(t *testing.T) {
	const base = `package p

type L[T any] struct{}

func (l *L[T]) M(a int, rest ...string) (n int, err error) { return 0, nil }

func G[T any](x T) {}

func H(a, b int) {}
`
	cases := []struct {
		name, old, new string
		want           Kind
	}{
		{"param rename", "M(a int,", "M(b int,", Body},
		{"variadic rename", "rest ...string", "more ...string", Body},
		{"result rename", "(n int, err error)", "(count int, e error)", Body},
		{"named to unnamed results", "(n int, err error)", "(int, error)", Body},
		{"receiver name", "(l *L[T])", "(r *L[T])", Body},
		{"ungrouped params", "H(a, b int)", "H(a int, b int)", Body},
		{"grouped param type", "H(a, b int)", "H(a, b int64)", Signature},
		{"param type", "M(a int,", "M(a int64,", Signature},
		{"variadic to slice", "rest ...string", "rest []string", Signature},
		{"added result", "(n int, err error)", "(n int, ok bool, err error)", Signature},
		{"result type", "(n int, err error)", "(n int64, err error)", Signature},
		{"receiver pointerness", "(l *L[T])", "(l L[T])", Signature},
		{"type param constraint", "G[T any]", "G[T comparable]", Signature},
		{"type param name", "G[T any](x T)", "G[U any](x U)", Signature},
		{"param order", "M(a int, rest ...string)", "M(rest string, a ...int)", Signature},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			head := strings.Replace(base, c.old, c.new, 1)
			if head == base {
				t.Fatalf("replacement %q not found", c.old)
			}
			line := 5
			switch c.old[0] {
			case 'G':
				line = 7
			case 'H':
				line = 9
			}
			r := run(t, []diff.FileChange{mod("p/p.go", []int{line}, []int{line})},
				files{"p/p.go": head}, files{"p/p.go": base})
			if len(r.Funcs) != 1 || r.Funcs[0].Kind != c.want {
				t.Fatalf("%+v", r.Funcs)
			}
		})
	}
}

func TestNewFunc(t *testing.T) {
	head := pricingBase + "\nfunc added() {}\n"
	r := run(t, []diff.FileChange{mod("pricing/p.go", []int{11, 12}, nil)},
		files{"pricing/p.go": head}, files{"pricing/p.go": pricingBase})
	if len(r.Funcs) != 1 || r.Funcs[0].Kind != New || r.Funcs[0].Display != "pricing.added" || r.Funcs[0].Exported {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestDeletedFuncInModifiedFile(t *testing.T) {
	head := strings.Replace(pricingBase, "func other() {}\n", "", 1)
	r := run(t, []diff.FileChange{mod("pricing/p.go", nil, []int{10})},
		files{"pricing/p.go": head}, files{"pricing/p.go": pricingBase})
	if len(r.Funcs) != 1 || r.Funcs[0].Kind != Deleted || r.Funcs[0].Key != "pricing.other" || r.Funcs[0].StartLine != 10 {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestDeletedFile(t *testing.T) {
	ch := diff.FileChange{OldPath: "pricing/p.go", Status: diff.Deleted, DeletedLines: []int{1, 2}}
	r := run(t, []diff.FileChange{ch}, files{}, files{"pricing/p.go": pricingBase})
	if len(r.Funcs) != 2 {
		t.Fatalf("%+v", r.Funcs)
	}
	for _, f := range r.Funcs {
		if f.Kind != Deleted || f.File != "pricing/p.go" {
			t.Errorf("%+v", f)
		}
	}
}

func TestDeletionOnlyBodyChange(t *testing.T) {
	head := strings.Replace(pricingBase, "\tx := a\n\treturn x", "\treturn a", 1)
	r := run(t, []diff.FileChange{mod("pricing/p.go", nil, []int{6})},
		files{"pricing/p.go": head}, files{"pricing/p.go": pricingBase})
	if len(r.Funcs) != 1 || r.Funcs[0].Kind != Body || len(r.Funcs[0].AddedLines) != 0 {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestMethodDisplay(t *testing.T) {
	src := "package list\n\ntype List[T any] struct{}\n\nfunc (l *List[T]) Push(v T) {\n}\n\nfunc (l List[T]) Len() int { return 0 }\n"
	r := run(t, []diff.FileChange{{Path: "list/l.go", Status: diff.Added, AddedLines: []int{1, 2, 3, 4, 5, 6, 7, 8}}},
		files{"list/l.go": src}, files{})
	if len(r.Funcs) != 2 {
		t.Fatalf("%+v", r.Funcs)
	}
	if f := r.Funcs[0]; f.Display != "list.(*List[T]).Push" || f.Key != "list.List.Push" {
		t.Errorf("%+v", f)
	}
	if f := r.Funcs[1]; f.Display != "list.List[T].Len" || f.Key != "list.List.Len" {
		t.Errorf("%+v", f)
	}
}

func TestExported(t *testing.T) {
	src := "package p\n\ntype calc struct{}\n\nfunc (c *calc) Total() {}\n"
	r := run(t, []diff.FileChange{{Path: "p.go", Status: diff.Added, AddedLines: []int{5}}},
		files{"p.go": src}, files{})
	if len(r.Funcs) != 1 || r.Funcs[0].Exported || r.Funcs[0].Key != "..calc.Total" {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestMultipleInit(t *testing.T) {
	base := "package p\n\nfunc init() {\n\ta()\n}\n\nfunc init() {\n\tb()\n}\n"
	head := strings.Replace(base, "b()", "c()", 1)
	r := run(t, []diff.FileChange{mod("p.go", []int{8}, []int{8})}, files{"p.go": head}, files{"p.go": base})
	if len(r.Funcs) != 1 || r.Funcs[0].Key != "..init@p.go#2" || r.Funcs[0].Kind != Body {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestClosureAttributedToParent(t *testing.T) {
	base := "package p\n\nfunc Run() {\n\tgo func() {\n\t\ta()\n\t}()\n}\n"
	head := strings.Replace(base, "a()", "b()", 1)
	r := run(t, []diff.FileChange{mod("p.go", []int{5}, []int{5})}, files{"p.go": head}, files{"p.go": base})
	if len(r.Funcs) != 1 || r.Funcs[0].Key != "..Run" {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestDeclChange(t *testing.T) {
	base := "package p\n\nimport \"os\"\n\ntype Foo struct{ A int }\n\nvar _ = os.Args\n"
	head := "package p\n\nimport \"os\"\nimport \"io\"\n\ntype Foo struct{ A, B int }\n\nvar _ = os.Args\nvar _ io.Reader\n"
	// HEAD: import "io" at 4 (not reported), Foo at 6, new var at 9; BASE Foo (line 5) dedups with HEAD Foo.
	r := run(t, []diff.FileChange{mod("p.go", []int{4, 6, 9}, []int{5})}, files{"p.go": head}, files{"p.go": base})
	var got []string
	for _, d := range r.Decls {
		got = append(got, d.What)
	}
	if len(r.Funcs) != 0 || strings.Join(got, "|") != "type Foo|var _" {
		t.Fatalf("%+v", r)
	}
	if r.Decls[0].Line != 6 || r.Decls[1].Line != 9 {
		t.Errorf("%+v", r.Decls)
	}
}

func TestDeletedFileDeclsNotReported(t *testing.T) {
	src := "package p\n\ntype Foo struct{}\n\nvar x = 1\n\nfunc F() {}\n"
	ch := diff.FileChange{OldPath: "p/p.go", Status: diff.Deleted, DeletedLines: []int{1, 2, 3, 4, 5, 6, 7}}
	r := run(t, []diff.FileChange{ch}, files{}, files{"p/p.go": src})
	if len(r.Decls) != 0 || len(r.DeletedFiles) != 1 || r.DeletedFiles[0] != "p/p.go" {
		t.Fatalf("Decls=%+v DeletedFiles=%v", r.Decls, r.DeletedFiles)
	}
}

func TestDeletedRefs(t *testing.T) {
	base := "package p\n\nimport sm \"ex.com/m/internal/status\"\n\nfunc F() int {\n\tx := helper()\n\treturn x + sm.Thing() + o.ts()\n}\n"
	head := "package p\n\nimport sm \"ex.com/m/internal/status\"\n\nfunc F() int {\n\treturn sm.Keep()\n}\n"
	r := run(t, []diff.FileChange{mod("p/p.go", []int{6}, []int{6, 7})}, files{"p/p.go": head}, files{"p/p.go": base})
	for _, want := range []Ref{
		{Dir: "p", Name: "helper"},
		{Import: "ex.com/m/internal/status", Name: "Thing"},
		{Dir: "p", Name: "ts"},
	} {
		if !r.DeletedRefs[want] {
			t.Errorf("DeletedRefs missing %+v: %v", want, r.DeletedRefs)
		}
	}
	for _, absent := range []Ref{{Dir: "p", Name: "F"}, {Dir: "p", Name: "Thing"}, {Import: "ex.com/m/internal/status", Name: "Keep"}} {
		if r.DeletedRefs[absent] {
			t.Errorf("DeletedRefs has %+v: %v", absent, r.DeletedRefs)
		}
	}
}

func TestBranchesRemoved(t *testing.T) {
	base := "package p\n\nfunc F(m int) int {\n\tswitch m {\n\tcase 1:\n\t\treturn 1\n\tcase 2:\n\t\treturn 2\n\t}\n\treturn 0\n}\n"
	head := "package p\n\nfunc F(m int) int {\n\tswitch m {\n\tcase 1:\n\t\treturn 1\n\t}\n\treturn 0\n}\n"
	r := run(t, []diff.FileChange{mod("p.go", nil, []int{7, 8})}, files{"p.go": head}, files{"p.go": base})
	if len(r.Funcs) != 1 || r.Funcs[0].Kind != Body || !r.Funcs[0].BranchesRemoved {
		t.Fatalf("%+v", r.Funcs)
	}
	// Adding a branch is not a removal.
	r = run(t, []diff.FileChange{mod("p.go", []int{7, 8}, nil)}, files{"p.go": base}, files{"p.go": head})
	if len(r.Funcs) != 1 || r.Funcs[0].BranchesRemoved {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestCrossDirRenameKeepsBaseDir(t *testing.T) {
	base := "package a\n\nfunc Keep() {}\n\nfunc Gone() {}\n"
	head := "package b\n\nfunc Keep() {}\n"
	ch := diff.FileChange{Path: "b/x.go", OldPath: "a/x.go", Status: diff.Renamed, DeletedLines: []int{5}}
	r := run(t, []diff.FileChange{ch}, files{"b/x.go": head}, files{"a/x.go": base})
	var del []string
	for _, f := range r.Funcs {
		if f.Kind == Deleted {
			del = append(del, f.Key)
		}
	}
	// Keep changes package (a -> b), so it is Deleted a.Keep + New b.Keep; Gone is a.Gone.
	if strings.Join(del, ",") != "a.Keep,a.Gone" {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestFuncMovedBetweenFiles(t *testing.T) {
	a := "package p\n\nfunc Keep() {}\n\nfunc Moved() {\n\tx()\n}\n"
	b := "package p\n\nfunc Other() {}\n"
	aHead := "package p\n\nfunc Keep() {}\n"
	bHead := b + "\nfunc Moved() {\n\tx()\n}\n"
	r := run(t, []diff.FileChange{
		mod("p/a.go", nil, []int{4, 5, 6, 7}),
		mod("p/b.go", []int{4, 5, 6, 7}, nil),
	}, files{"p/a.go": aHead, "p/b.go": bHead}, files{"p/a.go": a, "p/b.go": b})
	if len(r.Funcs) != 0 {
		t.Fatalf("pure move should report nothing: %+v", r.Funcs)
	}
	// Moved and edited: reported as a Body change, not New/Deleted.
	bHead2 := b + "\nfunc Moved() {\n\ty()\n}\n"
	r = run(t, []diff.FileChange{
		mod("p/a.go", nil, []int{4, 5, 6, 7}),
		mod("p/b.go", []int{4, 5, 6, 7}, nil),
	}, files{"p/a.go": aHead, "p/b.go": bHead2}, files{"p/a.go": a, "p/b.go": b})
	if len(r.Funcs) != 1 || r.Funcs[0].Kind != Body || r.Funcs[0].File != "p/b.go" {
		t.Fatalf("moved+edited: %+v", r.Funcs)
	}
}

func TestSameNameInBuildTaggedFiles(t *testing.T) {
	lin := "//go:build linux\n\npackage p\n\nfunc Foo() { a() }\n"
	dar := "//go:build darwin\n\npackage p\n\nfunc Foo() { a() }\n"
	linHead := strings.Replace(lin, "a()", "b()", 1)
	r := run(t, []diff.FileChange{mod("p/foo_linux.go", []int{5}, []int{5})},
		files{"p/foo_linux.go": linHead, "p/foo_darwin.go": dar},
		files{"p/foo_linux.go": lin, "p/foo_darwin.go": dar})
	if len(r.Funcs) != 1 || r.Funcs[0].File != "p/foo_linux.go" || r.Funcs[0].Kind != Body {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestTestFilesAndParseError(t *testing.T) {
	r := run(t, []diff.FileChange{
		{Path: "a_test.go", OldPath: "a_test.go", Status: diff.Modified, IsTest: true, AddedLines: []int{1}},
		{OldPath: "b_test.go", Status: diff.Deleted, IsTest: true},
	}, files{}, files{})
	if len(r.Funcs) != 0 || len(r.TestsTouched) != 2 || r.TestsTouched[0] != "a_test.go" || r.TestsTouched[1] != "b_test.go" {
		t.Fatalf("%+v", r)
	}
	_, err := Find([]diff.FileChange{mod("bad.go", []int{1}, nil)}, files{"bad.go": "package {"}.src, files{"bad.go": "package p"}.src)
	if err == nil || !strings.Contains(err.Error(), "bad.go") {
		t.Fatalf("err = %v", err)
	}
}

func TestRemovedCases(t *testing.T) {
	base := "package p\n\nimport m \"ex.com/mod/model\"\n\nfunc F(x m.Mode, v any) {\n\tswitch x {\n\tcase m.A:\n\tcase m.B, Local:\n\tdefault:\n\t}\n\tswitch v.(type) {\n\tcase *m.T:\n\tcase string:\n\t}\n\tswitch y := 1; y {\n\tcase 1:\n\t}\n}\n"
	head := "package p\n\nimport m \"ex.com/mod/model\"\n\nfunc F(x m.Mode, v any) {\n\tswitch x {\n\tcase m.A:\n\tdefault:\n\t}\n\tswitch v.(type) {\n\tcase string:\n\t}\n}\n"
	r := run(t, []diff.FileChange{mod("p/p.go", nil, []int{8, 12, 15, 16, 17})}, files{"p/p.go": head}, files{"p/p.go": base})
	var got []string
	for _, c := range r.RemovedCases {
		got = append(got, fmt.Sprintf("%s %s %+v default=%v line=%d", c.Func, c.Case, c.Ref, c.HasDefault, c.Line))
	}
	want := []string{
		"p.F m.B {Dir: Import:ex.com/mod/model Name:B} default=true line=6",
		"p.F Local {Dir:p Import: Name:Local} default=true line=6",
		"p.F *m.T {Dir: Import:ex.com/mod/model Name:T} default=false line=10",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("RemovedCases =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestFind_SetsWiring(t *testing.T) {
	base := "package p\n\nfunc setup() *S {\n\ts := NewS()\n\treturn s\n}\n\nfunc logic(n int) int {\n\tif n > 0 {\n\t\treturn n\n\t}\n\treturn 0\n}\n"
	head := "package p\n\nfunc setup() *S {\n\ts := NewS()\n\ts.Init()\n\treturn s\n}\n\nfunc logic(n int) int {\n\tif n > 1 {\n\t\treturn n\n\t}\n\treturn 0\n}\n"
	r := run(t, []diff.FileChange{mod("p.go", []int{5, 10}, []int{9})}, files{"p.go": head}, files{"p.go": base})
	got := map[string]bool{}
	for _, f := range r.Funcs {
		got[f.Display] = f.Wiring
	}
	if len(got) != 2 || !got["p.setup"] || got["p.logic"] {
		t.Fatalf("%+v", r.Funcs)
	}
}
