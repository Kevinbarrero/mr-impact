// Package callgraph builds a VTA call graph over the loaded program and
// answers "who calls this function" (reverse reachability) queries.
package callgraph

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	gocg "golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/Kevinbarrero/mr-impact/internal/load"
	"github.com/Kevinbarrero/mr-impact/internal/route"
	"github.com/Kevinbarrero/mr-impact/internal/symbols"
)

type Caller struct {
	Display string // same format as symbols.ChangedFunc.Display
	Pkg     string // import path
	Depth   int
	Via     string // Display of the callee one level nearer the changed function
	Kind    Kind   // how it reaches Via
}

// Kind is how a caller reaches the function one level nearer.
type Kind string

const (
	Calls     Kind = ""          // a direct (or interface) call
	Dispatch  Kind = "dispatch"  // calls it through a func value it was given
	Registers Kind = "registers" // hands it, as a value, to Via (a dispatcher)
)

type Impact struct {
	Found       bool     // an SSA function exists at File:StartLine
	Callers     []Caller // non-test callers up to depth, nearest depth first then Display; deduped by function
	TestCallers int      // distinct test funcs among callers up to depth
	TestReached bool     // some Test/Benchmark/Fuzz/Example func in a _test.go file reaches it (unbounded depth)
	TestPkgs    []string // sorted, distinct packages of the test entries that reach it (unbounded depth); nil if none
	Registered  []Site   // non-test places that take the function as a value (callback registration), by File then Line
}

// Site is one place where a function is used as a value rather than called.
type Site struct {
	File string // repo-relative
	Line int
	Call string // the call it is passed to, shortened, the function itself as "…" (e.g. "Subscribe(\"topic.a\", …)"); "" if not a call argument
}

// Text is the site as shown: its call, or "file:line" when it is not a call
// argument.
func (s Site) Text() string {
	if s.Call == "" {
		return fmt.Sprintf("%s:%d", s.File, s.Line)
	}
	return s.Call
}

// Unused is a function nothing reachable in the repo calls or takes as a
// value any more.
type Unused struct {
	Display, Pkg string
	File         string
	Line         int
	TestOnly     bool     // only tests still reach it
	Via          string   // non-empty: it was reached only through this other unused function
	DeadCallers  []string // callers left in non-test code, all unreachable themselves
}

// FuncInfo describes a function to the Dead callbacks: PkgName is the
// package clause name, Name the bare function or method name.
type FuncInfo struct {
	Pkg, PkgName, Name, File string
	Recv                     string // receiver type name, no pointer or type parameters; "" for a function
	Method                   bool
}

type node struct {
	display, pkg string
	name         string // bare name, "" for synthetic nodes
	recv         string // receiver type name for methods
	method       bool
	file         string
	line         int
	test         bool // declared in a _test.go file
	testEntry    bool // Test/Benchmark/Fuzz/Example func in a _test.go file
	synthetic    bool
}

// edge kinds, a bit set: a caller may reach a callee both ways.
type edge uint8

const (
	edgeDirect   edge = 1 << iota // static call or interface method call
	edgeDynamic                   // call through a func value (callback, handler)
	edgeImplicit                  // a method the standard library calls on an interface value converted here
)

type pos struct {
	file string
	line int
}

type Graph struct {
	nodes []node
	in    []map[int]edge   // in[callee][caller]
	refs  []map[int][]Site // refs[fn][referencer]: fn used as a value by referencer
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
		addDeclaredMethods(p.SSA, funcs)
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
		// A closure's callers are whoever the closure value was handed to
		// (a transaction runner, an event bus, a later lookup), not callers
		// of the function that created it: skip them. Calls the closure
		// makes still count as calls by its enclosing function.
		if fn == nil || fn.Parent() != nil {
			continue
		}
		callee := x.id(fn)
		if callee == external {
			continue
		}
		for _, e := range n.In {
			caller := x.id(e.Caller.Func)
			if caller != external && caller != callee {
				x.g.in[callee][caller] |= edgeKind(e)
			}
		}
	}
	for fn := range r.funcs {
		x.implicitEdges(fn)
	}
	for fn := range r.funcs {
		x.valueRefs(fn)
	}
	return x.g, nil
}

