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
	// BranchesRemoved: HEAD has fewer case clauses, if/else branches or
	// returns than BASE (Body and Signature only). Removed control flow
	// changes behaviour even when no line was added.
	BranchesRemoved bool
	// Wiring: straight-line setup code (see isWiring). False for deleted
	// functions.
	Wiring bool
}

// DeclChange is a changed top-level declaration outside any function.
type DeclChange struct {
	File string
	Line int
	What string // "import", "type Foo", "var x", "const y"
}

type Result struct {
	Funcs        []ChangedFunc
	Decls        []DeclChange // never imports, nor declarations of deleted files
	TestsTouched []string
	DeletedFiles []string // non-test files removed by the diff, sorted
	// DeletedRefs holds what the deleted lines of non-test files name: the
	// functions the removed code called or referenced, among others. A
	// HEAD function named here may have lost its last caller.
	DeletedRefs map[Ref]bool
	// RemovedCases: named case values (constants, types) removed from a
	// switch that is still there. If the value still exists it now reaches
	// default, or no case at all.
	RemovedCases []RemovedCase
}

// RemovedCase is a case value a changed function's switch no longer lists
// while the switch itself remains.
type RemovedCase struct {
	Func       string // Display of the function
	File       string // HEAD
	Line       int    // HEAD line of the switch
	Case       string // the case expression as written in BASE
	Ref        Ref    // what it names; the caller checks it still exists
	HasDefault bool   // the HEAD switch has a default clause
}

// Ref is a name used by removed code, resolved from its syntax alone.
type Ref struct {
	Dir    string // repo-relative dir of the package the name belongs to ("" if Import is set)
	Import string // import path, for pkg.Name through the file's imports
	Name   string
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
	branches   [4]int // case clauses, ifs, elses, returns
	wiring     bool
	cases      []caseEntry
	switches   map[string]switchInfo // by switchKey
}

type caseEntry struct {
	key  string // switchKey + "\x00" + case text
	text string
	ref  Ref
}

type switchInfo struct {
	line       int
	hasDefault bool
}

