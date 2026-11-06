// Package load type-checks the module's packages (with tests) and builds
// their SSA form.
package load

import (
	"context"
	"errors"
	"fmt"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

// PackageError describes a package excluded from analysis because it, or a
// package it imports, fails to load or type-check.
type PackageError struct {
	PkgPath string
	Files   []string // repo-relative
	Msg     string
}

type Program struct {
	Dir        string // absolute module root
	ModulePath string
	Fset       *token.FileSet
	SSA        *ssa.Program
	Files      map[string]bool // repo-relative paths of every compiled file
	Broken     []PackageError

	rel func(string) (string, bool)
}

const mode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
	packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule

// Load runs packages.Load("./...") in dir with Tests:true and -tags=<tags>
// (if any), and builds SSA (with ssa.InstantiateGenerics) for the packages
// that are well-typed and import only well-typed packages. It returns an
// error only if no package loaded at all.
func Load(ctx context.Context, dir string, tags []string) (*Program, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	cfg := &packages.Config{Context: ctx, Mode: mode, Dir: abs, Tests: true}
	if len(tags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(tags, ",")}
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	if len(pkgs) == 0 {
		return nil, errors.New("load: no packages found")
	}

	rel := relativizer(abs)
	p := &Program{Dir: abs, Fset: pkgs[0].Fset, Files: map[string]bool{}, rel: rel}
	for _, pkg := range pkgs {
		if pkg.Module != nil && p.ModulePath == "" {
			p.ModulePath = pkg.Module.Path
		}
		for _, f := range pkg.CompiledGoFiles {
			if r, ok := rel(f); ok {
				p.Files[r] = true
			}
		}
	}

	// A package is broken if it has errors or (transitively) imports one
	// that does; building SSA for ill-typed code panics.
	broken := map[*packages.Package]string{}
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		var msgs []string
		for _, e := range pkg.Errors {
			msgs = append(msgs, e.Error())
		}
		if len(msgs) == 0 {
			for _, imp := range sortedImports(pkg) {
				if _, bad := broken[imp]; bad {
					msgs = append(msgs, "imports broken package "+imp.PkgPath)
					break
				}
			}
		}
		if len(msgs) == 0 && pkg.IllTyped {
			msgs = append(msgs, "package or a dependency has errors")
		}
		if len(msgs) > 0 {
			broken[pkg] = strings.Join(msgs, "; ")
		}
	})

	byPath := map[string]*PackageError{}
	prog := ssa.NewProgram(p.Fset, ssa.InstantiateGenerics)
	isInitial := map[*packages.Package]bool{}
	for _, pkg := range pkgs {
		isInitial[pkg] = true
	}
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if msg, bad := broken[pkg]; bad {
			if isInitial[pkg] {
				addBroken(byPath, pkg, msg, rel)
			}
			return
		}
		if pkg.Types == nil {
			return
		}
		if isInitial[pkg] {
			prog.CreatePackage(pkg.Types, pkg.Syntax, pkg.TypesInfo, true)
		} else {
			prog.CreatePackage(pkg.Types, nil, nil, true)
		}
	})
	prog.Build()
	p.SSA = prog

	for _, e := range byPath {
		sort.Strings(e.Files)
		p.Broken = append(p.Broken, *e)
	}
	sort.Slice(p.Broken, func(i, j int) bool { return p.Broken[i].PkgPath < p.Broken[j].PkgPath })
	return p, nil
}

// addBroken records pkg under its import path, merging test variants. A
// package with no files in the repo (a generated test main) is not reported.
func addBroken(byPath map[string]*PackageError, pkg *packages.Package, msg string, rel func(string) (string, bool)) {
	var files []string
	for _, f := range pkg.CompiledGoFiles {
		if r, ok := rel(f); ok {
			files = append(files, r)
		}
	}
	if len(files) == 0 {
		for _, f := range pkg.GoFiles {
			if r, ok := rel(f); ok {
				files = append(files, r)
			}
		}
	}
	if len(files) == 0 && len(pkg.CompiledGoFiles)+len(pkg.GoFiles) > 0 {
		return
	}
	e := byPath[pkg.PkgPath]
	if e == nil {
		e = &PackageError{PkgPath: pkg.PkgPath, Msg: msg}
		byPath[pkg.PkgPath] = e
	}
	have := map[string]bool{}
	for _, f := range e.Files {
		have[f] = true
	}
	for _, f := range files {
		if !have[f] {
			e.Files = append(e.Files, f)
		}
	}
}

func sortedImports(pkg *packages.Package) []*packages.Package {
	keys := make([]string, 0, len(pkg.Imports))
	for k := range pkg.Imports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*packages.Package, len(keys))
	for i, k := range keys {
		out[i] = pkg.Imports[k]
	}
	return out
}

// relativizer returns a func mapping an absolute file name to its
// slash-separated path relative to root; ok is false outside root.
func relativizer(root string) func(string) (string, bool) {
	roots := []string{root}
	if real, err := filepath.EvalSymlinks(root); err == nil && real != root {
		roots = append(roots, real)
	}
	return func(name string) (string, bool) {
		for _, r := range roots {
			rel, err := filepath.Rel(r, name)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				return filepath.ToSlash(rel), true
			}
		}
		return "", false
	}
}

// Rel maps an absolute file name (as recorded in Fset) to its repo-relative,
// slash-separated path; ok is false for files outside the module root.
func (p *Program) Rel(name string) (string, bool) {
	return p.rel(name)
}