// addDeclaredMethods adds every method declared on a named type of a
// source package: AllFunctions skips methods of types never converted to
// an interface, and an uncalled method must still be found (Impact) and
// reported (Unused).
func addDeclaredMethods(prog *ssa.Program, funcs map[*ssa.Function]bool) {
	var add func(fn *ssa.Function)
	add = func(fn *ssa.Function) {
		if fn == nil || funcs[fn] {
			return
		}
		funcs[fn] = true
		for _, a := range fn.AnonFuncs {
			add(a)
		}
	}
	for _, pkg := range prog.AllPackages() {
		for _, m := range pkg.Members {
			t, ok := m.(*ssa.Type)
			if !ok {
				continue
			}
			named, ok := t.Type().(*types.Named)
			if !ok || named.TypeParams().Len() > 0 {
				continue
			}
			for i := range named.NumMethods() {
				if fn := prog.FuncValue(named.Method(i)); fn != nil && fn.Syntax() != nil {
					add(fn)
				}
			}
		}
	}
}

func edgeKind(e *gocg.Edge) edge {
	if e.Site == nil {
		return edgeDirect
	}
	c := e.Site.Common()
	if c.IsInvoke() || c.StaticCallee() != nil {
		return edgeDirect
	}
	return edgeDynamic
}

// implicitEdges adds an edgeImplicit from fn's node (closures count as
// their outermost function's) to every library-called method with a module
// body of each type fn converts to an interface: the standard library calls
// them inside bodies the call graph never sees (errors.Is calls Unwrap,
// net/http calls ServeHTTP).
func (x *indexer) implicitEdges(fn *ssa.Function) {
	owner := x.id(fn)
	if owner == external {
		return
	}
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			mi, ok := instr.(*ssa.MakeInterface)
			if !ok {
				continue
			}
			mset := x.p.SSA.MethodSets.MethodSet(mi.X.Type())
			for i := range mset.Len() {
				obj, ok := mset.At(i).Obj().(*types.Func)
				if !ok || !libraryCalled(obj) {
					continue
				}
				// FuncValue of the declared method, not MethodValue: a
				// value method reached through *T would be a wrapper; a
				// generic type's method is found through its origin.
				m := x.p.SSA.FuncValue(obj.Origin())
				if m == nil {
					continue
				}
				if id := x.id(m); id != external && id != owner {
					x.g.in[id][owner] |= edgeImplicit
				}
			}
		}
	}
}

// valueRefs records every function fn uses as a value (passed, stored,
// returned, bound as a method value) rather than calls.
func (x *indexer) valueRefs(fn *ssa.Function) {
	from := x.id(fn)
	if from == external {
		return
	}
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			var vals []*ssa.Value
			if call, ok := instr.(ssa.CallInstruction); ok {
				for i := range call.Common().Args {
					vals = append(vals, &call.Common().Args[i])
				}
			} else {
				vals = instr.Operands(nil)
			}
			for _, v := range vals {
				ref, ok := (*v).(*ssa.Function)
				if !ok || ref.Parent() != nil { // closures belong to fn's own node
					continue
				}
				to := x.id(ref)
				if to == external || to == from {
					continue
				}
				x.g.refs[to][from] = append(x.g.refs[to][from], x.site(fn, instr, *v))
			}
		}
	}
}

// site describes where instr uses v as a value: the call v (or the method
// value / conversion made from it) is passed to, if any.
func (x *indexer) site(fn *ssa.Function, instr ssa.Instruction, v ssa.Value) Site {
	s := Site{}
	at := x.p.Fset.Position(instr.Pos())
	call, arg, variadic := argCall(instr, v)
	if call != nil {
		at = x.p.Fset.Position(call.Pos())
	}
	if f, ok := x.p.Rel(at.Filename); ok {
		s.File = f
	}
	s.Line = at.Line
	if call != nil {
		s.Call = renderCall(fn, call, arg, variadic)
	}
	return s
}

// argCall finds the call that takes v (directly, or after it is bound as a
// method value or converted) as an argument, and the argument's index.
// variadic is true when v is an element of a slice passed to the call: the
// list built for a variadic parameter (h in g.POST("/x", mw, h)) or a slice
// literal passed as a []T argument; arg is then that slice's index.
func argCall(instr ssa.Instruction, v ssa.Value) (call ssa.CallInstruction, arg int, variadic bool) {
	cur := v
	for range 4 {
		if call, ok := instr.(ssa.CallInstruction); ok {
			for i, a := range call.Common().Args {
				if a == cur {
					return call, i, false
				}
			}
			return nil, 0, false
		}
		if st, ok := instr.(*ssa.Store); ok && st.Val == cur {
			call, arg := sliceCall(st)
			return call, arg, call != nil
		}
		next, ok := instr.(ssa.Value)
		if !ok {
			return nil, 0, false
		}
		switch instr.(type) {
		case *ssa.MakeClosure, *ssa.ChangeType, *ssa.MakeInterface, *ssa.Convert:
		default:
			return nil, 0, false
		}
		refs := next.Referrers()
		if refs == nil || len(*refs) == 0 {
			return nil, 0, false
		}
		cur, instr = next, (*refs)[0]
	}
	return nil, 0, false
}

