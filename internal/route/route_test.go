package route

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

// renderLast parses src (a file body after the package clause), takes the
// last call named target inside the first function and renders it.
func renderLast(t *testing.T, src, target string) (string, bool) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if d, ok := d.(*ast.FuncDecl); ok {
			fn = d
			break
		}
	}
	var call *ast.CallExpr
	ast.Inspect(fn, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := c.Fun.(type) {
		case *ast.SelectorExpr:
			if fun.Sel.Name == target {
				call = c
			}
		case *ast.Ident:
			if fun.Name == target {
				call = c
			}
		}
		return true
	})
	if call == nil {
		t.Fatalf("no call to %s", target)
	}
	return Render(call, fn)
}

func TestRender(t *testing.T) {
	cases := []struct {
		name, src, target, want string
		ok                      bool
	}{
		{"GroupPost", `func setup(r *R) { api := r.Group("/trade"); api.POST("", h) }`, "POST", "POST /trade", true},
		{"GroupGetParam", `func setup(r *R) { api := r.Group("/trade"); api.GET("/:id", h) }`, "GET", "GET /trade/:id", true},
		{"Nested", `func setup(r *R) { v1 := r.Group("/api"); t := v1.Group("/trade"); t.GET("/x", h) }`, "GET", "GET /api/trade/x", true},
		{"ChainedReceiver", `func setup(r *R) { r.Group("/a").GET("/x", h) }`, "GET", "GET /a/x", true},
		{"ChiRoute", `func setup(r chi.Router) { r.Route("/trade", func(r chi.Router) { r.Get("/x", h) }) }`, "Get", "GET /trade/x", true},
		{"ChiRouteNested", `func setup(r chi.Router) { r.Route("/a", func(r chi.Router) { r.Route("/b", func(q chi.Router) { q.Post("/x", h) }) }) }`, "Post", "POST /a/b/x", true},
		{"GorillaPathPrefix", `func setup(r *mux.Router) { s := r.PathPrefix("/api").Subrouter(); s.HandleFunc("/x", h) }`, "HandleFunc", "/api/x", true},
		{"HandleFunc", `func setup() { http.HandleFunc("/x", h) }`, "HandleFunc", "/x", true},
		{"HandleFuncMethod", `func setup() { http.HandleFunc("GET /x", h) }`, "HandleFunc", "GET /x", true},
		{"Handle", `func setup(m *M) { m.Handle("/x", h) }`, "Handle", "/x", true},
		{"Match", `func setup(m *M) { m.Match("/x", h) }`, "Match", "/x", true},
		{"Any", `func setup(r *R) { r.Any("/p", h) }`, "Any", "ANY /p", true},
		{"LowercaseVerb", `func setup(a *fiber.App) { a.Get("/p", h) }`, "Get", "GET /p", true},
		{"RootSlash", `func setup(r *R) { r.GET("/", h) }`, "GET", "GET /", true},
		{"EmptyNoPrefixFallsBack", `func setup(r *R) { r.GET("", h) }`, "GET", "", false},
		{"EmptyUnderGroup", `func setup(r *R) { g := r.Group("/a"); g.GET("", h) }`, "GET", "GET /a", true},
		{"TrailingSlash", `func setup(r *R) { g := r.Group("/a"); g.GET("/b/", h) }`, "GET", "GET /a/b/", true},
		{"NonLiteralPath", `func setup(r *R) { r.GET(p, h) }`, "GET", "", false},
		{"NotVerb", `func setup(e *E) { e.SubscribeAsync("t", h) }`, "SubscribeAsync", "", false},
		{"PrefixAssignedTwiceFallsBack", `func setup(r *R) { g := r.Group("/a"); g = r.Group("/b"); g.GET("/x", h) }`, "GET", "", false},
		{"AssignedInClosureCounts", `func setup(r *R) { g := r.Group("/a"); func() { g = r.Group("/b") }(); g.GET("/x", h) }`, "GET", "", false},
		{"RootParameterNoPrefix", `func setup(g *Group) { g.GET("/x", h) }`, "GET", "GET /x", true},
		{"RootCallResultNoPrefix", `func setup() { g := newGroup(); g.GET("/x", h) }`, "GET", "GET /x", true},
		{"FiveLevelsTooDeep", `func setup(r *R) { a := r.Group("/1"); b := a.Group("/2"); c := b.Group("/3"); d := c.Group("/4"); e := d.Group("/5"); e.GET("/x", h) }`, "GET", "", false},
		{"GroupNonLiteralChained", `func setup(r *R) { r.Group(p).GET("/x", h) }`, "GET", "", false},
		{"GroupNonLiteralLocal", `func setup(r *R) { g := r.Group(prefix); g.GET("/x", h) }`, "GET", "", false},
		{"UseInChain", `func setup(r *R) { r.Group("/a").Use(mw).GET("/x", h) }`, "GET", "", false},
		{"UseOnLocalAssigned", `func setup(r *R) { g := r.Group("/a"); h2 := g.Use(mw); h2.GET("/x", h) }`, "GET", "", false},
		{"PackageCallRoot", `func setup() { r := gin.Default(); r.GET("/x", h) }`, "GET", "GET /x", true},
		{"GinHandleMethod", `func setup(r *R) { g := r.Group("/a"); g.Handle("get", "/x", h) }`, "Handle", "GET /a/x", true},
		{"GinHandleNonLiteralPath", `func setup(r *R) { r.Handle("GET", p, h) }`, "Handle", "", false},
		{"HandleFuncNotPath", `func setup(m *M) { m.HandleFunc("email:deliver", h) }`, "HandleFunc", "", false},
		{"HandleTopicNotPath", `func setup(bus *B) { bus.Handle("user.created", h) }`, "Handle", "", false},
		{"GetKeyNotPath", `func setup(cache *C) { cache.Get("key", f) }`, "Get", "", false},
		{"GroupedKeyNotPath", `func setup(r *R) { g := r.Group("/a"); g.GET("x", h) }`, "GET", "", false},
		{"EmptyUnderNonPathPrefix", `func setup(r *R) { g := r.Group("jobs"); g.GET("", h) }`, "GET", "", false},
		{"GinHandleNotPath", `func setup(r *R) { r.Handle("GET", "x", h) }`, "Handle", "", false},
		{"AliasKeepsPrefix", `func setup(r *R) { g := r.Group("/a"); h2 := g; h2.GET("/x", h) }`, "GET", "GET /a/x", true},
		{"AliasCountsTowardCap", `func setup(r *R) { a := r.Group("/1"); b := a.Group("/2"); c := b.Group("/3"); d := c.Group("/4"); e := d; e.GET("/x", h) }`, "GET", "", false},
		{"AliasOfTwiceAssignedFallsBack", `func setup(r *R) { g := r.Group("/a"); g = r.Group("/b"); h2 := g; h2.GET("/x", h) }`, "GET", "", false},
		{"FourLevels", `func setup(r *R) { a := r.Group("/1"); b := a.Group("/2"); c := b.Group("/3"); d := c.Group("/4"); d.GET("/x", h) }`, "GET", "GET /1/2/3/4/x", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := renderLast(t, c.src, c.target)
			if got != c.want || ok != c.ok {
				t.Fatalf("Render = %q, %v; want %q, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestLiterals(t *testing.T) {
	f, err := parser.ParseExpr(`x.M("a", y, "b\n", 3, ` + "`c`" + `)`)
	if err != nil {
		t.Fatal(err)
	}
	got := Literals(f.(*ast.CallExpr))
	want := []string{"a", "b\n", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Literals = %q, want %q", got, want)
	}
}
