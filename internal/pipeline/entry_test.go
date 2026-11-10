package pipeline

import (
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/callgraph"
	"github.com/Kevinbarrero/mr-impact/internal/load"
)

func TestEntryRoot(t *testing.T) {
	prog := &load.Program{Generated: map[string]bool{"lib/gen.go": true}}
	pred := entryRoot(prog)
	cases := []struct {
		name string
		f    callgraph.FuncInfo
		want bool
	}{
		{"exported func, importable", callgraph.FuncInfo{Pkg: "m/lib", PkgName: "lib", Name: "Do", File: "lib/a.go"}, true},
		{"unexported func", callgraph.FuncInfo{Pkg: "m/lib", PkgName: "lib", Name: "do", File: "lib/a.go"}, false},
		{"exported func, internal", callgraph.FuncInfo{Pkg: "m/internal/x", PkgName: "x", Name: "Do", File: "internal/x/a.go"}, false},
		{"exported func, main", callgraph.FuncInfo{Pkg: "m/cmd", PkgName: "main", Name: "Do", File: "cmd/a.go"}, false},
		{"exported method, exported type", callgraph.FuncInfo{Pkg: "m/lib", PkgName: "lib", Name: "Do", Recv: "T", Method: true, File: "lib/a.go"}, true},
		{"exported method, unexported type", callgraph.FuncInfo{Pkg: "m/lib", PkgName: "lib", Name: "Do", Recv: "t", Method: true, File: "lib/a.go"}, false},
		{"exported method, internal", callgraph.FuncInfo{Pkg: "m/internal/x", PkgName: "x", Name: "Do", Recv: "T", Method: true, File: "internal/x/a.go"}, false},
		{"generated", callgraph.FuncInfo{Pkg: "m/lib", PkgName: "lib", Name: "Do", File: "lib/gen.go"}, false},
	}
	for _, c := range cases {
		if got := pred(c.f); got != c.want {
			t.Errorf("%s: entryRoot = %v, want %v", c.name, got, c.want)
		}
	}
	// Dead keeps its wider root: an exported method of an internal type is
	// still API there (it may implement a dependency's interface).
	if !apiRoot(prog)(callgraph.FuncInfo{Pkg: "m/internal/x", PkgName: "x", Name: "String", Recv: "t", Method: true, File: "internal/x/a.go"}) {
		t.Error("apiRoot changed")
	}
}