// sliceCall follows the SSA shape of a slice literal or variadic argument
// list passed to a call: a store into an element of an array Alloc, the
// Alloc sliced once, and that slice's only use a real call (not a builtin
// such as len or append, not a range loop). It returns that call and the
// slice's argument index, or nil when the slice has any other shape, so the
// site keeps its file:line.
func sliceCall(st *ssa.Store) (ssa.CallInstruction, int) {
	ia, ok := st.Addr.(*ssa.IndexAddr)
	if !ok {
		return nil, 0
	}
	alloc, ok := ia.X.(*ssa.Alloc)
	if !ok || alloc.Referrers() == nil {
		return nil, 0
	}
	var sl *ssa.Slice
	for _, r := range *alloc.Referrers() {
		switch r := r.(type) {
		case *ssa.IndexAddr:
		case *ssa.Slice:
			if sl != nil {
				return nil, 0
			}
			sl = r
		default:
			return nil, 0
		}
	}
	if sl == nil || sl.Referrers() == nil || len(*sl.Referrers()) != 1 {
		return nil, 0
	}
	call, ok := (*sl.Referrers())[0].(ssa.CallInstruction)
	if !ok || !call.Pos().IsValid() {
		return nil, 0
	}
	if _, builtin := call.Common().Value.(*ssa.Builtin); builtin {
		return nil, 0
	}
	idx := -1
	for i, a := range call.Common().Args {
		if a == ssa.Value(sl) {
			if idx >= 0 {
				return nil, 0
			}
			idx = i
		}
	}
	if idx < 0 {
		return nil, 0
	}
	return call, idx
}

const maxCallText = 90

// renderCall prints the route call registers (route.Render), or else its
// source call expression, shortened: the callee's last name, the args as
// written, args[arg] as "…". A variadic element keeps every arg as written,
// so all module functions in one variadic list share one text.
func renderCall(fn *ssa.Function, call ssa.CallInstruction, arg int, variadic bool) string {
	root := fn
	for root.Parent() != nil {
		root = root.Parent()
	}
	syn := root.Syntax()
	if syn == nil {
		return ""
	}
	lparen := call.Common().Pos()
	var ce *ast.CallExpr
	ast.Inspect(syn, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && c.Lparen == lparen {
			ce = c
		}
		return ce == nil
	})
	if ce == nil {
		return ""
	}
	if r, ok := route.Render(ce, syn); ok {
		return r
	}
	if variadic {
		arg = -1
	}
	// SSA passes a static method call's receiver as Args[0].
	if c := call.Common(); !c.IsInvoke() && c.Signature() != nil && c.Signature().Recv() != nil {
		arg--
	}
	name := types.ExprString(ce.Fun)
	if sel, ok := ce.Fun.(*ast.SelectorExpr); ok {
		name = sel.Sel.Name
	}
	args := make([]string, len(ce.Args))
	for i, a := range ce.Args {
		args[i] = types.ExprString(a)
		if i == arg {
			args[i] = "…"
		}
	}
	if ce.Ellipsis != token.NoPos && len(args) > 0 {
		args[len(args)-1] += "..."
	}
	out := name + "(" + strings.Join(args, ", ") + ")"
	if r := []rune(out); len(r) > maxCallText {
		out = string(r[:maxCallText-1]) + "…"
	}
	return out
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
		// The generated test main package ("<pkg>.test") lists every test
		// in its initializer: its synthetic functions are test code.
		test := fn.Pkg != nil && strings.HasSuffix(fn.Pkg.Pkg.Path(), ".test")
		return x.add(node{display: fn.String(), synthetic: true, test: test})
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
	n.name, n.method = fd.Name.Name, fd.Recv != nil
	if n.method {
		n.recv = recvName(fd.Recv.List[0].Type)
	}
	n.file, n.line = file, x.p.Fset.Position(fd.Pos()).Line
	n.test = strings.HasSuffix(file, "_test.go")
	n.testEntry = n.test && fd.Recv == nil && isTestEntry(fd.Name.Name)
	id := x.add(n)
	x.keys[key] = id
	return id
}

