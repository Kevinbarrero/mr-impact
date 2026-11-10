package arch

import "testing"

func customModel() Model {
	return Model{
		Name: "custom",
		Layers: []Layer{
			{Name: "domain", Dirs: []string{"model", "domain"}},
			{Name: "adapters", Dirs: []string{"adapters"}, Isolated: true, MayImport: []string{"domain"}},
			{Name: "application", Dirs: []string{"app"}, MayImport: []string{"domain"}},
		},
	}
}

func TestClassify_ShallowestSegmentWins(t *testing.T) {
	l, ok := customModel().Classify("m", "m/internal/adapters/http/model")
	if !ok || l != "adapters" {
		t.Fatalf("got %q %v", l, ok)
	}
}

func TestClassify_ModuleSegmentsIgnored(t *testing.T) {
	if l, ok := customModel().Classify("example.com/app", "example.com/app/internal/x"); ok {
		t.Fatalf("got %q", l)
	}
}

func TestClassify_GlobBeatsNames(t *testing.T) {
	m := customModel()
	m.Layers[0].Globs = []string{"internal/adapters/http/..."}
	l, ok := m.Classify("m", "m/internal/adapters/http/x")
	if !ok || l != "domain" {
		t.Fatalf("got %q %v", l, ok)
	}
}

func TestClassify_FirstGlobInOrder(t *testing.T) {
	m := customModel()
	m.Layers[1].Globs = []string{"x/..."}
	m.Layers[2].Globs = []string{"x/..."}
	if l, _ := m.Classify("m", "m/x/y"); l != "adapters" {
		t.Fatalf("got %q", l)
	}
}

func TestClassify_Entry(t *testing.T) {
	m := customModel()
	if l, ok := m.Classify("m", "m/cmd/server"); !ok || l != EntryLayer {
		t.Fatalf("cmd: %q %v", l, ok)
	}
	if l, ok := m.ClassifyNamed("m", "m/tools/gen", "main"); !ok || l != EntryLayer {
		t.Fatalf("main: %q %v", l, ok)
	}
	m.Layers[0].Globs = []string{"cmd/..."}
	if l, _ := m.Classify("m", "m/cmd/server"); l != "domain" {
		t.Fatalf("glob override: %q", l)
	}
}

func e(from, to string) Edge { return Edge{From: from, To: to, File: from + ".go", Line: 1} }

func TestJudge_AllowedForbiddenSameLayer(t *testing.T) {
	g := Graph{Module: "m", Edges: []Edge{
		e("m/app", "m/model"),   // allowed
		e("m/model", "m/app"),   // forbidden
		e("m/app/a", "m/app/b"), // same layer
	}}
	j, v := customModel().Judge(g)
	if j != 3 || len(v) != 1 || v[0].FromLayer != "domain" || v[0].ToLayer != "application" {
		t.Fatalf("%d %+v", j, v)
	}
}

func TestJudge_EntryRules(t *testing.T) {
	g := Graph{Module: "m", Edges: []Edge{
		e("m/cmd/x", "m/model"),
		e("m/model", "m/cmd/x"),
		e("m/cmd/x", "m/cmd/y"),
	}}
	j, v := customModel().Judge(g)
	if j != 3 || len(v) != 1 || v[0].ToLayer != EntryLayer {
		t.Fatalf("%d %+v", j, v)
	}
}

func TestJudge_NamesMainOutsideCmd(t *testing.T) {
	g := Graph{Module: "m", Names: map[string]string{"m/tools/gen": "main"},
		Edges: []Edge{e("m/tools/gen", "m/model"), e("m/model", "m/tools/gen")}}
	j, v := customModel().Judge(g)
	if j != 2 || len(v) != 1 {
		t.Fatalf("%d %+v", j, v)
	}
}

func TestJudge_Isolated(t *testing.T) {
	g := Graph{Module: "m", Edges: []Edge{
		e("m/adapters/pg", "m/adapters/http"),
		e("m/adapters", "m/adapters/pg"),
		e("m/adapters/pg", "m/adapters"),
	}}
	j, v := customModel().Judge(g)
	if j != 3 || len(v) != 1 || !v[0].Isolated || v[0].Edge.From != "m/adapters/pg" {
		t.Fatalf("%d %+v", j, v)
	}
}

func TestJudge_UnclassifiedNotJudged(t *testing.T) {
	g := Graph{Module: "m", Edges: []Edge{e("m/zzz", "m/model"), e("m/model", "m/zzz")}}
	if j, v := customModel().Judge(g); j != 0 || len(v) != 0 {
		t.Fatalf("%d %+v", j, v)
	}
}

func TestJudge_Modular(t *testing.T) {
	m := Model{Name: "modular", Kind: Modular, ModulesRoot: "modules"}
	if l, ok := m.Classify("m", "m/modules/billing/db"); !ok || l != "billing" {
		t.Fatalf("%q %v", l, ok)
	}
	if _, ok := m.Classify("m", "m/other"); ok {
		t.Fatal("outside root classified")
	}
	g := Graph{Module: "m", Edges: []Edge{
		e("m/modules/a", "m/modules/billing"),
		e("m/modules/a", "m/modules/billing/db"),
		e("m/modules/billing/x", "m/modules/billing/db"),
	}}
	j, v := m.Judge(g)
	if j != 3 || len(v) != 1 || v[0].Edge.To != "m/modules/billing/db" {
		t.Fatalf("%d %+v", j, v)
	}
}

func TestConformance(t *testing.T) {
	if p, ok := Conformance(10, 1); !ok || p != 90.0 {
		t.Fatalf("%v %v", p, ok)
	}
	if _, ok := Conformance(0, 0); ok {
		t.Fatal("want !ok")
	}
}
