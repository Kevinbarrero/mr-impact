package symbols

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestIsWiring(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"chain", "a := NewA(); b := NewB(a); b.Set(a); return b", true},
		{"err-return", "x, err := f(); if err != nil { return nil, err }; return x, nil", true},
		{"if-init", "if err := f(); err != nil { return err }; return nil", true},
		{"log-then-return", "if err != nil { log.Printf(\"x\"); return err }", true},
		{"fatal", "if err != nil { log.Fatal(err) }", true},
		{"exit", "if err != nil { os.Exit(1) }", true},
		{"panic", "if err != nil { panic(err) }", true},
		{"closure-arg-with-loop", "r.Handle(func() { for {} })", true},
		{"composite", "return &S{A: a, B: []int{1}}", true},
		{"var-decl", "var s S; s.X = f(); return s", true},
		{"nil-on-left", "g(); if nil != err { return err }", true},

		{"plumb-call", "return NewS(&cfg, Opts{A: a.B})", true},
		{"plumb-composite-use", "s := S{}; s.Use(f()); return &s", true},
		{"plumb-conversion", "return T(x)", true},

		{"arith-return", "return a + b", false},
		{"arith-assign", "x := a * 2; return x", false},
		{"index", "return s[i]", false},
		{"assert", "return v.(T)", false},
		{"not", "return !ok", false},
		{"recv", "c := <-ch; return c", false},
		{"no-call", "return x", false},
		{"return-nil", "return nil", false},
		{"cond", "if x > 0 { return }", false},
		{"else", "if err != nil { return err } else { g() }", false},
		{"for", "for i := 0; i < 3; i++ { g() }", false},
		{"range", "for range xs { g() }", false},
		{"switch", "switch x { case 1: g() }", false},
		{"typeswitch", "switch x.(type) { case int: g() }", false},
		{"select", "select { case <-c: g() }", false},
		{"go", "go g()", false},
		{"defer", "defer g()", false},
		{"if-init-call", "if f(); err != nil { return err }", false},
		{"log-and-continue", "if err != nil { log.Printf(\"x\") }", false},
		{"empty", "", false},
		{"label", "L: g()", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "package p\nfunc f() {\n" + c.body + "\n}\n"
			f, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
			if err != nil {
				t.Fatal(err)
			}
			fd := f.Decls[0].(*ast.FuncDecl)
			if got := isWiring(fd); got != c.want {
				t.Errorf("isWiring = %v, want %v", got, c.want)
			}
		})
	}
}
