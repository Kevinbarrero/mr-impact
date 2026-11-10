// Package registrations finds the places where a function of the module is
// handed to a call as a value (an event subscription, an HTTP route, a CLI
// command, a job) and compares them between two revisions. It looks at Go
// syntax and types only, so it works the same for any framework.
package registrations

import (
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Registration is one call that takes a module function as an argument.
type Registration struct {
	Callee  string // the call's function, e.g. "SubscribeAsync", "GET"
	Call    string // the call as written, the handler as "…", shortened
	Handler string // display name of the function passed, e.g. "api.(*Server).handleUsers"
	File    string // repo-relative
	Line    int
	key     string // Callee + args, the handler by name: what Diff compares
	slot    string // Callee + args, the handler as "…": the same call with another handler is a rewiring
}

// Rewired is a registration whose call stayed and whose handler changed.
type Rewired struct {
	From, To Registration // base and head registration
}

const maxCallText = 90

// Find returns the registrations in the non-test, non-generated files of
// pkgs whose handler is declared in module modulePath, sorted by file and
// line. rel maps an absolute file name to a repo-relative one. Test
// variants of a package are deduplicated.
func Find(pkgs []*packages.Package, modulePath string, rel func(string) (string, bool)) []Registration {
	var out []Registration
	seen := map[string]bool{}
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, f := range pkg.Syntax {
			file, ok := rel(pkg.Fset.Position(f.Package).Filename)
			if !ok || strings.HasSuffix(file, "_test.go") || ast.IsGenerated(f) {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || isConversion(pkg.TypesInfo, call) {
					return true
				}
				for i, arg := range call.Args {
					fn := handler(pkg.TypesInfo, arg)
					if fn == nil || fn.Pkg() == nil || !inModule(fn.Pkg().Path(), modulePath) {
						continue
					}
					at := pkg.Fset.Position(call.Lparen)
					dedup := file + ":" + at.String() + ":" + fn.FullName()
					if seen[dedup] {
						continue
					}
					seen[dedup] = true
					out = append(out, newRegistration(call, i, fn, file, at.Line))
				}
				return true
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].key < out[j].key
	})
	return out
}

func newRegistration(call *ast.CallExpr, arg int, fn *types.Func, file string, line int) Registration {
	r := Registration{Callee: calleeName(call.Fun), Handler: Display(fn), File: file, Line: line}
	shown := make([]string, len(call.Args))
	keyed := make([]string, len(call.Args))
	for i, a := range call.Args {
		shown[i] = types.ExprString(a)
		keyed[i] = shown[i]
		if i == arg {
			shown[i], keyed[i] = "…", r.Handler
		}
	}
	r.Call = r.Callee + "(" + strings.Join(shown, ", ") + ")"
	if rs := []rune(r.Call); len(rs) > maxCallText {
		r.Call = string(rs[:maxCallText-1]) + "…"
	}
	r.key = r.Callee + "(" + strings.Join(keyed, ", ") + ")"
	full := append([]string(nil), keyed...)
	full[arg] = "…"
	r.slot = r.Callee + "(" + strings.Join(full, ", ") + ")"
	return r
}

// Diff returns the registrations of base missing from head (removed) and
// those of head missing from base (added), compared by call and handler
// (not position). A registration made twice and kept once counts once.
func Diff(base, head []Registration) (removed, added []Registration) {
	count := map[string]int{}
	for _, r := range head {
		count[r.key]++
	}
	for _, r := range base {
		if count[r.key] > 0 {
			count[r.key]--
		} else {
			removed = append(removed, r)
		}
	}
	count = map[string]int{}
	for _, r := range base {
		count[r.key]++
	}
	for _, r := range head {
		if count[r.key] > 0 {
			count[r.key]--
		} else {
			added = append(added, r)
		}
	}
	return removed, added
}

// Pair moves removed/added registrations made by the same call (same
// function and arguments, another handler) into rewired; what stays in
// removed has no replacement. Pairs are taken in order.
func Pair(removed, added []Registration) (rewired []Rewired, gone, fresh []Registration) {
	bySlot := map[string][]int{}
	for i, a := range added {
		bySlot[a.slot] = append(bySlot[a.slot], i)
	}
	used := map[int]bool{}
	for _, r := range removed {
		if q := bySlot[r.slot]; len(q) > 0 {
			bySlot[r.slot] = q[1:]
			used[q[0]] = true
			rewired = append(rewired, Rewired{From: r, To: added[q[0]]})
			continue
		}
		gone = append(gone, r)
	}
	for i, a := range added {
		if !used[i] {
			fresh = append(fresh, a)
		}
	}
	return rewired, gone, fresh
}

// handler returns the function arg denotes when it is a function or method
// value (possibly converted to a named func type), else nil.
func handler(info *types.Info, arg ast.Expr) *types.Func {
	for {
		arg = ast.Unparen(arg)
		c, ok := arg.(*ast.CallExpr)
		if !ok || !isConversion(info, c) || len(c.Args) != 1 {
			break
		}
		arg = c.Args[0]
	}
	var obj types.Object
	switch e := arg.(type) {
	case *ast.Ident:
		obj = info.Uses[e]
	case *ast.SelectorExpr:
		if sel := info.Selections[e]; sel != nil {
			if sel.Kind() == types.FieldVal {
				return nil
			}
			obj = sel.Obj()
		} else {
			obj = info.Uses[e.Sel]
		}
	}
	fn, _ := obj.(*types.Func)
	return fn
}

func isConversion(info *types.Info, call *ast.CallExpr) bool {
	tv, ok := info.Types[call.Fun]
	return ok && tv.IsType()
}

func calleeName(fun ast.Expr) string {
	switch f := ast.Unparen(fun).(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.IndexExpr:
		return calleeName(f.X)
	case *ast.IndexListExpr:
		return calleeName(f.X)
	}
	return types.ExprString(fun)
}

func inModule(pkgPath, mod string) bool {
	return pkgPath == mod || strings.HasPrefix(pkgPath, mod+"/")
}

// Display names fn like symbols.DisplayName: "pkg.Name", "pkg.(*T).Name"
// or "pkg.T.Name", with the package clause name.
func Display(fn *types.Func) string {
	pkg := fn.Pkg().Name()
	sig, _ := fn.Type().(*types.Signature)
	if sig == nil || sig.Recv() == nil {
		return pkg + "." + fn.Name()
	}
	t := sig.Recv().Type()
	ptr := false
	if p, ok := t.(*types.Pointer); ok {
		t, ptr = p.Elem(), true
	}
	name := types.TypeString(t, func(*types.Package) string { return "" })
	if n, ok := t.(*types.Named); ok {
		name = n.Obj().Name()
		if tp := n.TypeParams(); tp != nil && tp.Len() > 0 {
			var ps []string
			for i := range tp.Len() {
				ps = append(ps, tp.At(i).Obj().Name())
			}
			name += "[" + strings.Join(ps, ", ") + "]"
		}
	}
	if ptr {
		return pkg + ".(*" + name + ")." + fn.Name()
	}
	return pkg + "." + name + "." + fn.Name()
}
