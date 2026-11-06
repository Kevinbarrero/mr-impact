// Package callgraph builds a VTA call graph over the loaded program and
// answers "who calls this function" (reverse reachability) queries.
package callgraph

import (
	"context"
	"go/ast"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	gocg "golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"gitlab.com/wh1t3hat/mrimpact/internal/load"
	"gitlab.com/wh1t3hat/mrimpact/internal/symbols"
)

type Caller struct {
	Display string // same format as symbols.ChangedFunc.Display
	Pkg     string // import path
	Depth   int
	Via     string // Display of the callee one level nearer the changed function
}

type Impact struct {
	Found       bool     // an SSA function exists at File:StartLine
	Callers     []Caller // non-test callers up to depth, nearest depth first then Display; deduped by function
	TestCallers int      // distinct test funcs among callers up to depth
	TestReached bool     // some Test/Benchmark/Fuzz/Example func in a _test.go file reaches it (unbounded depth)
}

// node is one source-level function: test variants and generic instances of
// the same declaration share a node; each synthetic function (wrapper,
// thunk, package init) gets its own node, which is traversed transparently.
type node struct {
	display, pkg string
	test         bool // declared in a _test.go file
	testEntry    bool // Test/Benchmark/Fuzz/Example func in a _test.go file
	synthetic    bool
}

type pos struct {
	file string
	line int
}

type Graph struct {
	nodes []node
	in    []map[int]bool // in[callee] = set of caller node ids
	byPos map[pos]int
}

const external = -1 // function outside the repo (e.g. generated test main)

type indexer struct {
	p    *load.Program
	g    *Graph
	ids  map[*ssa.Function]int
	keys map[[2]string]int
}

// Build computes the VTA call graph of p.SSA and indexes it. It returns
// ctx.Err() if ctx is done before the graph is ready.
func Build(ctx context.Context, p *load.Program) (*Graph, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type result struct {
		funcs map[*ssa.Function]bool
		cg    *gocg.Graph
	}
	done := make(chan result, 1)
	go func() {
		funcs := ssautil.AllFunctions(p.SSA)
		done <- result{funcs, vta.CallGraph(funcs, cha.CallGraph(p.SSA))}
	}()
	var r result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r = <-done:
	}

	x := &indexer{p: p, g: &Graph{byPos: map[pos]int{}}, ids: map[*ssa.Function]int{}, keys: map[[2]string]int{}}
	for fn := range r.funcs {
		if fn.Parent() != nil {
			continue
		}
		id := x.id(fn)
		if id == external || x.g.nodes[id].synthetic {
			continue
		}
		at := p.Fset.Position(fn.Syntax().Pos())
		if file, ok := p.Rel(at.Filename); ok {
			x.g.byPos[pos{file, at.Line}] = id
		}
	}
	for fn, n := range r.cg.Nodes {
		if fn == nil {
			continue
		}
		callee := x.id(fn)
		if callee == external {
			continue
		}
		for _, e := range n.In {
			caller := x.id(e.Caller.Func)
			if caller != external && caller != callee {
				x.g.in[callee][caller] = true
			}
		}
	}
	return x.g, nil
}

// id returns the node of fn's top-level declaration: closures map to their
// outermost enclosing function, generic instances to their origin.
func (x *indexer) id(fn *ssa.Function) int {
	if id, ok := x.ids[fn]; ok {
		return id
	}
	root := fn
	for root.Parent() != nil {
		root = root.Parent()
	}
	if o := root.Origin(); o != nil {
		root = o
	}
	id := x.newID(root)
	x.ids[fn] = id
	return id
}

func (x *indexer) newID(fn *ssa.Function) int {
	fd, ok := fn.Syntax().(*ast.FuncDecl)
	if !ok || fn.Pkg == nil {
		return x.add(node{display: fn.String(), synthetic: true})
	}
	file, ok := x.p.Rel(x.p.Fset.Position(fd.Pos()).Filename)
	if !ok {
		return external
	}
	n := node{display: symbols.DisplayName(fn.Pkg.Pkg.Name(), fd), pkg: fn.Pkg.Pkg.Path()}
	key := [2]string{n.display, n.pkg}
	if id, ok := x.keys[key]; ok {
		return id
	}
	n.test = strings.HasSuffix(file, "_test.go")
	n.testEntry = n.test && fd.Recv == nil && isTestEntry(fd.Name.Name)
	id := x.add(n)
	x.keys[key] = id
	return id
}

func (x *indexer) add(n node) int {
	x.g.nodes = append(x.g.nodes, n)
	x.g.in = append(x.g.in, map[int]bool{})
	return len(x.g.nodes) - 1
}

// isTestEntry reports whether name is a go test entry point name.
func isTestEntry(name string) bool {
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if len(name) == len(prefix) {
			return true
		}
		r, _ := utf8.DecodeRuneInString(name[len(prefix):])
		if !unicode.IsLower(r) {
			return true
		}
	}
	return false
}

// Impact returns the callers of the function declared at file:startLine
// (repo-relative file, line of the func keyword) up to depth levels.
func (g *Graph) Impact(file string, startLine, depth int) Impact {
	target, ok := g.byPos[pos{file, startLine}]
	if !ok {
		return Impact{}
	}
	im := Impact{Found: true}
	depthOf := map[int]int{target: 0}
	via := map[int]string{}
	frontier := []int{target}
	for d := 1; len(frontier) > 0; d++ {
		var next []int
		for _, u := range frontier {
			for _, c := range g.callers(u) {
				cd, seen := depthOf[c]
				switch {
				case !seen:
					depthOf[c], via[c] = d, g.nodes[u].display
					next = append(next, c)
				case cd == d && g.nodes[u].display < via[c]:
					via[c] = g.nodes[u].display
				}
			}
		}
		frontier = next
	}

	for id, d := range depthOf {
		n := g.nodes[id]
		if id == target {
			continue
		}
		if n.testEntry {
			im.TestReached = true
		}
		if d > depth {
			continue
		}
		switch {
		case n.testEntry:
			im.TestCallers++
		case !n.test:
			im.Callers = append(im.Callers, Caller{Display: n.display, Pkg: n.pkg, Depth: d, Via: via[id]})
		}
	}
	sort.Slice(im.Callers, func(i, j int) bool {
		a, b := im.Callers[i], im.Callers[j]
		if a.Depth != b.Depth {
			return a.Depth < b.Depth
		}
		if a.Display != b.Display {
			return a.Display < b.Display
		}
		return a.Pkg < b.Pkg
	})
	return im
}

// callers returns the non-synthetic direct callers of u, looking through
// synthetic functions.
func (g *Graph) callers(u int) []int {
	var out []int
	seen := map[int]bool{u: true}
	var walk func(int)
	walk = func(v int) {
		for c := range g.in[v] {
			if seen[c] {
				continue
			}
			seen[c] = true
			if g.nodes[c].synthetic {
				walk(c)
			} else {
				out = append(out, c)
			}
		}
	}
	walk(u)
	sort.Ints(out)
	return out
}
