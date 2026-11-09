// Package configfields finds configuration struct fields (fields carrying
// an env/envconfig/yaml/... tag) that no code reads: settings, often
// environment variables, that a deployment still sets for nothing.
package configfields

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Keys are the struct tag keys that mark a configuration field (envconfig,
// caarlos0/env, cleanenv, viper/mapstructure, yaml, toml, koanf).
var Keys = []string{"envconfig", "env", "env-default", "mapstructure", "yaml", "toml", "koanf", "default", "required", "split_words"}

// Field is a configuration field nothing reads.
type Field struct {
	Display  string // "pkg.Type.Field"
	Tag      string // the first config tag, e.g. `envconfig:"HTTP_PORT"`
	File     string // repo-relative
	Line     int
	TestOnly bool // only tests read it
}

// Unused returns the configuration fields of non-test, non-generated files
// that want accepts (by package path and field name) and that no non-test
// code uses (a use is any
// reference to the field: read, write or composite literal key; filling by
// reflection is not one). Sorted by file and line.
func Unused(pkgs []*packages.Package, rel func(string) (string, bool), want func(pkgPath, name string) bool) []Field {
	type cand struct {
		f        Field
		used     bool
		testUsed bool
	}
	cands := map[token.Position]*cand{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Syntax {
			file, ok := rel(pkg.Fset.Position(f.Package).Filename)
			if !ok || strings.HasSuffix(file, "_test.go") || ast.IsGenerated(f) {
				continue
			}
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts := spec.(*ast.TypeSpec)
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, fld := range st.Fields.List {
						tag := configTag(fld.Tag)
						if tag == "" {
							continue
						}
						for _, n := range fld.Names {
							if !want(pkg.PkgPath, n.Name) {
								continue
							}
							at := pkg.Fset.Position(n.Pos())
							key := token.Position{Filename: file, Line: at.Line, Column: at.Column}
							if cands[key] == nil {
								cands[key] = &cand{f: Field{Display: pkg.Name + "." + ts.Name.Name + "." + n.Name, Tag: tag, File: file, Line: at.Line}}
							}
						}
					}
				}
			}
		}
	}
	if len(cands) == 0 {
		return nil
	}
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for id, obj := range pkg.TypesInfo.Uses {
			v, ok := obj.(*types.Var)
			if !ok || !v.IsField() {
				continue
			}
			decl := pkg.Fset.Position(v.Pos())
			dfile, ok := rel(decl.Filename)
			if !ok {
				continue
			}
			c := cands[token.Position{Filename: dfile, Line: decl.Line, Column: decl.Column}]
			if c == nil {
				continue
			}
			if ufile, _ := rel(pkg.Fset.Position(id.Pos()).Filename); strings.HasSuffix(ufile, "_test.go") {
				c.testUsed = true
			} else {
				c.used = true
			}
		}
	}
	var out []Field
	for _, c := range cands {
		if !c.used {
			c.f.TestOnly = c.testUsed
			out = append(out, c.f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// configTag returns the first configuration key of tag as `key:"value"`,
// or "" if it has none.
func configTag(lit *ast.BasicLit) string {
	if lit == nil {
		return ""
	}
	raw, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	st := reflect.StructTag(raw)
	for _, k := range Keys {
		if v, ok := st.Lookup(k); ok {
			return k + `:"` + v + `"`
		}
	}
	return ""
}
