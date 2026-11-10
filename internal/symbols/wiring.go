package symbols

import (
	"go/ast"
	"go/token"
	"strings"
)

// isWiring reports whether fd constructs and connects but computes nothing:
// calls, assignments, returns and `if x != nil` error exits whose
// expressions are all plumbing (identifiers, selectors, literals, calls,
// composite literals, &, *, parentheses, function literals), with at
// least one call or composite literal. A function literal's body is held
// to the same rules: a closure that computes makes fd compute too, and its
// calls and composite literals count.
//
// A function with a context.Context parameter (receiver excluded) is never
// wiring: it runs per request, not once at startup. imports maps fd's
// file's import names to paths (fileImports); a dot-imported context is
// not resolved.
func isWiring(fd *ast.FuncDecl, imports map[string]string) bool {
	if fd.Body == nil || len(fd.Body.List) == 0 || takesContext(fd, imports) {
		return false
	}
	w := &wiringCheck{}
	for _, s := range fd.Body.List {
		if !w.stmt(s) {
			return false
		}
	}
	return w.built
}

func takesContext(fd *ast.FuncDecl, imports map[string]string) bool {
	if fd.Type.Params == nil {
		return false
	}
	for _, p := range fd.Type.Params.List {
		sel, ok := p.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Context" {
			continue
		}
		if id, ok := sel.X.(*ast.Ident); ok && imports[id.Name] == "context" {
			return true
		}
	}
	return false
}

type wiringCheck struct {
	built bool // saw a call or composite literal
}

func (w *wiringCheck) exprs(es []ast.Expr) bool {
	for _, e := range es {
		if !w.expr(e) {
			return false
		}
	}
	return true
}

func (w *wiringCheck) expr(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ident, *ast.BasicLit:
		return true
	case *ast.FuncLit:
		for _, s := range e.Body.List {
			if !w.stmt(s) {
				return false
			}
		}
		return true
	case *ast.SelectorExpr:
		return w.expr(e.X)
	case *ast.ParenExpr:
		return w.expr(e.X)
	case *ast.StarExpr:
		return w.expr(e.X)
	case *ast.UnaryExpr:
		return e.Op == token.AND && w.expr(e.X)
	case *ast.CallExpr:
		w.built = true
		switch e.Fun.(type) {
		case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType:
		default:
			if !w.expr(e.Fun) {
				return false
			}
		}
		// make and new take a type as their first argument.
		if id, ok := e.Fun.(*ast.Ident); ok && (id.Name == "make" || id.Name == "new") && len(e.Args) > 0 {
			return w.typ(e.Args[0]) && w.exprs(e.Args[1:])
		}
		return w.exprs(e.Args)
	case *ast.CompositeLit:
		w.built = true
		for _, el := range e.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if !w.expr(kv.Key) || !w.expr(kv.Value) {
					return false
				}
				continue
			}
			if !w.expr(el) {
				return false
			}
		}
		return true
	case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType, *ast.StructType, *ast.Ellipsis:
		// Type expressions only appear here as make/new type arguments.
		return w.typ(e)
	}
	return false
}

// typ accepts a type expression. A generic instantiation (IndexExpr,
// IndexListExpr) is accepted only here, where it is a type: in a value
// position s[i] is an index, which expr rejects. Other parts go to expr.
func (w *wiringCheck) typ(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.ArrayType:
		return (e.Len == nil || w.expr(e.Len)) && w.typ(e.Elt)
	case *ast.MapType:
		return w.typ(e.Key) && w.typ(e.Value)
	case *ast.ChanType:
		return w.typ(e.Value)
	case *ast.Ellipsis:
		return e.Elt == nil || w.typ(e.Elt)
	case *ast.FuncType, *ast.InterfaceType, *ast.StructType:
		return true
	case *ast.StarExpr:
		return w.typ(e.X)
	case *ast.ParenExpr:
		return w.typ(e.X)
	case *ast.IndexExpr:
		return w.typ(e.X) && w.typ(e.Index)
	case *ast.IndexListExpr:
		if !w.typ(e.X) {
			return false
		}
		for _, ix := range e.Indices {
			if !w.typ(ix) {
				return false
			}
		}
		return true
	}
	return w.expr(e)
}

func (w *wiringCheck) stmt(s ast.Stmt) bool {
	switch s := s.(type) {
	case *ast.ExprStmt:
		_, ok := s.X.(*ast.CallExpr)
		return ok && w.expr(s.X)
	case *ast.AssignStmt:
		// Compound assignment (+=, <<=, ...) computes.
		if s.Tok != token.ASSIGN && s.Tok != token.DEFINE {
			return false
		}
		return w.exprs(s.Lhs) && w.exprs(s.Rhs)
	case *ast.ReturnStmt:
		return w.exprs(s.Results)
	case *ast.DeclStmt:
		gd, ok := s.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			return false
		}
		for _, sp := range gd.Specs {
			if vs, ok := sp.(*ast.ValueSpec); ok && !w.exprs(vs.Values) {
				return false
			}
		}
		return true
	case *ast.IfStmt:
		return w.ifStmt(s)
	}
	return false
}

// ifStmt accepts `if [init;] x != nil { [calls/assigns;] terminating }`
// with no else.
func (w *wiringCheck) ifStmt(s *ast.IfStmt) bool {
	if s.Else != nil || !notNilCheck(s.Cond) {
		return false
	}
	if s.Init != nil {
		a, ok := s.Init.(*ast.AssignStmt)
		if !ok || !w.stmt(a) {
			return false
		}
	}
	body := s.Body.List
	if len(body) == 0 {
		return false
	}
	for _, b := range body[:len(body)-1] {
		switch b.(type) {
		case *ast.ExprStmt, *ast.AssignStmt:
			if !w.stmt(b) {
				return false
			}
		default:
			return false
		}
	}
	return w.terminating(body[len(body)-1])
}

func notNilCheck(e ast.Expr) bool {
	b, ok := e.(*ast.BinaryExpr)
	if !ok || b.Op != token.NEQ {
		return false
	}
	isNil := func(e ast.Expr) bool { id, ok := e.(*ast.Ident); return ok && id.Name == "nil" }
	isIdent := func(e ast.Expr) bool { _, ok := e.(*ast.Ident); return ok }
	return (isIdent(b.X) && isNil(b.Y)) || (isNil(b.X) && isIdent(b.Y))
}

// terminating: a return, or a call named panic, Fatal* or Exit (matched on
// the called name only).
func (w *wiringCheck) terminating(s ast.Stmt) bool {
	switch s := s.(type) {
	case *ast.ReturnStmt:
		return w.stmt(s)
	case *ast.ExprStmt:
		call, ok := s.X.(*ast.CallExpr)
		if !ok || !w.expr(call) {
			return false
		}
		var name string
		switch f := call.Fun.(type) {
		case *ast.Ident:
			name = f.Name
		case *ast.SelectorExpr:
			name = f.Sel.Name
		}
		return name == "panic" || name == "Exit" || strings.HasPrefix(name, "Fatal")
	}
	return false
}
