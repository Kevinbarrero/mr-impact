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
		{"closure-arg-with-loop", "r.Handle(func() { for {} })", false},
		{"closure-tx-loop", "return s.store.InTx(ctx, func(q Store) error { for _, x := range xs { _ = x }; return nil })", false},
		{"closure-handler", "r.Handle(\"/x\", func(w W, r R) { h.Serve(w, r) })", true},
		{"closure-plumbing", "g.Go(func() error { return work(c) }); return g.Wait()", true},
		{"closure-arith", "r.Handle(func() int { return a + b })", false},
		{"closure-only-build", "return func() { g() }", true},
		{"composite", "return &S{A: a, B: []int{1}}", true},
		{"var-decl", "var s S; s.X = f(); return s", true},
		{"nil-on-left", "g(); if nil != err { return err }", true},

		{"plumb-call", "return NewS(&cfg, Opts{A: a.B})", true},
		{"plumb-composite-use", "s := S{}; s.Use(f()); return &s", true},
		{"plumb-conversion", "return T(x)", true},

		{"arith-return", "return a + b", false},
		{"arith-assign", "x := a * 2; return x", false},
		{"op-assign", "x := f(); x += 1; return x", false},
		{"shift-assign", "x := f(); x <<= 2; return x", false},
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
			if got := isWiring(fd, fileImports(f)); got != c.want {
				t.Errorf("isWiring = %v, want %v", got, c.want)
			}
		})
	}
}

// A function taking a context.Context runs per request, so it is never
// wiring, whatever its body.
func TestIsWiring_ContextParam(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"ctx", "import \"context\"\nfunc f(ctx context.Context, q Q) error { if err := q.Debit(ctx); err != nil { return err }; return nil }", false},
		{"ctx-aliased", "import c \"context\"\nfunc f(ctx c.Context, q Q) error { if err := q.Debit(ctx); err != nil { return err }; return nil }", false},
		{"ctx-grouped-param", "import \"context\"\nfunc f(a, ctx context.Context) error { return g(ctx) }", false},
		{"ctx-method", "import \"context\"\nfunc (s *S) f(ctx context.Context) error { return s.q.Debit(ctx) }", false},
		{"ctx-pointer", "import \"context\"\nfunc f(ctx *context.Context) error { return g(ctx) }", true},
		{"other-context-pkg", "import \"example.com/context\"\nfunc f(ctx context.Context) error { return g(ctx) }", true},
		{"dot-import", "import . \"context\"\nfunc f(ctx Context) error { return g(ctx) }", true},
		{"no-import", "func f(ctx context.Context) error { return g(ctx) }", true},
		{"constructor", "import \"context\"\nfunc NewS(a A) *S { return &S{a: a} }", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := parser.ParseFile(token.NewFileSet(), "p.go", "package p\n"+c.src+"\n", 0)
			if err != nil {
				t.Fatal(err)
			}
			fd := f.Decls[len(f.Decls)-1].(*ast.FuncDecl)
			if got := isWiring(fd, fileImports(f)); got != c.want {
				t.Errorf("isWiring = %v, want %v", got, c.want)
			}
		})
	}
}

// make and new take a type argument, which expr must accept as plumbing: a
// constructor that only builds its struct is wiring.
func TestWiring_ConstructorMakeNew(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"make-new-builds", "return &svc{m: make(map[string]int), s: make([]int, 0, n), c: make(chan int), mu: new(sync.Mutex), f: fn}", true},
		{"make-slice-len", "return &S{xs: make([]int, n)}", true},
		{"make-nested-map", "return &S{m: make(map[string][]chan<- int)}", true},
		{"make-func-type", "return &S{f: make(chan func(int) error)}", true},
		{"make-arith-arg", "return &S{xs: make([]int, n+1)}", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isWiring(parseWiringFunc(t, c.body), nil); got != c.want {
				t.Errorf("isWiring = %v, want %v", got, c.want)
			}
		})
	}
}

// A generic type instantiation as a make/new type argument is a type too.
// s[i] stays a value index and is not wiring.
func TestWiring_GenericMake(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"make-index-list", "return &S{m: make(Map[K, V])}", true},
		{"make-index", "return &S{b: new(Box[int])}", true},
		{"make-nested-instantiation", "return &S{m: make(map[string]Box[int])}", true},
		{"make-pointer-instantiation", "return &S{m: make(map[string]*Box[K, V])}", true},
		{"index-value", "return s[i]", false},
		{"index-value-in-make", "return &S{m: make(map[string]int, s[i])}", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isWiring(parseWiringFunc(t, c.body), nil); got != c.want {
				t.Errorf("isWiring = %v, want %v", got, c.want)
			}
		})
	}
}

// Building a map inside a constructor is wiring, but logic around it is not.
func TestWiring_ConstructorWithIfLogic(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"if-cond", "m := make(map[string]int); if n > 0 { m[\"a\"] = 1 }; return &S{m: m}"},
		{"range-fill", "m := make(map[string]int); for k := range ks { m[k] = 1 }; return &S{m: m}"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isWiring(parseWiringFunc(t, c.body), nil); got {
				t.Errorf("isWiring = true, want false")
			}
		})
	}
}

// parseWiringFunc parses body as the body of func f() and returns it.
func parseWiringFunc(t *testing.T, body string) *ast.FuncDecl {
	t.Helper()
	src := "package p\nfunc f() {\n" + body + "\n}\n"
	f, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f.Decls[0].(*ast.FuncDecl)
}
