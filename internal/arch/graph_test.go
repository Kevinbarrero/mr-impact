package arch

import (
	"context"
	"testing"

	"gitlab.com/wh1t3hat/mrimpact/internal/load"
)

func loadFixture(t *testing.T) Graph {
	t.Helper()
	pkgs, rel, err := load.Syntax(context.Background(), "testdata/graph", nil)
	if err != nil {
		t.Fatal(err)
	}
	return FromPackages(pkgs, "example.com/g", rel)
}

func TestFromPackages_OneEdgePerPair(t *testing.T) {
	g := loadFixture(t)
	if len(g.Edges) != 1 {
		t.Fatalf("edges: %+v", g.Edges)
	}
	e := g.Edges[0]
	if e.From != "example.com/g/a" || e.To != "example.com/g/b" || e.File != "a/x.go" || e.Line != 3 {
		t.Fatalf("edge: %+v", e)
	}
	if g.Names["example.com/g/a"] != "a" {
		t.Fatalf("names: %v", g.Names)
	}
}

func TestFromPackages_TypeCounts(t *testing.T) {
	g := loadFixture(t)
	p := "example.com/g/t"
	if g.Named[p] != 4 || g.Interfaces[p] != 2 {
		t.Fatalf("named %d ifaces %d", g.Named[p], g.Interfaces[p])
	}
}