// Find compares HEAD and BASE for every non-test change.
func Find(changes []diff.FileChange, head, base Source) (Result, error) {
	res := Result{DeletedRefs: map[Ref]bool{}}
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
			fns, decls, err := load(head, c.Path, keyFile, i, c.AddedLines, nil)
			if err != nil {
				return Result{}, err
			}
			heads = append(heads, fns...)
			addDecls(&res, declSeen, decls)
		}
		if c.Status != diff.Added {
			fns, decls, err := load(base, c.OldPath, keyFile, i, c.DeletedLines, res.DeletedRefs)
			if err != nil {
				return Result{}, err
			}
			bases = append(bases, fns...)
			// A deleted file's declarations went with it; DeletedFiles says so.
			if c.Status == diff.Deleted {
				res.DeletedFiles = append(res.DeletedFiles, c.OldPath)
			} else {
				addDecls(&res, declSeen, decls)
			}
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
			Exported: h.exported, IsTest: false, Wiring: h.wiring,
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
			for k := range h.branches {
				if h.branches[k] < b.branches[k] {
					cf.BranchesRemoved = true
				}
			}
			res.RemovedCases = append(res.RemovedCases, removedCases(h, b)...)
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
	sort.Strings(res.DeletedFiles)
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
// declarations (outside any function) containing any of changedLines. If
// refs is non-nil, the names used on changed lines are added to it.
func load(src Source, p, keyFile string, change int, changedLines []int, refs map[Ref]bool) ([]*fn, []DeclChange, error) {
	data, err := src(p)
	if err != nil {
		return nil, nil, fmt.Errorf("symbols: read %s: %w", p, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, p, data, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("symbols: parse %s: %w", p, err)
	}
	if refs != nil && len(changedLines) > 0 {
		collectRefs(fset, f, path.Dir(p), changedLines, refs)
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
			x.branches = branches(d)
			x.wiring = isWiring(d)
			x.cases, x.switches = switchCases(fset, d, path.Dir(p), fileImports(f))
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

// collectRefs adds the names used on lines of f to refs: a bare name
// belongs to f's own package (dir); pkg.Name to the package f imports as
// pkg; x.name (a method or field) to f's own package too, since only
// unexported methods can be reported unused and those are callable only
// from their own package.
func collectRefs(fset *token.FileSet, f *ast.File, dir string, lines []int, refs map[Ref]bool) {
	on := map[int]bool{}
	for _, l := range lines {
		on[l] = true
	}
	imports := fileImports(f)
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if on[fset.Position(n.Sel.Pos()).Line] {
				if x, ok := n.X.(*ast.Ident); ok && imports[x.Name] != "" {
					refs[Ref{Import: imports[x.Name], Name: n.Sel.Name}] = true
				} else {
					refs[Ref{Dir: dir, Name: n.Sel.Name}] = true
				}
			}
			ast.Inspect(n.X, visit)
			return false
		case *ast.Ident:
			if on[fset.Position(n.Pos()).Line] {
				refs[Ref{Dir: dir, Name: n.Name}] = true
			}
		}
		return true
	}
	ast.Inspect(f, visit)
}

// fileImports maps f's import names to import paths.
func fileImports(f *ast.File) map[string]string {
	imports := map[string]string{}
	for _, im := range f.Imports {
		ip := strings.Trim(im.Path.Value, `"`)
		name := importName(ip)
		if im.Name != nil {
			name = im.Name.Name
		}
		if name != "_" && name != "." {
			imports[name] = ip
		}
	}
	return imports
}

// switchCases lists d's named case values (identifiers, pkg.Name, *T) per
// switch, keyed by the switched expression, and its switches.
func switchCases(fset *token.FileSet, d *ast.FuncDecl, dir string, imports map[string]string) ([]caseEntry, map[string]switchInfo) {
	var cases []caseEntry
	switches := map[string]switchInfo{}
	if d.Body == nil {
		return nil, switches
	}
	ast.Inspect(d.Body, func(n ast.Node) bool {
		var key string
		var body *ast.BlockStmt
		switch n := n.(type) {
		case *ast.SwitchStmt:
			if n.Tag == nil {
				return true // switch { case cond: } has no values
			}
			key, body = "expr:"+types.ExprString(n.Tag), n.Body
		case *ast.TypeSwitchStmt:
			x := typeSwitchSubject(n)
			if x == nil {
				return true
			}
			key, body = "type:"+types.ExprString(x), n.Body
		default:
			return true
		}
		info := switchInfo{line: fset.Position(n.Pos()).Line}
		for _, st := range body.List {
			cc := st.(*ast.CaseClause)
			if cc.List == nil {
				info.hasDefault = true
			}
			for _, e := range cc.List {
				ref, ok := caseRef(e, dir, imports)
				if !ok {
					continue
				}
				text := types.ExprString(e)
				cases = append(cases, caseEntry{key: key + "\x00" + text, text: text, ref: ref})
			}
		}
		switches[key] = info
		return true
	})
	return cases, switches
}

func typeSwitchSubject(n *ast.TypeSwitchStmt) ast.Expr {
	var e ast.Expr
	switch a := n.Assign.(type) {
	case *ast.ExprStmt:
		e = a.X
	case *ast.AssignStmt:
		if len(a.Rhs) == 1 {
			e = a.Rhs[0]
		}
	}
	if ta, ok := e.(*ast.TypeAssertExpr); ok {
		return ta.X
	}
	return nil
}

// caseRef resolves a case expression naming a package-level object.
func caseRef(e ast.Expr, dir string, imports map[string]string) (Ref, bool) {
	switch e := ast.Unparen(e).(type) {
	case *ast.StarExpr:
		return caseRef(e.X, dir, imports)
	case *ast.Ident:
		return Ref{Dir: dir, Name: e.Name}, true
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok && imports[x.Name] != "" {
			return Ref{Import: imports[x.Name], Name: e.Sel.Name}, true
		}
	}
	return Ref{}, false
}

// removedCases returns the case values of b missing from h, for switches
// h still has.
func removedCases(h, b *fn) []RemovedCase {
	left := map[string]int{}
	for _, c := range h.cases {
		left[c.key]++
	}
	var out []RemovedCase
	for _, c := range b.cases {
		if left[c.key] > 0 {
			left[c.key]--
			continue
		}
		sw, _, _ := strings.Cut(c.key, "\x00")
		info, ok := h.switches[sw]
		if !ok {
			continue
		}
		out = append(out, RemovedCase{Func: h.display, File: h.file, Line: info.line,
			Case: c.text, Ref: c.ref, HasDefault: info.hasDefault})
	}
	return out
}

// importName guesses the package name of import path ip from its last
// element, skipping a major-version suffix (example.com/m/v2 -> m).
func importName(ip string) string {
	parts := strings.Split(ip, "/")
	name := parts[len(parts)-1]
	if len(parts) > 1 && len(name) > 1 && name[0] == 'v' && strings.Trim(name[1:], "0123456789") == "" {
		name = parts[len(parts)-2]
	}
	return name
}

// branches counts d's case clauses (switch and select), ifs, elses and
// returns, closures included.
func branches(d *ast.FuncDecl) [4]int {
	var c [4]int
	if d.Body == nil {
		return c
	}
	ast.Inspect(d.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CaseClause, *ast.CommClause:
			c[0]++
		case *ast.IfStmt:
			c[1]++
			if n.Else != nil {
				c[2]++
			}
		case *ast.ReturnStmt:
			c[3]++
		}
		return true
	})
	return c
}

func genDeclChanges(fset *token.FileSet, file string, d *ast.GenDecl, lines []int) []DeclChange {
	// Import changes follow from the code changes; the compiler checks them.
	if d.Tok == token.IMPORT {
		return nil
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
