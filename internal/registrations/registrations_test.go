package registrations

import (
	"context"
	"strings"
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/load"
)

func find(t *testing.T, dir string) []Registration {
	t.Helper()
	pkgs, rel, err := load.Syntax(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return Find(pkgs, "example.com/reg", rel)
}

func describe(rs []Registration) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Call+" -> "+r.Handler)
	}
	return strings.Join(out, "\n")
}

func TestFind(t *testing.T) {
	got := describe(find(t, "testdata/base"))
	want := strings.Join([]string{
		`GET("/list", …) -> web.(*ctl).list`,
		`GET("/delete", …) -> web.(*ctl).delete`,
		`GET("/show", …) -> web.show`,
	}, "\n")
	if got != want {
		t.Errorf("Find =\n%s\nwant\n%s", got, want)
	}
}

func TestDiff(t *testing.T) {
	removed, added := Diff(find(t, "testdata/base"), find(t, "testdata/head"))
	if got := describe(removed); got != "GET(\"/delete\", …) -> web.(*ctl).delete\nGET(\"/show\", …) -> web.show" {
		t.Errorf("removed =\n%s", got)
	}
	if got := describe(added); got != "GET(\"/show\", …) -> web.health\nGET(\"/healthz\", …) -> web.health" {
		t.Errorf("added =\n%s", got)
	}
	if len(added) == 2 && (added[1].File != "web/web.go" || added[1].Line != 22) {
		t.Errorf("added position = %s:%d", added[1].File, added[1].Line)
	}
}

func TestPair(t *testing.T) {
	removed, added := Diff(find(t, "testdata/base"), find(t, "testdata/head"))
	rewired, gone, fresh := Pair(removed, added)
	if len(rewired) != 1 || rewired[0].From.Handler != "web.show" || rewired[0].To.Handler != "web.health" ||
		rewired[0].To.Call != `GET("/show", …)` {
		t.Errorf("rewired = %+v", rewired)
	}
	if got := describe(gone); got != `GET("/delete", …) -> web.(*ctl).delete` {
		t.Errorf("gone =\n%s", got)
	}
	if got := describe(fresh); got != `GET("/healthz", …) -> web.health` {
		t.Errorf("fresh =\n%s", got)
	}
}

// reg builds a registration with the identity and attributes Find would give.
func reg(callee string, lits []string, handler string, attrs ...string) Registration {
	r := Registration{Callee: callee, Handler: handler, Call: callee + "(…)"}
	r.identity = identityOf(callee, "", lits, handler)
	r.attrs = attrs
	r.key = r.identity + "\x00" + strings.Join(attrs, "\x00")
	r.slot = callee + strings.Join(lits, "") + strings.Join(attrs, "")
	return r
}

func TestPairChanged_MiddlewareAdded(t *testing.T) {
	removed := []Registration{reg("GET", []string{"/:id"}, "trade.(*controller).getStatus")}
	added := []Registration{reg("GET", []string{"/:id"}, "trade.(*controller).getStatus", "c.signatureMiddleware")}
	changed, gone, fresh := PairChanged(removed, added)
	if len(changed) != 1 || len(gone) != 0 || len(fresh) != 0 {
		t.Fatalf("changed=%+v gone=%+v fresh=%+v", changed, gone, fresh)
	}
	c := changed[0]
	if strings.Join(c.Added, ",") != "c.signatureMiddleware" || len(c.Removed) != 0 {
		t.Errorf("Added=%q Removed=%q", c.Added, c.Removed)
	}
	if c.From.Handler != removed[0].Handler || c.To.Handler != added[0].Handler {
		t.Errorf("From/To = %+v", c)
	}
}

func TestPairChanged_MiddlewareRemoved(t *testing.T) {
	removed := []Registration{reg("GET", []string{"/:id"}, "h.H", "a", "b")}
	added := []Registration{reg("GET", []string{"/:id"}, "h.H", "b")}
	changed, gone, fresh := PairChanged(removed, added)
	if len(changed) != 1 || len(gone) != 0 || len(fresh) != 0 {
		t.Fatalf("changed=%+v gone=%+v fresh=%+v", changed, gone, fresh)
	}
	if len(changed[0].Added) != 0 || strings.Join(changed[0].Removed, ",") != "a" {
		t.Errorf("Added=%q Removed=%q", changed[0].Added, changed[0].Removed)
	}
}

