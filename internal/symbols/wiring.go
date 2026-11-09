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
func isWiring(fd *ast.FuncDecl) bool {
	if fd.Body == nil || len(fd.Body.List) == 0 {
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
	}
	return false
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
