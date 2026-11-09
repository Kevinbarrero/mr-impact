package registrations

import (
	"context"
	"strings"
	"testing"

	"gitlab.com/wh1t3hat/mrimpact/internal/load"
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
