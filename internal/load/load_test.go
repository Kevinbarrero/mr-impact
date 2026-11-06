package load

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const goMod = "module example.com/m\n\ngo 1.26.0\n"

// user imports dep; each case swaps in a different dep.
const userSrc = "package user\n\nimport \"example.com/m/dep\"\n\nfunc U() int { return dep.F() }\n"

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, c := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type pkgFiles struct {
	Path  string
	Files []string
}

func summarize(es []PackageError) []pkgFiles {
	var out []pkgFiles
	for _, e := range es {
		out = append(out, pkgFiles{e.PkgPath, e.Files})
	}
	return out
}

func TestLoadClassification(t *testing.T) {
	cases := []struct {
		name       string
		dir        func(t *testing.T) string
		broken     []pkgFiles
		warnings   []string          // package paths
		analyzable []string          // package paths that must have SSA
		msgs       map[string]string // exact Broken messages to check, by package path
	}{
		{
			name: "type error breaks package and importer",
			dir: func(t *testing.T) string {
				return writeModule(t, map[string]string{
					"go.mod": goMod, "user/user.go": userSrc,
					"dep/dep.go": "package dep\n\nfunc F() int { return \"x\" }\n",
				})
			},
			broken: []pkgFiles{
				{"example.com/m/dep", []string{"dep/dep.go"}},
				{"example.com/m/user", []string{"user/user.go"}},
			},
			msgs: map[string]string{"example.com/m/user": "imports broken package example.com/m/dep"},
		},
		{
			name: "parse error breaks package and importer",
			dir: func(t *testing.T) string {
				return writeModule(t, map[string]string{
					"go.mod": goMod, "user/user.go": userSrc,
					"dep/dep.go": "package dep\n\nfunc F() int {\n\treturn 1 +\n",
				})
			},
			broken: []pkgFiles{
				{"example.com/m/dep", []string{"dep/dep.go"}},
				{"example.com/m/user", []string{"user/user.go"}},
			},
			msgs: map[string]string{"example.com/m/user": "imports broken package example.com/m/dep"},
		},
		{
			name: "go:embed with no match is a warning, importer analyzable",
			dir: func(t *testing.T) string {
				return filepath.Join("..", "..", "testdata", "e2e", "embed_missing", "head")
			},
			warnings:   []string{"example.com/fx/ui"},
			analyzable: []string{"example.com/fx/ui", "example.com/fx/app"},
		},
		{
			name: "unresolved import breaks importer",
			dir: func(t *testing.T) string {
				return writeModule(t, map[string]string{
					"go.mod": goMod, "user/user.go": userSrc,
					"ok/ok.go": "package ok\n\nfunc OK() {}\n",
				})
			},
			broken:     []pkgFiles{{"example.com/m/user", []string{"user/user.go"}}},
			analyzable: []string{"example.com/m/ok"},
		},
		{
			name: "test-variant type error is reported once",
			dir: func(t *testing.T) string {
				return writeModule(t, map[string]string{
					"go.mod":          goMod,
					"dep/dep.go":      "package dep\n\nfunc F() int { return 1 }\n",
					"dep/dep_test.go": "package dep\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) { var s string = F(); _ = s }\n",
				})
			},
			broken:     []pkgFiles{{"example.com/m/dep", []string{"dep/dep.go", "dep/dep_test.go"}}},
			analyzable: []string{"example.com/m/dep"}, // the non-test variant is well-typed
		},
		{
			name: "broken package and its test variant merge into one entry",
			dir: func(t *testing.T) string {
				return writeModule(t, map[string]string{
					"go.mod":          goMod,
					"dep/dep.go":      "package dep\n\nfunc F() int { return \"x\" }\n",
					"dep/dep_test.go": "package dep\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) { F() }\n",
				})
			},
			broken: []pkgFiles{{"example.com/m/dep", []string{"dep/dep.go", "dep/dep_test.go"}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Load(context.Background(), c.dir(t), nil)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := summarize(p.Broken); !reflect.DeepEqual(got, c.broken) {
				t.Errorf("Broken = %+v, want %+v", got, c.broken)
			}
			var warned []string
			for _, w := range p.Warnings {
				warned = append(warned, w.PkgPath)
				if w.Msg == "" {
					t.Errorf("warning for %s has no message", w.PkgPath)
				}
			}
			if !reflect.DeepEqual(warned, c.warnings) {
				t.Errorf("Warnings = %v, want %v", warned, c.warnings)
			}
			for _, e := range p.Broken {
				if e.Msg == "" {
					t.Errorf("broken %s has no message", e.PkgPath)
				}
				if want, ok := c.msgs[e.PkgPath]; ok && e.Msg != want {
					t.Errorf("broken %s: Msg = %q, want %q", e.PkgPath, e.Msg, want)
				}
				if p.SSA.ImportedPackage(e.PkgPath) != nil && !contains(c.analyzable, e.PkgPath) {
					t.Errorf("broken package %s has SSA", e.PkgPath)
				}
			}
			for _, path := range c.analyzable {
				if p.SSA.ImportedPackage(path) == nil {
					t.Errorf("package %s not analyzable (no SSA)", path)
				}
			}
		})
	}
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}