func (x *indexer) add(n node) int {
	x.g.nodes = append(x.g.nodes, n)
	x.g.in = append(x.g.in, map[int]edge{})
	x.g.refs = append(x.g.refs, map[int][]Site{})
	return len(x.g.nodes) - 1
}

// recvName returns the type name of a receiver expression.
func recvName(e ast.Expr) string {
	for {
		switch t := e.(type) {
		case *ast.StarExpr:
			e = t.X
		case *ast.ParenExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		case *ast.IndexListExpr:
			e = t.X
		case *ast.Ident:
			return t.Name
		default:
			return ""
		}
	}
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
//
// A caller that reaches a function through a func value (a dispatcher such
// as an event bus) is listed, but the dispatcher's own callers are narrowed
// to those that take that function as a value: everyone else calling the
// dispatcher registers other handlers. With no such caller, the
// registrants themselves are the next level.
func (g *Graph) Impact(file string, startLine, depth int) Impact {
	target, ok := g.byPos[pos{file, startLine}]
	if !ok {
		return Impact{}
	}
	im := Impact{Found: true, Registered: g.registered(target)}

	type item struct{ id, only int } // only >= 0: expand only into registrants of node only
	depthOf := map[int]int{target: 0}
	via := map[int]string{}
	kindOf := map[int]Kind{}
	queued := map[item]bool{{target, -1}: true}
	frontier := []item{{target, -1}}
	for d := 1; len(frontier) > 0; d++ {
		var next []item
		for _, it := range frontier {
			u := it.id
			cs := g.callersOf(u, it.only, false)
			for _, c := range cs {
				kind := Calls
				switch {
				case it.only >= 0:
					kind = Registers
				case c.kind&edgeDirect == 0:
					kind = Dispatch
				}
				cd, seen := depthOf[c.id]
				switch {
				case !seen:
					depthOf[c.id], via[c.id], kindOf[c.id] = d, g.nodes[u].display, kind
				case cd == d && g.nodes[u].display < via[c.id]:
					via[c.id] = g.nodes[u].display
				}
				for _, nx := range c.items(u) {
					it := item{c.id, nx}
					if !queued[it] {
						queued[it] = true
						next = append(next, it)
					}
				}
			}
		}
		frontier = next
	}

	pkgs := map[string]bool{}
	// Test reach also follows implicit edges (a test that converts a type
	// to an interface reaches the methods the standard library calls on
	// it); Callers and TestCallers do not.
	for id := range g.reach(target, true) {
		if n := g.nodes[id]; id != target && n.testEntry {
			im.TestReached = true
			pkgs[n.pkg] = true
		}
	}
	for id, d := range depthOf {
		n := g.nodes[id]
		if id == target {
			continue
		}
		if n.testEntry {
			im.TestReached = true
			pkgs[n.pkg] = true
		}
		if d > depth {
			continue
		}
		switch {
		case n.testEntry:
			im.TestCallers++
		case !n.test:
			im.Callers = append(im.Callers, Caller{Display: n.display, Pkg: n.pkg, Depth: d, Via: via[id], Kind: kindOf[id]})
		}
	}
	for p := range pkgs {
		im.TestPkgs = append(im.TestPkgs, p)
	}
	sort.Strings(im.TestPkgs)
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

// callersOf returns the callers of u to walk on. With only >= 0 (u was
// reached through a call of a func value) they are narrowed to the
// registrants of node only, or those registrants themselves when no caller
// is one. implicit also walks implicit edges (see callers).
func (g *Graph) callersOf(u, only int, implicit bool) []callerEdge {
	cs := g.callers(u, implicit)
	if only < 0 {
		return cs
	}
	regs := g.referencers(only)
	var kept []callerEdge
	for _, c := range cs {
		if regs[c.id] {
			kept = append(kept, c)
		}
	}
	if len(kept) == 0 {
		for _, id := range sortedKeys(regs) {
			kept = append(kept, callerEdge{id: id, kind: edgeDirect})
		}
	}
	return kept
}

type callerEdge struct {
	id   int
	kind edge
}

// items returns how to continue from caller c of callee u: unrestricted
// for a direct call, only into u's registrants for a call through a value.
func (c callerEdge) items(u int) []int {
	var out []int
	if c.kind&edgeDirect != 0 {
		out = append(out, -1)
	}
	if c.kind&edgeDynamic != 0 {
		out = append(out, u)
	}
	return out
}

// callers returns the non-synthetic direct callers of u, looking through
// synthetic functions; a path through a dynamic call is dynamic. With
// implicit, a function reaching u through an implicit edge (it converts
// u's type to an interface) counts as a direct caller.
func (g *Graph) callers(u int, implicit bool) []callerEdge {
	bits := []edge{edgeDirect, edgeDynamic}
	if implicit {
		bits = append(bits, edgeImplicit)
	}
	kinds := map[int]edge{}
	type state struct {
		id  int
		dyn bool
	}
	seen := map[state]bool{}
	var walk func(v int, dyn bool)
	walk = func(v int, dyn bool) {
		for c, ek := range g.in[v] {
			if c == u {
				continue
			}
			for _, bit := range bits {
				if ek&bit == 0 {
					continue
				}
				d := dyn || bit == edgeDynamic
				if seen[state{c, d}] {
					continue
				}
				seen[state{c, d}] = true
				switch {
				case g.nodes[c].synthetic:
					walk(c, d)
				case d:
					kinds[c] |= edgeDynamic
				default:
					kinds[c] |= edgeDirect
				}
			}
		}
	}
	walk(u, false)
	out := make([]callerEdge, 0, len(kinds))
	for _, id := range sortedKeys(kinds) {
		out = append(out, callerEdge{id: id, kind: kinds[id]})
	}
	return out
}

// referencers returns the non-synthetic functions that use u as a value,
// directly or through a synthetic wrapper of u (e.g. a method value).
func (g *Graph) referencers(u int) map[int]bool {
	out := map[int]bool{}
	g.eachRef(u, func(from int, _ []Site) {
		if !g.nodes[from].synthetic {
			out[from] = true
		}
	})
	return out
}

// eachRef calls f for every function using u, or a synthetic wrapper
// around u, as a value.
func (g *Graph) eachRef(u int, f func(from int, sites []Site)) {
	seen := map[int]bool{u: true}
	var walk func(v int)
	walk = func(v int) {
		for from, sites := range g.refs[v] {
			f(from, sites)
		}
		for c := range g.in[v] {
			if g.nodes[c].synthetic && !seen[c] {
				seen[c] = true
				walk(c)
			}
		}
	}
	walk(u)
}

// registered lists u's non-test value uses, by file then line.
func (g *Graph) registered(u int) []Site {
	var out []Site
	seen := map[Site]bool{}
	g.eachRef(u, func(from int, sites []Site) {
		if g.nodes[from].test {
			return
		}
		for _, s := range sites {
			if s.File == "" || strings.HasSuffix(s.File, "_test.go") || seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	})
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Call < b.Call
	})
	return out
}

// Dead returns the functions unreachable from the program's entry points
// that seed accepts, plus the functions reachable only through them (Via).
// Entry points are main and init functions, package initializers, and every
// function root accepts (e.g. exported API); reachability follows calls of
// any kind and uses as a value. TestOnly marks those tests still reach.
// main, init and test functions are never reported. Sorted by File, Line.
func (g *Graph) Dead(root, seed func(FuncInfo) bool) []Unused {
	out := make([][]int, len(g.nodes))
	for callee := range g.nodes {
		for c := range g.in[callee] {
			out[c] = append(out[c], callee)
		}
		for from := range g.refs[callee] {
			out[from] = append(out[from], callee)
		}
	}
	reach := func(starts []int) []bool {
		seen := make([]bool, len(g.nodes))
		queue := starts
		for _, s := range starts {
			seen[s] = true
		}
		for len(queue) > 0 {
			u := queue[0]
			queue = queue[1:]
			for _, v := range out[u] {
				if !seen[v] {
					seen[v] = true
					queue = append(queue, v)
				}
			}
		}
		return seen
	}
	var prodRoots, testRoots []int
	for id, n := range g.nodes {
		switch {
		case n.test:
			testRoots = append(testRoots, id)
		case n.synthetic:
			if strings.HasSuffix(n.display, ".init") {
				prodRoots = append(prodRoots, id)
			}
		case g.entry(n) || root(g.info(n)):
			prodRoots = append(prodRoots, id)
		}
	}
	prod, test := reach(prodRoots), reach(testRoots)

	via := map[int]string{}
	var queue []int
	for id, n := range g.nodes {
		if !prod[id] && !n.synthetic && !n.test && !g.entry(n) && seed(g.info(n)) {
			via[id] = ""
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range out[u] {
			if _, ok := via[v]; ok || prod[v] || g.nodes[v].test {
				continue
			}
			from := g.nodes[u].display
			if via[u] != "" {
				from = via[u]
			}
			via[v] = from // synthetic wrappers are walked through, never reported
			queue = append(queue, v)
		}
	}

	var res []Unused
	for id, v := range via {
		n := g.nodes[id]
		if n.synthetic || g.entry(n) {
			continue
		}
		u := Unused{Display: n.display, Pkg: n.pkg, File: n.file, Line: n.line, TestOnly: test[id], Via: v}
		for c := range g.in[id] {
			if cn := g.nodes[c]; !cn.synthetic && !cn.test && c != id {
				u.DeadCallers = append(u.DeadCallers, cn.display)
			}
		}
		sort.Strings(u.DeadCallers)
		res = append(res, u)
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].File != res[j].File {
			return res[i].File < res[j].File
		}
		return res[i].Line < res[j].Line
	})
	return res
}

