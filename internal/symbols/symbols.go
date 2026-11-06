// Package symbols classifies the Go functions touched by a diff by comparing
// the HEAD and BASE ASTs of every changed file.
package symbols

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"path"
	"sort"
	"strings"

	"gitlab.com/wh1t3hat/mrimpact/internal/diff"
)

type Kind string

const (
	New       Kind = "new"
	Body      Kind = "body"
	Signature Kind = "signature"
	Deleted   Kind = "deleted"
)

type ChangedFunc struct {
	Key        string // dir + "." + [recvTypeName + "."] + name; init funcs "<dir>.init@<file>#N"
	Display    string // package-clause-qualified, e.g. "pricing.(*Calculator).Total"
	File       string // HEAD path (BASE path if Deleted)
	StartLine  int    // FuncDecl position line (HEAD, or BASE if Deleted)
	Kind       Kind
	Exported   bool  // name exported AND (no receiver OR receiver type exported)
	AddedLines []int // added HEAD lines inside the decl range
	IsTest     bool
}

// DeclChange is a changed top-level declaration outside any function.
type DeclChange struct {
	File string
	Line int
	What string // "import", "type Foo", "var x", "const y"
}

type Result struct {
	Funcs        []ChangedFunc
	Decls        []DeclChange
	TestsTouched []string
}

// Source returns file contents; it must return an error wrapping
// fs.ErrNotExist when the file is absent.
type Source func(path string) ([]byte, error)

// fn is one parsed function declaration.
type fn struct {
	change     int // index into the changes slice
	key        string
	display    string
	file       string
	start, end int
	sig        string // printed receiver type + signature types (see signature)
	text       string // printed whole declaration
	exported   bool
	paired     bool
}

