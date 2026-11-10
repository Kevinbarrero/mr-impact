package arch

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/wh1t3hat/mrimpact/internal/config"
)

func parseArch(t *testing.T, y string) config.Architecture {
	t.Helper()
	c, err := config.Parse("t", strings.NewReader(y))
	if err != nil {
		t.Fatal(err)
	}
	return c.Architecture
}

func TestConfigPresetNamesMatchArch(t *testing.T) {
	if !reflect.DeepEqual(config.PresetNames(), PresetNames()) {
		t.Errorf("config %v != arch %v", config.PresetNames(), PresetNames())
	}
}

func TestFromConfig_NotSetIsInferred(t *testing.T) {
	m, err := FromConfig(config.Architecture{})
	if err != nil || !m.Inferred {
		t.Errorf("%v %v", m, err)
	}
}

func TestFromConfig_PresetOverride(t *testing.T) {
	m, err := FromConfig(parseArch(t, "architecture:\n  preset: hexagonal\n  layers:\n    domain: [internal/core/...]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := m.Classify("m", "m/internal/core/x"); !ok || l != "domain" {
		t.Errorf("got %q %v", l, ok)
	}
	if m.Name != "hexagonal" || len(m.Layers) != 4 || m.Layers[0].Name != "domain" || len(m.Layers[0].Dirs) == 0 {
		t.Errorf("%+v", m.Layers)
	}
}

func TestFromConfig_LayerOrder(t *testing.T) {
	m, err := FromConfig(parseArch(t, "architecture:\n  preset: hexagonal\n  layers:\n    adapters: [x/...]\n    extra: [y/...]\n"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, l := range m.Layers {
		names = append(names, l.Name)
	}
	if !reflect.DeepEqual(names, []string{"adapters", "extra", "domain", "ports", "application"}) {
		t.Errorf("%v", names)
	}
	if !m.Layers[0].Isolated || len(m.Layers[1].MayImport) != 0 {
		t.Errorf("%+v", m.Layers)
	}
}

func TestFromConfig_Custom(t *testing.T) {
	m, err := FromConfig(parseArch(t, "architecture:\n  layers:\n    a: [a/...]\n    b: [b/...]\n  allow: [\"b -> a\"]\n  isolated: [a]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "custom" {
		t.Error(m.Name)
	}
	edgeOK(t, m, "m/b/x", "m/a/y", true)
	edgeOK(t, m, "m/a/y", "m/b/x", false)
}

func TestFromConfig_Pipeline(t *testing.T) {
	m, err := FromConfig(parseArch(t, "architecture:\n  preset: pipeline\n  stages: [in, out]\n  layers:\n    out: [sink/...]\n  allow: [\"in -> out\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	edgeOK(t, m, "m/in", "m/sink/x", true)
	edgeOK(t, m, "m/sink/x", "m/in", true)
}

func TestFromConfig_Modular(t *testing.T) {
	m, err := FromConfig(parseArch(t, "architecture:\n  preset: modular\n  modules_root: mods\n"))
	if err != nil || m.Kind != Modular || m.ModulesRoot != "mods" {
		t.Errorf("%+v %v", m, err)
	}
}

func TestFromConfig_UnknownNames(t *testing.T) {
	for _, y := range []string{
		"architecture:\n  layers:\n    a: [a/...]\n  allow: [\"a -> zz\"]\n",
		"architecture:\n  layers:\n    a: [a/...]\n  allow: [\"zz -> a\"]\n",
		"architecture:\n  layers:\n    a: [a/...]\n  isolated: [zz]\n",
	} {
		_, err := FromConfig(parseArch(t, y))
		if !errors.Is(err, config.ErrConfig) || !strings.Contains(err.Error(), "zz") {
			t.Errorf("%q: %v", y, err)
		}
	}
	// entry is known.
	if _, err := FromConfig(parseArch(t, "architecture:\n  layers:\n    a: [a/...]\n  allow: [\"entry -> a\"]\n")); err != nil {
		t.Error(err)
	}
}