// entry reports main and init functions.
func (g *Graph) entry(n node) bool {
	if n.method {
		return false
	}
	pkgName, _, _ := strings.Cut(n.display, ".")
	return n.name == "init" || (n.name == "main" && pkgName == "main")
}

func (g *Graph) info(n node) FuncInfo {
	pkgName, _, _ := strings.Cut(n.display, ".")
	return FuncInfo{Pkg: n.pkg, PkgName: pkgName, Name: n.name, File: n.file, Recv: n.recv, Method: n.method}
}

func sortedKeys[V any](m map[int]V) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// reach returns target and every function that reaches it through callers
// (no depth limit, narrowing calls through func values as Impact does);
// implicit also follows implicit edges.
func (g *Graph) reach(target int, implicit bool) map[int]bool {
	type item struct{ id, only int }
	queued := map[item]bool{{target, -1}: true}
	reached := map[int]bool{target: true}
	frontier := []item{{target, -1}}
	for len(frontier) > 0 {
		var next []item
		for _, it := range frontier {
			for _, c := range g.callersOf(it.id, it.only, implicit) {
				reached[c.id] = true
				for _, nx := range c.items(it.id) {
					if n := (item{c.id, nx}); !queued[n] {
						queued[n] = true
						next = append(next, n)
					}
				}
			}
		}
		frontier = next
	}
	return reached
}