// Find compares HEAD and BASE for every non-test change.
func Find(changes []diff.FileChange, head, base Source) (Result, error) {
	var res Result
	var heads, bases []*fn
	declSeen := map[string]bool{}

	for i, c := range changes {
		if c.IsTest {
			p := c.Path
			if c.Status == diff.Deleted {
				p = c.OldPath
			}
			res.TestsTouched = append(res.TestsTouched, p)
			continue
		}
		// init keys use the HEAD base name so renames within a dir still pair.
		keyFile := c.Path
		if keyFile == "" {
			keyFile = c.OldPath
		}
		if c.Status != diff.Deleted {
			fns, decls, err := load(head, c.Path, keyFile, i, c.AddedLines)
			if err != nil {
				return Result{}, err
			}
			heads = append(heads, fns...)
			addDecls(&res, declSeen, decls)
		}
		if c.Status != diff.Added {
			fns, decls, err := load(base, c.OldPath, keyFile, i, c.DeletedLines)
			if err != nil {
				return Result{}, err
			}
			bases = append(bases, fns...)
			addDecls(&res, declSeen, decls)
		}
	}

	// Pair HEAD with BASE: same Key in the same file first, then unique Key
	// across files (a function moved between changed files).
	pair := map[*fn]*fn{}
	for _, h := range heads {
		for _, b := range bases {
			if !b.paired && b.change == h.change && b.key == h.key {
				b.paired, h.paired = true, true
				pair[h] = b
				break
			}
		}
	}
	count := func(fs []*fn) map[string]int {
		m := map[string]int{}
		for _, f := range fs {
			if !f.paired {
				m[f.key]++
			}
		}
		return m
	}
	hc, bc := count(heads), count(bases)
	moved := map[*fn]bool{}
	for _, h := range heads {
		if h.paired || hc[h.key] != 1 || bc[h.key] != 1 {
			continue
		}
		for _, b := range bases {
			if !b.paired && b.key == h.key {
				b.paired, h.paired = true, true
				pair[h] = b
				moved[h] = true
			}
		}
	}

	// Deleted-line hits per BASE function.
	for _, h := range heads {
		cf := ChangedFunc{
			Key: h.key, Display: h.display, File: h.file, StartLine: h.start,
			Exported: h.exported, IsTest: false,
		}
		b := pair[h]
		cf.AddedLines = within(changes[h.change].AddedLines, h.start, h.end)
		switch {
		case b == nil:
			cf.Kind = New
		default:
			changed := len(cf.AddedLines) > 0 || anyIn(changes[b.change].DeletedLines, b.start, b.end)
			if moved[h] {
				// A pure move re-adds the whole body; only content differences count.
				changed = h.text != b.text
			}
			if !changed {
				continue
			}
			cf.Kind = Body
			if h.sig != b.sig {
				cf.Kind = Signature
			}
		}
		res.Funcs = append(res.Funcs, cf)
	}
	for _, b := range bases {
		if b.paired {
			continue
		}
		res.Funcs = append(res.Funcs, ChangedFunc{
			Key: b.key, Display: b.display, File: b.file, StartLine: b.start,
			Kind: Deleted, Exported: b.exported,
		})
	}

	sort.Slice(res.Funcs, func(i, j int) bool {
		a, b := res.Funcs[i], res.Funcs[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.StartLine < b.StartLine
	})
	sort.Slice(res.Decls, func(i, j int) bool {
		a, b := res.Decls[i], res.Decls[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	sort.Strings(res.TestsTouched)
	return res, nil
}

func addDecls(res *Result, seen map[string]bool, ds []DeclChange) {
	for _, d := range ds {
		k := d.File + "\x00" + d.What
		if !seen[k] {
			seen[k] = true
			res.Decls = append(res.Decls, d)
		}
	}
}

// load parses one file and returns its functions plus the top-level
// declarations (outside any function) containing any of changedLines.
func load(src Source, p, keyFile string, change int, changedLines []int) ([]*fn, []DeclChange, error) {
	data, err := src(p)
	if err != nil {
		return nil, nil, fmt.Errorf("symbols: read %s: %w", p, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, p, data, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("symbols: parse %s: %w", p, err)
	}
	pkg := f.Name.Name
	dir := path.Dir(p)
	var fns []*fn
	var decls []DeclChange
	inits := 0
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			x := &fn{
				change: change, file: p, exported: d.Name.IsExported(),
				start: fset.Position(d.Pos()).Line, end: fset.Position(d.End()).Line,
			}
			name := d.Name.Name
			x.display = DisplayName(pkg, d)
			switch {
			case d.Recv == nil && name == "init":
				inits++
				x.key = fmt.Sprintf("%s.init@%s#%d", dir, path.Base(keyFile), inits)
			case d.Recv == nil:
				x.key = dir + "." + name
			default:
				rt := d.Recv.List[0].Type
				tn := recvTypeName(rt)
				x.key = dir + "." + tn + "." + name
				x.exported = x.exported && ast.IsExported(tn)
				x.sig = render(fset, rt)
			}
			x.sig += "|" + signature(fset, d.Type)
			x.text = render(fset, d)
			fns = append(fns, x)
		case *ast.GenDecl:
			s, e := fset.Position(d.Pos()).Line, fset.Position(d.End()).Line
			if !anyIn(changedLines, s, e) {
				continue
			}
			decls = append(decls, genDeclChanges(fset, p, d, changedLines)...)
		}
	}
	return fns, decls, nil
}

func genDeclChanges(fset *token.FileSet, file string, d *ast.GenDecl, lines []int) []DeclChange {
	if d.Tok == token.IMPORT {
		return []DeclChange{{File: file, Line: fset.Position(d.Pos()).Line, What: "import"}}
	}
	var out []DeclChange
	for _, spec := range d.Specs {
		s, e := fset.Position(spec.Pos()).Line, fset.Position(spec.End()).Line
		if len(d.Specs) == 1 {
			s, e = fset.Position(d.Pos()).Line, fset.Position(d.End()).Line
		}
		if !anyIn(lines, s, e) {
			continue
		}
		var names []string
		switch spec := spec.(type) {
		case *ast.TypeSpec:
			names = []string{spec.Name.Name}
		case *ast.ValueSpec:
			for _, n := range spec.Names {
				names = append(names, n.Name)
			}
		}
		out = append(out, DeclChange{File: file, Line: fset.Position(spec.Pos()).Line,
			What: d.Tok.String() + " " + strings.Join(names, ", ")})
	}
	// Changed line in a grouped decl's own "var (" / ")" lines.
	if len(out) == 0 {
		out = append(out, DeclChange{File: file, Line: fset.Position(d.Pos()).Line, What: d.Tok.String()})
	}
	return out
}

// DisplayName returns the package-clause-qualified name of fd:
// "pkg.Name", "pkg.(*Recv).Name" or "pkg.Recv.Name", with receiver type
// parameters as written in source (e.g. "pkg.(*List[T]).Push").
func DisplayName(pkgName string, fd *ast.FuncDecl) string {
	name := fd.Name.Name
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return pkgName + "." + name
	}
	rt := fd.Recv.List[0].Type
	if star, ok := rt.(*ast.StarExpr); ok {
		return pkgName + ".(*" + types.ExprString(star.X) + ")." + name
	}
	return pkgName + "." + types.ExprString(rt) + "." + name
}

func recvTypeName(e ast.Expr) string {
	for {
		switch t := e.(type) {
		case *ast.StarExpr:
			e = t.X
		case *ast.ParenExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		case *ast.IndexListExpr:
			e = t.X
		case *ast.Ident:
			return t.Name
		default:
			return "?"
		}
	}
}

// signature renders the API-relevant part of a function type: type
// parameters with their names and constraints, then parameter and result
// types in order (variadic "..." preserved). Ordinary parameter and result
// names are ignored, so renaming them is not a signature change.
func signature(fset *token.FileSet, ft *ast.FuncType) string {
	var tparams []string
	if ft.TypeParams != nil {
		for _, f := range ft.TypeParams.List {
			c := render(fset, f.Type)
			for _, n := range f.Names {
				tparams = append(tparams, n.Name+" "+c)
			}
		}
	}
	return "[" + strings.Join(tparams, ", ") + "](" + fieldTypes(fset, ft.Params) + ")(" + fieldTypes(fset, ft.Results) + ")"
}

// fieldTypes renders each field's type once per name (once if unnamed).
func fieldTypes(fset *token.FileSet, fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	var out []string
	for _, f := range fl.List {
		t := render(fset, f.Type)
		for range max(1, len(f.Names)) {
			out = append(out, t)
		}
	}
	return strings.Join(out, ", ")
}

func render(fset *token.FileSet, n any) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, n); err != nil {
		return ""
	}
	return b.String()
}

func anyIn(lines []int, lo, hi int) bool {
	return len(within(lines, lo, hi)) > 0
}

func within(lines []int, lo, hi int) []int {
	var out []int
	for _, l := range lines {
		if l >= lo && l <= hi {
			out = append(out, l)
		}
	}
	return out
}
