package symbols

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

func TestBranchKeys(t *testing.T) {
	cases := []struct {
		name, body string
		want       []branchKey
	}{
		{"if", "if x > 0 {\n}", []branchKey{{"if x > 0", 4}}},
		{"init", "if err := f(); err != nil {\n}", []branchKey{{"if err := f(); err != nil", 4}}},
		{"else", "if a {\n} else {\n}", []branchKey{{"if a", 4}, {"else a", 5}}},
		{"else if", "if a {\n} else if b {\n}", []branchKey{{"if a", 4}, {"else a", 5}, {"if b", 5}}},
		{"switch", "switch v {\ncase 1, 2:\ndefault:\n}", []branchKey{{"case 1, 2", 5}, {"default", 6}}},
		{"type switch", "switch v.(type) {\ncase int:\n}", []branchKey{{"case int", 5}}},
		{"select", "select {\ncase <-ch:\n}", []branchKey{{"case <-ch", 5}}},
		{"return", "return a, b", []branchKey{{"return a, b", 4}}},
		{"bare return", "return", []branchKey{{"return", 4}}},
		{"closure", "func() {\nif a {\n}\n}()", []branchKey{{"if a", 5}}},
		{"multi-line", "if a &&\n\tb {\n}", []branchKey{{"if a && b", 4}}},
	}
	for _, c := range cases {
		src := "package p\n\nfunc F() {\n" + c.body + "\n}\n"
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p.go", src, 0)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := branchKeys(fset, f.Decls[0].(*ast.FuncDecl))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}
