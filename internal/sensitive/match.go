package sensitive

import (
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"
)

// api is a resolved callee: a package-level function or type (recv empty) or
// a method of the named type recv, declared in pkg.
type api struct {
	pkg  string
	recv string
	ptr  bool // pointer receiver
	name string
}

func (a api) String() string {
	switch {
	case a.recv == "":
		return a.pkg + "." + a.name
	case a.ptr:
		return a.pkg + ".(*" + a.recv + ")." + a.name
	default:
		return a.pkg + ".(" + a.recv + ")." + a.name
	}
}

// Site is one use of a sensitive API: the line span of the call expression
// (a reference that is not a call, such as a method value, spans its own
// identifier).
type Site struct {
	API      string
	From, To int
}

// Calls returns the APIs fn touches: resolved callees of calls, method values
// and composite literal types in fn's body and every nested function literal,
// rendered "pkgpath.Name" / "pkgpath.(*T).M" / "pkgpath.(T).M", deduplicated,
// in first-occurrence source order.
func (t Table) Calls(fn *ast.FuncDecl, info *types.Info) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range t.Sites(fn, info, token.NewFileSet()) {
		if !seen[s.API] {
			seen[s.API] = true
			out = append(out, s.API)
		}
	}
	return out
}

// Sites returns every use of a table API in fn, in source order, one per
// use, with the line span of the whole call expression so an added argument
// line of a multi-line call counts as touching it.
func (t Table) Sites(fn *ast.FuncDecl, info *types.Info, fset *token.FileSet) []Site {
	if fn == nil || fn.Body == nil || info == nil || len(t.patterns) == 0 {
		return nil
	}
	var out []Site
	callOf := map[*ast.Ident]ast.Node{} // callee identifier -> its call
	add := func(a api, ok bool, span ast.Node) {
		if !ok || !t.match(a) {
			return
		}
		out = append(out, Site{
			API:  a.String(),
			From: fset.Position(span.Pos()).Line,
			To:   fset.Position(span.End()).Line,
		})
	}
	// ast.Inspect descends into *ast.FuncLit bodies, so nested closures are
	// covered with no extra work.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			// Every function or method reference: a call, a method value,
			// a method expression or a function value. Uses also records the
			// Sel of a selector, so x.M lands here.
			var span ast.Node = n
			if c, ok := callOf[n]; ok {
				span = c
			}
			a, ok := objectAPI(info.Uses[n])
			add(a, ok, span)
		case *ast.CallExpr:
			if id := calleeIdent(n.Fun); id != nil {
				callOf[id] = n
			}
			// A conversion, e.g. unsafe.Pointer(p).
			if tv, ok := info.Types[n.Fun]; ok && tv.IsType() {
				a, ok := typeAPI(tv.Type)
				add(a, ok, n)
			}
		case *ast.CompositeLit:
			a, ok := typeAPI(info.TypeOf(n))
			add(a, ok, n)
		}
		return true
	})
	return out
}

// calleeIdent is the identifier naming the function a call's Fun denotes.
func calleeIdent(e ast.Expr) *ast.Ident {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.SelectorExpr:
			return x.Sel
		case *ast.Ident:
			return x
		default:
			return nil
		}
	}
}

func (t Table) match(a api) bool {
	for _, p := range t.patterns {
		if p.matches(a) {
			return true
		}
	}
	return false
}

// objectAPI resolves a function, method or package builtin (unsafe.Sizeof).
// Variables, constants and type names are not callees.
func objectAPI(obj types.Object) (api, bool) {
	switch obj := obj.(type) {
	case *types.Builtin:
		if obj.Pkg() == nil { // universe builtins: len, append, …
			return api{}, false
		}
		return api{pkg: obj.Pkg().Path(), name: obj.Name()}, true
	case *types.Func:
		obj = obj.Origin()
		if obj.Pkg() == nil { // error.Error
			return api{}, false
		}
		recv := obj.Signature().Recv()
		if recv == nil {
			return api{pkg: obj.Pkg().Path(), name: obj.Name()}, true
		}
		// The declared receiver: for a promoted method it is the embedded
		// type that declares it, for an interface method the interface.
		rt := types.Unalias(recv.Type())
		ptr := false
		if p, ok := rt.(*types.Pointer); ok {
			rt, ptr = types.Unalias(p.Elem()), true
		}
		named, ok := rt.(*types.Named)
		if !ok || named.Obj().Pkg() == nil { // a method of an unnamed interface
			return api{}, false
		}
		named = named.Origin()
		return api{pkg: named.Obj().Pkg().Path(), recv: named.Obj().Name(), ptr: ptr, name: obj.Name()}, true
	}
	return api{}, false
}

// typeAPI resolves the named type of a composite literal or conversion.
func typeAPI(t types.Type) (api, bool) {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok { // an elided &T{…} element
		t = types.Unalias(p.Elem())
	}
	if b, ok := t.(*types.Basic); ok && b.Kind() == types.UnsafePointer {
		return api{pkg: "unsafe", name: "Pointer"}, true
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return api{}, false
	}
	obj := named.Origin().Obj()
	return api{pkg: obj.Pkg().Path(), name: obj.Name()}, true
}

// Critical reports the first glob (in order) matching pkgPath; a glob without
// the module prefix is joined to module; "/..." matches the package and
// everything below.
func Critical(pkgPath, module string, globs []string) (glob string, ok bool) {
	for _, g := range globs {
		full := g
		if g != module && !strings.HasPrefix(g, module+"/") {
			full = module + "/" + g
		}
		if base, tree := strings.CutSuffix(full, "/..."); tree {
			if pkgPath == base || strings.HasPrefix(pkgPath, base+"/") {
				return g, true
			}
			continue
		}
		if m, err := path.Match(full, pkgPath); err == nil && m {
			return g, true
		}
	}
	return "", false
}
