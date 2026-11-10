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
// its callee is a verb and its first argument a path literal (one starting
// with "/", a method pattern "GET /x", or "" under a resolved "/" prefix),
// with prefixes resolved inside enclosing (an *ast.FuncDecl or *ast.FuncLit).
// gin's Handle("GET", "/x", h) renders as "GET /x". ok is false when it is
// not a route or a prefix cannot be resolved without guessing; callers then
// use their call text. Render does not judge whether a verb-named call is a
// registration at all: callers pass only registration-shaped calls.
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
	lit, hasLit := firstLiteral(call)
	if !hasLit {
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
		if verbs[strings.ToUpper(lit)] && len(call.Args) >= 2 {
			// gin: Handle("GET", "/x", h)
			p, isLit := stringLit(call.Args[1])
			if !isLit {
				return "", false
			}
			verb, lit = strings.ToUpper(lit), p
		} else if lower != "match" && hasMethodPrefix(lit) {
			return lit, true
		}
	default:
		return "", false
	}
	// Only a path is a route: "email:deliver", "user.created" or a cache key
	// passed to a verb-named call are not. The empty path needs a prefix
	// that is one.
	if lit != "" && !strings.HasPrefix(lit, "/") {
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
	if lit == "" && !strings.HasPrefix(path.Join(prefixes...), "/") {
		return "", false
	}
	p := join(prefixes, lit)
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
func firstLiteral(call *ast.CallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	return stringLit(call.Args[0])
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
	aliases := 0     // plain "h := g" steps, which count toward maxPrefixes too
	for {
		if len(rev)+aliases > maxPrefixes {
			return nil, false
		}
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
			continue
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok {
				return reverse(rev), true // newGroup(): a call result is a root
			}
			switch {
			case sel.Sel.Name == "Subrouter" && len(x.Args) == 0:
				e = sel.X
			case prefixMethods[sel.Sel.Name]:
				lit, ok := firstLiteral(x)
				if !ok {
					return nil, false
				}
				rev = append(rev, lit)
				e = sel.X
			default:
				// pkg.New() is a root; anything called on a local or on another
				// call (.Use(mw), .With(mw)) may change the prefix: undecided.
				if id, isID := sel.X.(*ast.Ident); !isID || r.isLocal(id.Name) {
					return nil, false
				}
				return reverse(rev), true
			}
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
				return reverse(rev), len(rev)+aliases <= maxPrefixes
			}
			if _, alias := next.expr.(*ast.Ident); alias {
				aliases++
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
// variable assigned once from a prefix expression or from another variable
// (followed as the next step), or a chain root.
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
		p, isStr := firstLiteral(call)
		if !isStr {
			return step{}, nil, false, false
		}
		return step{expr: sel.X, limit: i - 1}, &p, false, true
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
	case n == 0 || rhs == nil:
		return step{}, nil, true, true
	}
	switch x := ast.Unparen(rhs).(type) {
	case *ast.CallExpr:
		return step{expr: x, limit: limit}, nil, false, true
	case *ast.Ident:
		// h := g: g's prefixes are h's. A self-reference cannot be followed.
		if x.Name == id.Name {
			return step{}, nil, false, false
		}
		return step{expr: x, limit: limit}, nil, false, true
	}
	return step{}, nil, true, true
}

// isLocal reports whether name is a parameter, receiver or assigned
// variable anywhere in the enclosing function.
func (r resolver) isLocal(name string) bool {
	local := false
	fields := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				local = local || n.Name == name
			}
		}
	}
	if d, ok := r.enclosing.(*ast.FuncDecl); ok {
		fields(d.Recv)
	}
	ast.Inspect(r.enclosing, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.FuncType:
			fields(s.Params)
		case *ast.AssignStmt:
			for _, l := range s.Lhs {
				if id, ok := l.(*ast.Ident); ok && id.Name == name {
					local = true
				}
			}
		case *ast.ValueSpec:
			for _, id := range s.Names {
				local = local || id.Name == name
			}
		}
		return !local
	})
	return local
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