// Entry is a way into the program that reaches a function: Kind is "main",
// "init", "api" or "registered"; Display the function, or for a registered
// one the registration call.
type Entry struct{ Kind, Display string }

const maxEntries = 20

// EntryPoints walks all callers of the function declared at file:startLine
// (no depth limit, narrowing calls through func values as Impact does, and
// following implicit edges) and
// returns the main and init functions, the functions api accepts, and the
// registration sites of every function reached, the target included. Tests
// never count. Sorted by kind (registered, api, main, init: the most
// specific first) then Display, deduplicated, at most 20; nil when none or
// the function is unknown.
func (g *Graph) EntryPoints(file string, startLine int, api func(FuncInfo) bool) []Entry {
	target, ok := g.byPos[pos{file, startLine}]
	if !ok {
		return nil
	}
	reached := g.reach(target, true)

	seen := map[Entry]bool{}
	var out []Entry
	add := func(e Entry) {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	for _, id := range sortedKeys(reached) {
		n := g.nodes[id]
		if n.test || n.synthetic {
			continue
		}
		switch {
		case g.entry(n) && n.name == "init":
			add(Entry{"init", n.display})
		case g.entry(n):
			add(Entry{"main", n.display})
		}
		if api != nil && api(g.info(n)) {
			add(Entry{"api", n.display})
		}
		for _, s := range g.registered(id) {
			add(Entry{"registered", s.Text()})
		}
	}
	rank := map[string]int{"registered": 0, "api": 1, "main": 2, "init": 3}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Kind != b.Kind {
			return rank[a.Kind] < rank[b.Kind]
		}
		return a.Display < b.Display
	})
	if len(out) > maxEntries {
		out = out[:maxEntries]
	}
	return out
}