func TestPairChanged_Both(t *testing.T) {
	// Multiset difference: one "m" of two is gone, "x" and "y" are new.
	removed := []Registration{reg("POST", []string{""}, "h.H", "m", "a", "m")}
	added := []Registration{reg("POST", []string{""}, "h.H", "y", "m", "x")}
	changed, _, _ := PairChanged(removed, added)
	if len(changed) != 1 {
		t.Fatalf("changed = %+v", changed)
	}
	if got := strings.Join(changed[0].Added, ","); got != "y,x" {
		t.Errorf("Added = %q, want head order y,x", got)
	}
	if got := strings.Join(changed[0].Removed, ","); got != "a,m" {
		t.Errorf("Removed = %q, want base order a,m", got)
	}
}

func TestPairChanged_PathChangedNotPaired(t *testing.T) {
	removed := []Registration{reg("GET", []string{"/list"}, "h.H", "a")}
	added := []Registration{reg("GET", []string{"/all"}, "h.H", "a")}
	changed, gone, fresh := PairChanged(removed, added)
	if len(changed) != 0 || len(gone) != 1 || len(fresh) != 1 {
		t.Errorf("changed=%+v gone=%+v fresh=%+v", changed, gone, fresh)
	}
}

func TestPairChanged_AmbiguousNotPaired(t *testing.T) {
	removed := []Registration{reg("GET", []string{"/x"}, "h.H", "a"), reg("GET", []string{"/x"}, "h.H", "b")}
	added := []Registration{reg("GET", []string{"/x"}, "h.H", "c"), reg("GET", []string{"/x"}, "h.H", "d")}
	changed, gone, fresh := PairChanged(removed, added)
	if len(changed) != 0 || len(gone) != 2 || len(fresh) != 2 {
		t.Errorf("changed=%+v gone=%+v fresh=%+v", changed, gone, fresh)
	}
	// One side ambiguous is still ambiguous.
	changed, gone, fresh = PairChanged(removed[:1], added)
	if len(changed) != 0 || len(gone) != 1 || len(fresh) != 2 {
		t.Errorf("one removed, two added: changed=%+v gone=%+v fresh=%+v", changed, gone, fresh)
	}
}

func TestPairChanged_ReorderOnlyNotPaired(t *testing.T) {
	// The same attributes in another order: no attribute to list, keep today's output.
	removed := []Registration{reg("GET", []string{"/x"}, "h.H", "a", "b")}
	added := []Registration{reg("GET", []string{"/x"}, "h.H", "b", "a")}
	changed, gone, fresh := PairChanged(removed, added)
	if len(changed) != 0 || len(gone) != 1 || len(fresh) != 1 {
		t.Errorf("changed=%+v gone=%+v fresh=%+v", changed, gone, fresh)
	}
}

func TestPair_HandlerChangeStillRewired(t *testing.T) {
	removed, added := Diff(find(t, "testdata/base"), find(t, "testdata/head"))
	rewired, gone, fresh := Pair(removed, added)
	changed, gone, fresh := PairChanged(gone, fresh)
	if len(rewired) != 1 || rewired[0].From.Handler != "web.show" || rewired[0].To.Handler != "web.health" {
		t.Errorf("rewired = %+v", rewired)
	}
	if len(changed) != 0 || len(gone) != 1 || len(fresh) != 1 {
		t.Errorf("changed=%+v gone=%+v fresh=%+v", changed, gone, fresh)
	}
}

func TestFind_RouteAndChanged(t *testing.T) {
	base, head := find(t, "testdata/mw/base"), find(t, "testdata/mw/head")
	var routes []string
	for _, r := range head {
		routes = append(routes, r.Route+" -> "+r.Handler)
	}
	wantRoutes := strings.Join([]string{
		"GET /trade/:id -> trade.(*controller).getStatus",
		"GET /trade/:id -> trade.(*controller).signatureMiddleware",
		"POST /trade -> trade.(*controller).create",
		"GET /trade/all -> trade.(*controller).list",
	}, "\n")
	if got := strings.Join(routes, "\n"); got != wantRoutes {
		t.Errorf("head routes =\n%s\nwant\n%s", got, wantRoutes)
	}
	_, changed, gone, fresh := Compare(base, head)
	var got []string
	for _, c := range changed {
		got = append(got, c.To.Route+" "+c.To.Handler+" +"+strings.Join(c.Added, ",")+" -"+strings.Join(c.Removed, ","))
	}
	want := "GET /trade/:id trade.(*controller).getStatus +c.signatureMiddleware -\n" +
		"POST /trade trade.(*controller).create +c.limit -c.auth"
	if g := strings.Join(got, "\n"); g != want {
		t.Errorf("changed =\n%s\nwant\n%s", g, want)
	}
	if len(changed) == 2 && (changed[0].To.File != "trade/controller.go" || changed[0].To.Line != 21) {
		t.Errorf("changed position = %s:%d, want the head registration's", changed[0].To.File, changed[0].To.Line)
	}
	if g := describe(gone); g != `GET("/list", c.auth, …) -> trade.(*controller).list` {
		t.Errorf("gone =\n%s", g)
	}
	// The head registration of signatureMiddleware at the changed call is the
	// same edit: not listed again as added.
	if g := describe(fresh); g != `GET("/all", c.auth, …) -> trade.(*controller).list` {
		t.Errorf("fresh =\n%s", g)
	}
}

