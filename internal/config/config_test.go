package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rootFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".mrimpact.yml"), content)
	return dir
}

func TestLoad_NoFileIsDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir(), "")
	if err != nil || !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestLoad_EmptyFileIsDefaults(t *testing.T) {
	cfg, err := Load(rootFile(t, ""), "")
	if err != nil || !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestLoad_CommentsOnlyFileIsDefaults(t *testing.T) {
	cfg, err := Load(rootFile(t, "# nothing\n"), "")
	if err != nil || !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestLoad_Full(t *testing.T) {
	cfg, err := Load("", "testdata/full.yml")
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Critical:        []string{"internal/payment/..."},
		SensitiveAdd:    []string{"github.com/shopspring/decimal.*", "example.com/ledger.(*Ledger).Post"},
		SensitiveRemove: []string{"os/exec.*"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestLoad_UnknownKey(t *testing.T) {
	_, err := Load(rootFile(t, "critcal: [x]\n"), "")
	if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "critcal") {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_WrongType(t *testing.T) {
	_, err := Load(rootFile(t, "critical: internal/x\n"), "")
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_BadGlob(t *testing.T) {
	_, err := Load(rootFile(t, `critical: ["internal/[x"]`+"\n"), "")
	if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "internal/[x") {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_ExplicitMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, dir+"/nope.yml")
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_ExplicitPathWins(t *testing.T) {
	dir := rootFile(t, "critical: [root/...]\n")
	explicit := filepath.Join(t.TempDir(), "other.yml")
	write(t, explicit, "critical: [explicit/...]\n")
	cfg, err := Load(dir, explicit)
	if err != nil || !reflect.DeepEqual(cfg.Critical, []string{"explicit/..."}) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestParse(t *testing.T) {
	cfg, err := Parse("base:.mrimpact.yml", strings.NewReader("sensitive:\n  remove: [os/exec.*]\n"))
	if err != nil || !reflect.DeepEqual(cfg, Config{SensitiveRemove: []string{"os/exec.*"}}) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
	if _, err := Parse("base:.mrimpact.yml", strings.NewReader("nope: 1\n")); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "base:.mrimpact.yml") {
		t.Fatalf("got %v", err)
	}
}

func TestChanges(t *testing.T) {
	base := Config{Critical: []string{"a/...", "b"}, SensitiveAdd: []string{"x.F"}, SensitiveRemove: []string{"os.Remove"}}
	head := Config{Critical: []string{"a/...", "c"}, SensitiveAdd: []string{"y.G"}, SensitiveRemove: []string{"os.Remove", "os/exec.*"}}
	want := []Change{
		{"adds critical", []string{"c"}}, {"drops critical", []string{"b"}},
		{"removes from defaults", []string{"os/exec.*"}},
		{"adds", []string{"y.G"}}, {"drops added", []string{"x.F"}},
	}
	if got := Changes(base, head); !reflect.DeepEqual(got, want) {
		t.Errorf("Changes = %q, want %q", got, want)
	}
	if got := Changes(head, base); !slices.ContainsFunc(got, func(c Change) bool {
		return c.What == "restores defaults" && reflect.DeepEqual(c.Items, []string{"os/exec.*"})
	}) {
		t.Errorf("reverse Changes = %q", got)
	}
	if got := Changes(base, base); got != nil {
		t.Errorf("same config: %q", got)
	}
	// Order and empty-vs-nil do not count as a change.
	if got := Changes(Config{Critical: []string{"b", "a"}, SensitiveAdd: []string{}}, Config{Critical: []string{"a", "b"}}); got != nil {
		t.Errorf("reordered: %q", got)
	}
}

// Critical globs are cleaned: a leading "./" and a trailing "/" are accepted.
func TestLoad_CriticalGlobsCleaned(t *testing.T) {
	cfg, err := Load(rootFile(t, "critical: [./internal/pay/, internal//x/..., \"a/*/\", ./b/...]\n"), "")
	want := []string{"internal/pay", "internal/x/...", "a/*", "b/..."}
	if err != nil || !reflect.DeepEqual(cfg.Critical, want) {
		t.Fatalf("got %q, %v; want %q", cfg.Critical, err, want)
	}
}

func TestParse_LayerOrderPreserved(t *testing.T) {
	c, err := Parse("t", strings.NewReader("architecture:\n  layers:\n    zeta: [a/...]\n    alpha: [./a/b/]\n"))
	if err != nil {
		t.Fatal(err)
	}
	l := c.Architecture.Layers
	if !c.Architecture.Set || len(l) != 2 || l[0].Name != "zeta" || l[1].Name != "alpha" || l[1].Globs[0] != "a/b" {
		t.Errorf("%+v", c.Architecture)
	}
}

func TestParse_ArchitectureErrors(t *testing.T) {
	cases := map[string]string{
		"architecture:\n  preset: nope\n":                                    "nope",
		"architecture: {}\n":                                                 "preset or layers",
		"architecture:\n  layers:\n    a: [x]\n  allow: [\"a b\"]\n":         "a b",
		"architecture:\n  layers:\n    a: [x]\n  allow: [\"a -> b -> c\"]\n": "a -> b -> c",
		"architecture:\n  preset: clean\n  stages: [s1]\n":                   "stages",
		"architecture:\n  preset: pipeline\n":                                "stages",
		"architecture:\n  preset: pipeline\n  stages: [a, a]\n":              "\"a\"",
		"architecture:\n  preset: clean\n  modules_root: internal\n":         "modules_root",
		"architecture:\n  layers:\n    a: [x]\n    a: [y]\n":                 "duplicate layer \"a\"",
		"architecture:\n  layers:\n    a: [\"internal/[x\"]\n":               "internal/[x",
		"architecture:\n  layers:\n    a: x\n":                               "\"a\"",
		"architecture:\n  layers: [a]\n":                                     "mapping",
		"architecture:\n  preset: modular\n  layers:\n    a: [x]\n":          "modular",
		"architecture:\n  preset: modular\n  isolated: [a]\n":                "modular",
		"architecture:\n  layers:\n    a: [x]\n  allow: [\"a->b\"]\n":        "a->b",
		"architecture:\n  layers:\n    a: [x]\n  allow: [\"a ->b\"]\n":       "a ->b",
		"architecture:\n  layers:\n    a: [x]\n  allow: [\"a-> b\"]\n":       "a-> b",
		"architecture:\n  layers:\n    a: []\n":                              "layer \"a\"",
		"architecture:\n  preset: clean\n  bogus: 1\n":                       "bogus",
	}
	for y, want := range cases {
		_, err := Parse("t", strings.NewReader(y))
		if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v (want %q)", y, err, want)
		}
	}
}
