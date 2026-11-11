package symbols

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"sort"
	"strings"
)

// branchKey names one branch of a function body: its printed text with
// whitespace collapsed, and the line it starts on.
type branchKey struct {
	text string
	line int
}

// branchKeys returns the keys of d's case clauses, ifs, elses and returns
// in source order, closures included (the same set branches counts).
func branchKeys(fset *token.FileSet, d *ast.FuncDecl) []branchKey {
	if d.Body == nil {
		return nil
	}
	var out []branchKey
	add := func(pos token.Pos, text string) {
		out = append(out, branchKey{text: text, line: fset.Position(pos).Line})
	}
	list := func(xs []ast.Expr) string {
		parts := make([]string, len(xs))
		for i, x := range xs {
			parts[i] = printNode(fset, x)
		}
		return strings.Join(parts, ", ")
	}
	ast.Inspect(d.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CaseClause:
			if n.List == nil {
				add(n.Pos(), "default")
			} else {
				add(n.Pos(), "case "+list(n.List))
			}
		case *ast.CommClause:
			if n.Comm == nil {
				add(n.Pos(), "default")
			} else {
				add(n.Pos(), "case "+printNode(fset, n.Comm))
			}
		case *ast.IfStmt:
			cond := printNode(fset, n.Cond)
			if n.Init != nil {
				cond = printNode(fset, n.Init) + "; " + cond
			}
			add(n.Pos(), "if "+cond)
			if n.Else != nil {
				add(n.Else.Pos(), "else "+cond)
			}
		case *ast.ReturnStmt:
			if len(n.Results) == 0 {
				add(n.Pos(), "return")
			} else {
				add(n.Pos(), "return "+list(n.Results))
			}
		}
		return true
	})
	return out
}

func printNode(fset *token.FileSet, n ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, n); err != nil {
		return ""
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

// genericKeys are branch keys too common to say where a branch went: a
// removed one counts as moved only to a function the source function's
// HEAD body references (see refersTo). The text is the normalised key.
var genericKeys = map[string]bool{
	"return":          true,
	"return nil":      true,
	"return err":      true,
	"return nil, err": true,
	"return true":     true,
	"return false":    true,
	"default":         true,
	"if err != nil":   true,
	"else err != nil": true,
}

// destCopy is one branch key of a HEAD function that may be where a
// removed branch went; used once it has accounted for one removed key.
type destCopy struct {
	f    *fn
	used bool
}

// funcRefs returns what d's body names, resolved from syntax alone (the
// symbols package has no type information): a bare name belongs to d's
// own package dir; pkg.Name to the package the file imports as pkg;
// x.Name (a method call or method value) to any method named Name.
func funcRefs(d *ast.FuncDecl, dir string, imports map[string]string) map[Ref]bool {
	refs := map[Ref]bool{}
	if d.Body == nil {
		return refs
	}
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if x, ok := n.X.(*ast.Ident); ok && imports[x.Name] != "" {
				refs[Ref{Import: imports[x.Name], Name: n.Sel.Name}] = true
				return false
			}
			refs[Ref{Name: n.Sel.Name}] = true
			ast.Inspect(n.X, visit)
			return false
		case *ast.Ident:
			refs[Ref{Dir: dir, Name: n.Name}] = true
		}
		return true
	}
	ast.Inspect(d.Body, visit)
	return refs
}

// refersTo reports whether f's HEAD body references d, matched by name
// and package: a function by its name in its own dir, or through an
// import whose path is d's dir or ends in "/" + d's dir; a method by its
// name alone, since the receiver's type is not known.
func refersTo(f, d *fn) bool {
	if d.method {
		return f.refs[Ref{Name: d.name}]
	}
	if f.refs[Ref{Dir: d.dir, Name: d.name}] {
		return true
	}
	for r := range f.refs {
		if r.Import != "" && r.Name == d.name && (r.Import == d.dir || strings.HasSuffix(r.Import, "/"+d.dir)) {
			return true
		}
	}
	return false
}

// movedBranches returns the number of keys base has and head lacks (a
// multiset difference), how many of them have a destination copy, and
// those destinations' display names, sorted. A copy counts when it lies
// in a function other than h that h's HEAD body references, or, for a
// key outside genericKeys, in any other function; a referenced copy is
// preferred. Copies are shared by all source functions: the caller
// processes them in Display order and each copy accounts for one key.
func movedBranches(h, b *fn, dest map[string][]*destCopy) (removed, moved int, to []string) {
	have := map[string]int{}
	for _, k := range h.keys {
		have[k.text]++
	}
	set := map[string]bool{}
	for _, k := range b.keys {
		if have[k.text] > 0 {
			have[k.text]--
			continue
		}
		removed++
		var pick *destCopy
		for _, c := range dest[k.text] {
			if c.used || c.f == h {
				continue
			}
			if refersTo(h, c.f) {
				pick = c
				break
			}
			if pick == nil && !genericKeys[k.text] {
				pick = c
			}
		}
		if pick != nil {
			pick.used = true
			set[pick.f.display] = true
			moved++
		}
	}
	for d := range set {
		to = append(to, d)
	}
	sort.Strings(to)
	return removed, moved, to
}
