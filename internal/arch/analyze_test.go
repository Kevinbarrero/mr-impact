package arch

import (
	"fmt"
	"testing"
)

func TestMetrics(t *testing.T) {
	g := Graph{
		Packages:   []string{"x", "y", "z", "w", "iso"},
		Edges:      []Edge{{From: "y", To: "x"}, {From: "z", To: "x"}, {From: "w", To: "x"}, {From: "x", To: "y"}},
		Named:      map[string]int{"x": 2},
		Interfaces: map[string]int{"x": 1},
	}
	m := Metrics(g)
	x := m["x"]
	if x.Ca != 3 || x.Ce != 1 || x.I != 0.25 || x.A != 0.5 || x.D != 0.25 {
		t.Fatalf("x: %+v", x)
	}
	iso := m["iso"]
	if iso.I != 0 || iso.A != 0 || iso.D != 1 {
		t.Fatalf("iso: %+v", iso)
	}
}

func layerModel() Model {
	return Model{Name: "custom", Layers: []Layer{
		{Name: "domain", Dirs: []string{"domain"}},
		{Name: "infra", Dirs: []string{"infra"}, MayImport: []string{"domain"}},
	}}
}

func g1(edges ...Edge) Graph {
	g := Graph{Module: "m", Named: map[string]int{}, Interfaces: map[string]int{}, Names: map[string]string{}}
	seen := map[string]bool{}
	for _, e := range edges {
		for _, p := range []string{e.From, e.To} {
			if !seen[p] {
				seen[p] = true
				g.Packages = append(g.Packages, p)
			}
		}
	}
	g.Edges = edges
	return g
}

func TestAnalyze_NewVsExisting(t *testing.T) {
	old := Edge{From: "m/domain/a", To: "m/infra/x", File: "domain/a/a.go", Line: 3}
	nw := Edge{From: "m/domain/b", To: "m/infra/y", File: "domain/b/b.go", Line: 4}
	base := g1(old)
	head := g1(old, nw)
	r := Analyze(layerModel(), &base, head, []string{"m/domain/b"})
	if r.BaseMissing || r.Existing != 1 || len(r.New) != 1 || r.New[0].Edge.To != "m/infra/y" {
		t.Fatalf("%+v", r)
	}
	if r.BaseConf == nil || r.HeadConf == nil || *r.BaseConf != 0 || *r.HeadConf != 0 {
		t.Fatalf("conf %v %v", r.BaseConf, r.HeadConf)
	}
	if r.Model != "custom" || r.Total != 4 || r.Classified != 4 {
		t.Fatalf("%+v", r)
	}
}

func TestAnalyze_BaseMissing(t *testing.T) {
	head := g1(Edge{From: "m/domain/a", To: "m/infra/x", File: "f.go", Line: 1})
	r := Analyze(layerModel(), nil, head, []string{"m/domain/a", "m/zzz"})
	if !r.BaseMissing || r.BaseConf != nil || r.New != nil || r.Existing != 0 || len(r.Coupling) != 0 {
		t.Fatalf("%+v", r)
	}
	if r.HeadConf == nil {
		t.Fatal("head conf")
	}
	if len(r.TouchedLayers) != 1 || r.TouchedLayers[0] != (PkgLayer{"m/domain/a", "domain"}) {
		t.Fatalf("layers %+v", r.TouchedLayers)
	}
}

func TestAnalyze_CouplingOnlyChangedTouched(t *testing.T) {
	base := g1(Edge{From: "m/domain/a", To: "m/domain/b"})
	head := g1(Edge{From: "m/domain/a", To: "m/domain/b"}, Edge{From: "m/domain/c", To: "m/domain/b"})
	r := Analyze(layerModel(), &base, head, []string{"m/domain/a", "m/domain/b", "m/domain/c"})
	// a unchanged; b Ca 1->2; c new (zero base): Ce 0->1.
	got := map[string]bool{}
	for _, c := range r.Coupling {
		got[c.Pkg] = true
	}
	if got["m/domain/a"] || !got["m/domain/b"] || !got["m/domain/c"] || len(r.Coupling) != 2 {
		t.Fatalf("%+v", r.Coupling)
	}
	// c is absent at base: marked new, base left zero; b existed.
	for _, c := range r.Coupling {
		if c.New != (c.Pkg == "m/domain/c") || (c.New && c.Base != (Metric{})) {
			t.Errorf("New flag: %+v", c)
		}
	}
	if r.CouplingTotal != 2 {
		t.Errorf("CouplingTotal = %d, want 2", r.CouplingTotal)
	}
	// untouched changed package is not listed.
	r = Analyze(layerModel(), &base, head, []string{"m/domain/a"})
	if len(r.Coupling) != 0 {
		t.Fatalf("%+v", r.Coupling)
	}
}

func TestAnalyze_CouplingSortAndCap(t *testing.T) {
	var touched []string
	base := g1()
	head := g1()
	for i := 0; i < 12; i++ {
		to := fmt.Sprintf("m/domain/p%02d", i)
		touched = append(touched, to)
		// p00 has more importers => bigger |ΔD| ordering varies
		for j := 0; j <= i%3; j++ {
			head.Edges = append(head.Edges, Edge{From: fmt.Sprintf("m/domain/s%02d_%d", i, j), To: to})
		}
	}
	seen := map[string]bool{}
	for _, e := range head.Edges {
		for _, p := range []string{e.From, e.To} {
			if !seen[p] {
				seen[p] = true
				head.Packages = append(head.Packages, p)
			}
		}
	}
	r := Analyze(layerModel(), &base, head, touched)
	if len(r.Coupling) != 10 || r.CouplingTotal != 12 {
		t.Fatalf("cap: %d of %d, want 10 of 12", len(r.Coupling), r.CouplingTotal)
	}
	abs := func(x float64) float64 {
		if x < 0 {
			return -x
		}
		return x
	}
	for i := 1; i < len(r.Coupling); i++ {
		a, b := r.Coupling[i-1], r.Coupling[i]
		da := abs(a.Head.D - a.Base.D)
		db := abs(b.Head.D - b.Base.D)
		if da < db || (da == db && a.Pkg > b.Pkg) {
			t.Fatalf("order at %d: %+v %+v", i, a, b)
		}
	}
}

func TestAnalyze_TouchedLayersCap(t *testing.T) {
	var touched []string
	for i := 0; i < 12; i++ {
		touched = append(touched, fmt.Sprintf("m/domain/p%02d", i))
	}
	r := Analyze(layerModel(), nil, g1(), touched)
	if len(r.TouchedLayers) != 10 || r.TouchedLayers[0].Pkg != "m/domain/p00" {
		t.Fatalf("%+v", r.TouchedLayers)
	}
}
