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

// movedBranches returns the number of keys base has and head lacks (a
// multiset difference), how many of them start on an added line of a
// function other than h, and those functions' display names, sorted.
func movedBranches(h, b *fn, dest map[string][]*fn) (removed, moved int, to []string) {
	have := map[string]int{}
	for _, k := range h.keys {
		have[k.text]++
	}
	used := map[string]int{} // destinations already consumed, per text
	set := map[string]bool{}
	for _, k := range b.keys {
		if have[k.text] > 0 {
			have[k.text]--
			continue
		}
		removed++
		var cands []*fn
		for _, d := range dest[k.text] {
			if d != h {
				cands = append(cands, d)
			}
		}
		if used[k.text] < len(cands) {
			set[cands[used[k.text]].display] = true
			used[k.text]++
			moved++
		}
	}
	for d := range set {
		to = append(to, d)
	}
	sort.Strings(to)
	return removed, moved, to
}
