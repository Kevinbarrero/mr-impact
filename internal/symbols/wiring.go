package symbols

import (
	"go/ast"
	"go/token"
	"strings"
)

// isWiring reports whether fd is straight-line setup code: calls,
// assignments, returns and `if x != nil` error exits, nothing else.
// Function literals are never inspected; their bodies run later.
func isWiring(fd *ast.FuncDecl) bool {
	if fd.Body == nil || len(fd.Body.List) == 0 {
		return false
	}
	for _, s := range fd.Body.List {
		if !wiringStmt(s) {
			return false
		}
	}
	return true
}

func wiringStmt(s ast.Stmt) bool {
	switch s := s.(type) {
	case *ast.ExprStmt:
		_, ok := s.X.(*ast.CallExpr)
		return ok
	case *ast.AssignStmt, *ast.ReturnStmt:
		return true
	case *ast.DeclStmt:
		gd, ok := s.Decl.(*ast.GenDecl)
		return ok && gd.Tok == token.VAR
	case *ast.IfStmt:
		return wiringIf(s)
	}
	return false
}

// wiringIf accepts `if [init;] x != nil { [calls/assigns;] terminating }`
// with no else.
func wiringIf(s *ast.IfStmt) bool {
	if s.Else != nil || !notNilCheck(s.Cond) {
		return false
	}
	if s.Init != nil {
		if _, ok := s.Init.(*ast.AssignStmt); !ok {
			return false
		}
	}
	body := s.Body.List
	if len(body) == 0 {
		return false
	}
	for _, b := range body[:len(body)-1] {
		switch b := b.(type) {
		case *ast.ExprStmt:
			if _, ok := b.X.(*ast.CallExpr); !ok {
				return false
			}
		case *ast.AssignStmt:
		default:
			return false
		}
	}
	return terminating(body[len(body)-1])
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
func terminating(s ast.Stmt) bool {
	switch s := s.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.ExprStmt:
		call, ok := s.X.(*ast.CallExpr)
		if !ok {
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
