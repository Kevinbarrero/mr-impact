package pipeline

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// use is an object named on a deleted line, resolved with the BASE types:
// a function (Recv ""), a method (Recv = receiver type name) or a struct
// field (Field true).
type use struct {
	Pkg, Recv, Name string
	Field           bool
}

// anyRecv marks a method called through an interface.
const anyRecv = "?"

// deletedUses resolves every identifier on the deleted lines (by
// repo-relative BASE file) of the base packages to the function, method or
// field it names.
func deletedUses(pkgs []*packages.Package, rel func(string) (string, bool), lines map[string]map[int]bool) map[use]bool {
	out := map[use]bool{}
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, f := range pkg.Syntax {
			file, ok := rel(pkg.Fset.Position(f.Package).Filename)
			on := lines[file]
			if !ok || on == nil {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok || !on[pkg.Fset.Position(id.Pos()).Line] {
					return true
				}
				if u, ok := useOf(pkg.TypesInfo.Uses[id]); ok {
					out[u] = true
				}
				return true
			})
		}
	}
	return out
}

func useOf(obj types.Object) (use, bool) {
	switch o := obj.(type) {
	case *types.Func:
		o = o.Origin()
		if o.Pkg() == nil {
			return use{}, false
		}
		u := use{Pkg: o.Pkg().Path(), Name: o.Name()}
		if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
			t := sig.Recv().Type()
			if p, ok := t.(*types.Pointer); ok {
				t = p.Elem()
			}
			named, ok := t.(*types.Named)
			if !ok {
				return use{}, false
			}
			if types.IsInterface(named) {
				// Called through an interface: the concrete method is
				// unknown, so any method of that name matches.
				return use{Recv: anyRecv, Name: o.Name()}, true
			}
			u.Recv = named.Obj().Name()
		}
		return u, true
	case *types.Var:
		if o.IsField() && o.Pkg() != nil {
			return use{Pkg: o.Pkg().Path(), Name: o.Name(), Field: true}, true
		}
	}
	return use{}, false
}

// isProtobuf reports protoc-generated files: message code is a contract
// shared with other services, never this repo's to trim.
func isProtobuf(file string) bool {
	return strings.HasSuffix(file, ".pb.go")
}