func TestCompare_CrossRegistrationsOfOneCall(t *testing.T) {
	// Each call passes two module functions; each edit is one rewire, not
	// also a change of the other registration of the same call.
	rewired, changed, gone, fresh := Compare(find(t, "testdata/cross/base"), find(t, "testdata/cross/head"))
	var got []string
	for _, r := range rewired {
		got = append(got, r.To.Route+" "+r.From.Handler+" -> "+r.To.Handler)
	}
	want := "GET /trade/x trade.(*controller).mwA -> trade.(*controller).mwB\n" +
		"GET /trade/y trade.(*controller).h -> trade.(*controller).h2"
	if g := strings.Join(got, "\n"); g != want {
		t.Errorf("rewired =\n%s\nwant\n%s", g, want)
	}
	if len(changed) != 0 || len(gone) != 0 || len(fresh) != 0 {
		t.Errorf("changed=%+v gone=%+v fresh=%+v", changed, gone, fresh)
	}
}

func TestCompare_MiddlewareRemovedOneLine(t *testing.T) {
	rewired, changed, gone, fresh := Compare(find(t, "testdata/mwgone/base"), find(t, "testdata/mwgone/head"))
	if len(rewired) != 0 || len(gone) != 0 || len(fresh) != 0 {
		t.Errorf("rewired=%+v gone=%+v fresh=%+v", rewired, gone, fresh)
	}
	if len(changed) != 1 || changed[0].To.Handler != "trade.(*controller).h" ||
		len(changed[0].Added) != 0 || strings.Join(changed[0].Removed, ",") != "c.mw" {
		t.Errorf("changed = %+v", changed)
	}
}

func TestCompare_ChangedSortedByHeadPosition(t *testing.T) {
	at := func(r Registration, file string, line int) Registration { r.File, r.Line = file, line; return r }
	removed := []Registration{
		at(reg("GET", []string{"/b"}, "h.H", "m"), "a.go", 1),
		at(reg("GET", []string{"/a"}, "h.H", "m"), "a.go", 2),
	}
	added := []Registration{
		at(reg("GET", []string{"/a"}, "h.H"), "a.go", 5),
		at(reg("GET", []string{"/b"}, "h.H"), "a.go", 9),
	}
	_, changed, _, _ := Compare(removed, added)
	if len(changed) != 2 || changed[0].To.Line != 5 || changed[1].To.Line != 9 {
		t.Errorf("changed not in head order: %+v", changed)
	}
}

func TestFind_PackageLevelFuncLitIsItsOwnScope(t *testing.T) {
	var got []string
	for _, r := range find(t, "testdata/pkglevel") {
		got = append(got, r.Route)
	}
	if g := strings.Join(got, ","); g != "GET /a/q,GET /b/q" {
		t.Errorf("routes = %q, want GET /a/q,GET /b/q", g)
	}
}

func TestCompare_RouteMovedNotPaired(t *testing.T) {
	// admin.GET("/x", auth, h) -> public.GET("/x", h): the same literal under
	// another prefix is another route, not a middleware change.
	_, changed, gone, fresh := Compare(find(t, "testdata/moved/base"), find(t, "testdata/moved/head"))
	if len(changed) != 0 {
		t.Errorf("changed = %+v, want none", changed)
	}
	if len(gone) != 1 || gone[0].Route != "GET /admin/x" || len(fresh) != 1 || fresh[0].Route != "GET /public/x" {
		t.Errorf("gone=%+v fresh=%+v", gone, fresh)
	}
}
