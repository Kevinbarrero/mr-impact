// Package route renders a route-registration call such as api.POST("", h) as
// the route it registers ("POST /trade"). It works on syntax alone: group
// prefixes are followed only through local variables assigned once in the
// enclosing function, and anything ambiguous is reported as not a route so
// callers keep the call text.
package route

import (
	"go/ast"
	"go/token"
	"path"
	"strconv"
	"strings"
)

// maxPrefixes is how many prefix levels are followed before giving up.
const maxPrefixes = 4

var verbs = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	"HEAD": true, "OPTIONS": true, "CONNECT": true, "TRACE": true,
}

// pathOnly callees render the path without a verb.
var pathOnly = map[string]bool{"handle": true, "handlefunc": true, "match": true}

// prefixMethods open a sub-router whose path is prepended to its routes.
var prefixMethods = map[string]bool{"Group": true, "Route": true, "PathPrefix": true, "Prefix": true}

// Render returns the route form of call ("POST /trade", "/x", "GET /x") when
// its callee is a verb and its first argument a string literal, with
// prefixes resolved inside enclosing (an *ast.FuncDecl or *ast.FuncLit).
// ok is false when it is not a route or a prefix cannot be resolved without
// guessing; callers then use their call text.
func Render(call *ast.CallExpr, enclosing ast.Node) (route string, ok bool) {
	var name string
	var recv ast.Expr
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		name, recv = fun.Sel.Name, fun.X
	case *ast.Ident:
		name = fun.Name
	default:
		return "", false
	}
	lits := firstLiteral(call)
	if lits == nil {
		return "", false
	}
	lower := strings.ToLower(name)
	upper := strings.ToUpper(name)
	var verb string
	switch {
	case verbs[upper]:
		verb = upper
	case lower == "any":
		verb = "ANY"
	case pathOnly[lower]:
		if lower != "match" && hasMethodPrefix(*lits) {
			return *lits, true
		}
	default:
		return "", false
	}

	var prefixes []string
	if recv != nil {
		r := resolver{enclosing: enclosing, stack: stackTo(enclosing, call)}
		var good bool
		if prefixes, good = r.prefixes(recv, len(r.stack)); !good {
			return "", false
		}
	}
	p := join(prefixes, *lits)
	if verb == "" {
		return p, true
	}
	return verb + " " + p, true
}

// Literals returns call's string-literal arguments, unquoted, in order.
func Literals(call *ast.CallExpr) []string {
	var out []string
	for _, a := range call.Args {
		if s, ok := stringLit(a); ok {
			out = append(out, s)
		}
	}
	return out
}

func stringLit(e ast.Expr) (string, bool) {
	b, ok := e.(*ast.BasicLit)
	if !ok || b.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(b.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// firstLiteral returns call's first argument when it is a string literal.
func firstLiteral(call *ast.CallExpr) *string {
	if len(call.Args) == 0 {
		return nil
	}
	s, ok := stringLit(call.Args[0])
	if !ok {
		return nil
	}
	return &s
}

// hasMethodPrefix reports whether s starts with an HTTP method and a space
// ("GET /x"), the net/http pattern form.
func hasMethodPrefix(s string) bool {
	m, _, found := strings.Cut(s, " ")
	return found && verbs[m]
}

// join joins prefixes and the final literal like path.Join, keeping the
// trailing slash of the last non-empty part; the empty path is the prefix
// itself and the root renders "/".
func join(prefixes []string, last string) string {
	parts := append(append([]string{}, prefixes...), last)
	joined := path.Join(parts...)
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	tail := last
	if tail == "" && len(prefixes) > 0 {
		tail = prefixes[len(prefixes)-1]
	}
	if strings.HasSuffix(tail, "/") && !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	return joined
}

// stackTo returns the nodes from root down to (excluding) target.
func stackTo(root, target ast.Node) []ast.Node {
	var stack, found []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if n == target {
			found = append([]ast.Node{}, stack...)
			return false
		}
		stack = append(stack, n)
		return true
	})
	return found
}

type resolver struct {
	enclosing ast.Node
	stack     []ast.Node // ancestors of the registration call
}

