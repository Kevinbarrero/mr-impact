// Package load type-checks the module's packages (with tests) and builds
// their SSA form.
package load

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

// PackageError describes a package with load errors: in Program.Broken it is
// excluded from analysis because it, or a package it imports, fails to parse
// or type-check; in Program.Warnings it has only other errors (e.g. a
// //go:embed pattern with no match) and is still analyzed.
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
	Warnings   []PackageError // packages analyzed despite non-type errors
	// Packages are the analyzed (non-broken) packages of the module, test
	// variants included: syntax and type information.
	Packages []*packages.Package
	// Generated holds the repo-relative files with a "Code generated ...
	// DO NOT EDIT." header.
	Generated map[string]bool

	rel func(string) (string, bool)
}

const mode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
	packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule

// Load runs packages.Load("./...") in dir with Tests:true and -tags=<tags>
// (if any), and builds SSA (with ssa.InstantiateGenerics) for the packages
// that are well-typed and import only well-typed packages (see Broken and
// Warnings). It returns an error only if no package loaded at all.
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
	p := &Program{Dir: abs, Fset: pkgs[0].Fset, Files: map[string]bool{}, Generated: map[string]bool{}, rel: rel}
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

	// A package is broken if it failed to parse or type-check, or
	// (transitively) imports one that did; building SSA for ill-typed code
	// panics. pkg.IllTyped is not used: go/packages also sets it for list
	// errors (e.g. a //go:embed pattern with no match) that leave the types
	// complete. Such packages stay analyzable and are reported as warnings.
	broken := map[*packages.Package]string{}
	warned := map[*packages.Package]string{}
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if msg := typeBreakage(pkg); msg != "" {
			broken[pkg] = msg
			return
		}
		for _, imp := range sortedImports(pkg) {
			if _, bad := broken[imp]; bad {
				broken[pkg] = "imports broken package " + imp.PkgPath
				return
			}
		}
		if len(pkg.Errors) > 0 {
			warned[pkg] = firstLine(pkg.Errors[0].Error())
		}
	})

	byPath := map[string]*PackageError{}
	warnByPath := map[string]*PackageError{}
	prog := ssa.NewProgram(p.Fset, ssa.InstantiateGenerics)
	isInitial := map[*packages.Package]bool{}
	for _, pkg := range pkgs {
		isInitial[pkg] = true
	}
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if msg, bad := broken[pkg]; bad {
			if isInitial[pkg] {
				addPackageError(byPath, pkg, msg, rel)
			}
			return
		}
		if msg, ok := warned[pkg]; ok && isInitial[pkg] {
			addPackageError(warnByPath, pkg, msg, rel)
		}
		if isInitial[pkg] {
			prog.CreatePackage(pkg.Types, pkg.Syntax, pkg.TypesInfo, true)
			p.Packages = append(p.Packages, pkg)
			markGenerated(p.Generated, pkg, rel)
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
	for _, e := range warnByPath {
		sort.Strings(e.Files)
		p.Warnings = append(p.Warnings, *e)
	}
	sort.Slice(p.Warnings, func(i, j int) bool { return p.Warnings[i].PkgPath < p.Warnings[j].PkgPath })
	return p, nil
}

// Syntax type-checks the module's non-test packages in dir (no SSA): what
// comparing a second revision needs. Packages with errors are returned
// too, with whatever type information go/packages produced.
func Syntax(ctx context.Context, dir string, tags []string) ([]*packages.Package, func(string) (string, bool), error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("load: %w", err)
	}
	cfg := &packages.Config{Context: ctx, Mode: mode, Dir: abs}
	if len(tags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(tags, ",")}
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, nil, fmt.Errorf("load: %w", err)
	}
	if len(pkgs) == 0 {
		return nil, nil, errors.New("load: no packages found")
	}
	return pkgs, relativizer(abs), nil
}

func markGenerated(gen map[string]bool, pkg *packages.Package, rel func(string) (string, bool)) {
	for _, f := range pkg.Syntax {
		if !ast.IsGenerated(f) {
			continue
		}
		if r, ok := rel(pkg.Fset.Position(f.Package).Filename); ok {
			gen[r] = true
		}
	}
}

// typeBreakage returns a non-empty message if pkg itself failed to parse or
// type-check (or has incomplete types), ignoring errors of its imports.
func typeBreakage(pkg *packages.Package) string {
	if len(pkg.TypeErrors) > 0 {
		return firstLine(pkg.TypeErrors[0].Error())
	}
	for _, e := range pkg.Errors {
		if e.Kind == packages.TypeError || e.Kind == packages.ParseError {
			return firstLine(e.Error())
		}
	}
	// Complete is only meaningful for packages type-checked from source:
	// dependencies read lazily from export data report incomplete types.
	if pkg.Types == nil || (len(pkg.Syntax) > 0 && !pkg.Types.Complete()) {
		if len(pkg.Errors) > 0 {
			return firstLine(pkg.Errors[0].Error())
		}
		return "package types are incomplete"
	}
	return ""
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return strings.TrimSpace(s)
}

// addPackageError records pkg under its import path, merging test variants. A
// package with no files in the repo (a generated test main) is not reported.
func addPackageError(byPath map[string]*PackageError, pkg *packages.Package, msg string, rel func(string) (string, bool)) {
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
