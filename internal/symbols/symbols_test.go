package symbols

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"gitlab.com/wh1t3hat/mrimpact/internal/diff"
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
	head := pricingBase[:len(pricingBase)-0]
	head = strings.Replace(head, "x := a\n", "x := a + 1\n", 1)
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
	if len(r.Funcs) != 1 || r.Funcs[0].Exported || r.Funcs[0].Key != "..calc.Total" && r.Funcs[0].Key != ".calc.Total" {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestMultipleInit(t *testing.T) {
	base := "package p\n\nfunc init() {\n\ta()\n}\n\nfunc init() {\n\tb()\n}\n"
	head := strings.Replace(base, "b()", "c()", 1)
	r := run(t, []diff.FileChange{mod("p.go", []int{8}, []int{8})}, files{"p.go": head}, files{"p.go": base})
	if len(r.Funcs) != 1 || !strings.HasSuffix(r.Funcs[0].Key, "#2") || r.Funcs[0].Kind != Body {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestClosureAttributedToParent(t *testing.T) {
	base := "package p\n\nfunc Run() {\n\tgo func() {\n\t\ta()\n\t}()\n}\n"
	head := strings.Replace(base, "a()", "b()", 1)
	r := run(t, []diff.FileChange{mod("p.go", []int{5}, []int{5})}, files{"p.go": head}, files{"p.go": base})
	if len(r.Funcs) != 1 || r.Funcs[0].Key != ".Run" && r.Funcs[0].Key != "..Run" {
		t.Fatalf("%+v", r.Funcs)
	}
}

func TestDeclChange(t *testing.T) {
	base := "package p\n\nimport \"os\"\n\ntype Foo struct{ A int }\n\nvar _ = os.Args\n"
	head := strings.Replace(strings.Replace(base, "\"os\"", "\"os\"\nimport \"io\"", 1), "A int", "A, B int", 1)
	head = strings.Replace(head, "var _ = os.Args", "var _ = os.Args\nvar _ io.Reader", 1)
	// import at 4, type at 7 (shifted by one)
	r := run(t, []diff.FileChange{mod("p.go", []int{4, 7}, []int{5})}, files{"p.go": head}, files{"p.go": base})
	if len(r.Funcs) != 0 || len(r.Decls) != 2 {
		t.Fatalf("%+v", r)
	}
	got := []string{r.Decls[0].What, r.Decls[1].What}
	if got[0] != "import" || got[1] != "type Foo" {
		t.Errorf("%v", got)
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
	for _, f := range r.Funcs {
		if f.Kind == New || f.Kind == Deleted {
			t.Errorf("unexpected %+v", f)
		}
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