// prefixes resolves the group prefixes behind receiver e, outermost first.
// limit is how much of the ancestor stack is visible to e. ok is false when
// the receiver is ambiguous or deeper than maxPrefixes.
func (r resolver) prefixes(e ast.Expr, limit int) ([]string, bool) {
	var rev []string // innermost first
	for {
		if len(rev) > maxPrefixes {
			return nil, false
		}
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
			continue
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok {
				return reverse(rev), len(rev) <= maxPrefixes
			}
			if sel.Sel.Name == "Subrouter" && len(x.Args) == 0 {
				e = sel.X
				continue
			}
			lit := firstLiteral(x)
			if !prefixMethods[sel.Sel.Name] || lit == nil {
				return reverse(rev), len(rev) <= maxPrefixes
			}
			rev = append(rev, *lit)
			e = sel.X
			continue
		case *ast.Ident:
			next, add, root, ok := r.identStep(x, limit)
			if !ok {
				return nil, false
			}
			if add != nil {
				rev = append(rev, *add)
			}
			if root {
				return reverse(rev), len(rev) <= maxPrefixes
			}
			e = next.expr
			limit = next.limit
			continue
		default:
			return reverse(rev), len(rev) <= maxPrefixes
		}
	}
}

type step struct {
	expr  ast.Expr
	limit int
}

// identStep resolves one local variable: a Route closure parameter, a
// variable assigned once from a prefix expression, or a chain root.
func (r resolver) identStep(id *ast.Ident, limit int) (next step, add *string, root, ok bool) {
	// A parameter of a func literal passed to X.Route("lit", func(p) {...}).
	for i := limit - 1; i >= 1; i-- {
		lit, isLit := r.stack[i].(*ast.FuncLit)
		if !isLit || !hasParam(lit, id.Name) {
			continue
		}
		call, isCall := r.stack[i-1].(*ast.CallExpr)
		sel, isSel := (*ast.SelectorExpr)(nil), false
		if isCall {
			sel, isSel = call.Fun.(*ast.SelectorExpr)
		}
		if !isSel || sel.Sel.Name != "Route" || len(call.Args) != 2 || call.Args[1] != ast.Expr(lit) {
			return step{}, nil, true, true // another kind of parameter: a root
		}
		p := firstLiteral(call)
		if p == nil {
			return step{}, nil, false, false
		}
		return step{expr: sel.X, limit: i - 1}, p, false, true
	}

	var rhs ast.Expr
	n := 0
	ast.Inspect(r.enclosing, func(node ast.Node) bool {
		switch s := node.(type) {
		case *ast.AssignStmt:
			for i, l := range s.Lhs {
				if lid, ok := l.(*ast.Ident); ok && lid.Name == id.Name {
					n++
					if len(s.Lhs) == len(s.Rhs) {
						rhs = s.Rhs[i]
					} else {
						rhs = nil
					}
				}
			}
		case *ast.ValueSpec:
			for i, nm := range s.Names {
				if nm.Name == id.Name {
					n++
					if len(s.Names) == len(s.Values) {
						rhs = s.Values[i]
					} else {
						rhs = nil
					}
				}
			}
		}
		return true
	})
	switch {
	case n > 1:
		return step{}, nil, false, false
	case n == 0 || rhs == nil || !isPrefixExpr(rhs):
		return step{}, nil, true, true
	}
	return step{expr: rhs, limit: limit}, nil, false, true
}

// isPrefixExpr reports whether e is a prefix call, optionally ending in
// Subrouter().
func isPrefixExpr(e ast.Expr) bool {
	for {
		c, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if sel.Sel.Name == "Subrouter" && len(c.Args) == 0 {
			e = sel.X
			continue
		}
		return prefixMethods[sel.Sel.Name] && firstLiteral(c) != nil
	}
}

func hasParam(lit *ast.FuncLit, name string) bool {
	for _, f := range lit.Type.Params.List {
		for _, n := range f.Names {
			if n.Name == name {
				return true
			}
		}
	}
	return false
}

func reverse(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}
