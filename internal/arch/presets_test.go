package arch

import (
	"reflect"
	"testing"
)

const pm = "m"

func edgeOK(t *testing.T, mod Model, from, to string, want bool) {
	t.Helper()
	g := Graph{Module: pm, Edges: []Edge{{From: from, To: to, File: "f.go", Line: 1}}}
	judged, v := mod.Judge(g)
	if judged != 1 {
		t.Fatalf("%s -> %s not judged", from, to)
	}
	if (len(v) == 0) != want {
		t.Errorf("%s: %s -> %s allowed=%v, want %v", mod.Name, from, to, len(v) == 0, want)
	}
}

func TestPresets_Edges(t *testing.T) {
	cases := []struct {
		preset, from, to string
		ok               bool
	}{
		{"clean", "m/internal/usecase/x", "m/internal/entities/y", true},
		{"clean", "m/internal/entities/y", "m/internal/usecase/x", false},
		{"onion", "m/internal/application/x", "m/internal/domain/y", true},
		{"onion", "m/internal/domain/y", "m/internal/infra/x", false},
		{"hexagonal", "m/internal/adapters/pg", "m/internal/ports", true},
		{"hexagonal", "m/internal/domain/order", "m/internal/adapters/pg", false},
		{"ddd", "m/internal/interfaces/x", "m/internal/domain/y", true},
		{"ddd", "m/internal/domain/y", "m/internal/application/x", false},
		{"layered", "m/internal/presentation/x", "m/internal/service/y", true},
		{"layered", "m/internal/repository/x", "m/internal/service/y", false},
		{"mvc", "m/internal/controllers/x", "m/internal/views/y", true},
		{"mvc", "m/internal/models/x", "m/internal/controllers/y", false},
	}
	for _, c := range cases {
		mod, ok := Preset(c.preset)
		if !ok {
			t.Fatalf("no preset %s", c.preset)
		}
		edgeOK(t, mod, c.from, c.to, c.ok)
	}
}

func TestHexagonal_AdaptersIsolated(t *testing.T) {
	mod, _ := Preset("hexagonal")
	edgeOK(t, mod, "m/internal/adapters/pg", "m/internal/adapters/http", false)
	edgeOK(t, mod, "m/internal/adapters", "m/internal/adapters/pg", true)
}

func TestPipeline_Order(t *testing.T) {
	mod := NewPipeline([]string{"ingest", "transform", "load"})
	edgeOK(t, mod, "m/load", "m/transform", true)
	edgeOK(t, mod, "m/transform", "m/load", false)
	edgeOK(t, mod, "m/transform/a", "m/transform/b", false)
	edgeOK(t, mod, "m/transform", "m/transform/a", true)
}

func TestModular(t *testing.T) {
	if got := NewModular("").ModulesRoot; got != "internal" {
		t.Errorf("default root %q", got)
	}
	mod := NewModular("pkg")
	if mod.Kind != Modular || mod.ModulesRoot != "pkg" {
		t.Errorf("%+v", mod)
	}
}

func TestInferred_Map(t *testing.T) {
	mod := Inferred()
	if mod.Name != "inferred" || !mod.Inferred {
		t.Fatalf("%+v", mod)
	}
	for path, want := range map[string]string{
		"m/internal/service/x":     "application",
		"m/internal/repository/pg": "adapters",
		"m/internal/ports":         "ports",
		"m/internal/domain":        "core",
	} {
		if got, ok := mod.Classify("m", path); !ok || got != want {
			t.Errorf("%s -> %q,%v want %q", path, got, ok, want)
		}
	}
	if l, ok := mod.Classify("m", "m/internal/util"); ok {
		t.Errorf("util classified %q", l)
	}
	edgeOK(t, mod, "m/internal/domain/x", "m/internal/repository/pg", false)
	edgeOK(t, mod, "m/internal/repository/pg", "m/internal/domain/x", true)
}

func TestPresetNames(t *testing.T) {
	want := []string{"clean", "onion", "hexagonal", "ddd", "layered", "mvc", "pipeline", "modular"}
	if got := PresetNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("%v", got)
	}
	for _, n := range []string{"pipeline", "modular", "nope"} {
		if _, ok := Preset(n); ok {
			t.Errorf("Preset(%s) should be false", n)
		}
	}
}

func TestPreset_FreshCopies(t *testing.T) {
	a, _ := Preset("clean")
	a.Layers[0].Globs = append(a.Layers[0].Globs, "x/...")
	a.Layers[0].Dirs[0] = "mutated"
	a.Layers[1].MayImport[0] = "mutated"
	b, _ := Preset("clean")
	if len(b.Layers[0].Globs) != 0 || b.Layers[0].Dirs[0] != "entity" || b.Layers[1].MayImport[0] != "entities" {
		t.Errorf("shared state: %+v", b.Layers)
	}
	i := Inferred()
	i.Layers[0].Dirs[0] = "mutated"
	if Inferred().Layers[0].Dirs[0] == "mutated" {
		t.Error("inferred shared")
	}
}
