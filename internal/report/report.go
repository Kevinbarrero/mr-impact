// Package report renders the MR note: a pure Model-to-Markdown function.
package report

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"gitlab.com/wh1t3hat/mrimpact/internal/callgraph"
	"gitlab.com/wh1t3hat/mrimpact/internal/configfields"
	"gitlab.com/wh1t3hat/mrimpact/internal/registrations"
	"gitlab.com/wh1t3hat/mrimpact/internal/risk"
)

// Marker identifies the note so it can be found and updated.
const Marker = "<!-- mrimpact:v1 -->"

const (
	maxTableRows   = 50
	maxDiagramNode = 20
	maxCallerList  = 10
	maxUnusedList  = 15
	maxInlineList  = 5 // longer lists are counted and collapsed
)

// FuncRow is one changed function.
type FuncRow struct {
	Level      risk.Level
	Display    string
	Kind       string // "new" | "body" | "signature" | "signature (exported)"
	Callers    []callgraph.Caller
	CoverText  string           // "0 % (0/12)", "no added lines · reached by test", "reached by test", "not reached", "n/a"
	Registered []callgraph.Site // where the function is passed as a value (handler/callback registration)
}

// RemovedCase is a case value removed from a switch while the value still
// exists.
type RemovedCase struct {
	Func, File string
	Line       int // HEAD line of the switch
	Case       string
	HasDefault bool
}

// DeletedFunc is a function removed by the MR.
type DeletedFunc struct {
	Display string
	File    string // BASE path
}

// Decl is a changed top-level declaration outside any function.
type Decl struct {
	What string // "type Foo", "var x", "const y"
	File string
}

// Model is everything the note shows.
type Model struct {
	Funcs        []FuncRow          // unsorted; Render sorts by Level desc, len(Callers) desc, Display
	RemovedCases []RemovedCase      // behaviour changes: first in the note
	Unused       []callgraph.Unused // functions this MR left without callers
	// UnusedGenerated: generated functions (sqlc, protobuf, mocks) nothing
	// uses any more; to remove from their source and regenerate.
	UnusedGenerated []callgraph.Unused
	// UnusedConfig: configuration fields (often env vars) nothing reads any
	// more: deployments still set them for nothing.
	UnusedConfig []configfields.Field
	// Registrations removed with no replacement (base positions), added
	// (head positions) and rewired to another handler: routes,
	// subscriptions, hooks the MR unwires, wires or swaps.
	RegRemoved, RegAdded []registrations.Registration
	RegRewired           []registrations.Rewired
	Deleted              []DeletedFunc
	DeletedFiles         []string
	Decls                []Decl
	TestsTouched         []string
	NotAnalyzed          []string
	Warnings             []string
	Coverage             string // "coverprofile" | "static"
	BaseSHA              string // rendered as first 7 chars
	Degraded             bool
	Failure              string // non-empty: only marker + failure line + footer
	NoGoChanges          bool   // marker + "No Go code changes" + footer
	Version              string
	Duration             time.Duration
}

// Render produces the Markdown note.
func Render(m Model) string {
	var b strings.Builder
	b.WriteString(Marker + "\n")
	switch {
	case m.Failure != "":
		b.WriteString("couldn't analyze: " + m.Failure + "\n\n")
		writeFooter(&b, m)
		return b.String()
	case m.NoGoChanges:
		b.WriteString("No Go code changes\n\n")
		writeFooter(&b, m)
		return b.String()
	}

	funcs := sortedFuncs(m.Funcs)
	var red, yellow, green int
	for _, f := range funcs {
		switch f.Level {
		case risk.Red:
			red++
		case risk.Yellow:
			yellow++
		default:
			green++
		}
	}

	fmt.Fprintf(&b, "### 🔍 MR impact — %d functions changed\n", len(funcs))
	fmt.Fprintf(&b, "**🔴 %d · 🟡 %d · 🟢 %d** · coverage: `%s` · base `%s`", red, yellow, green, m.Coverage, shortSHA(m.BaseSHA))
	if m.Degraded {
		b.WriteString(" · ⚠️ degraded")
	}
	b.WriteString("\n\n")

	writeRemovedCases(&b, m.RemovedCases)
	writeUnused(&b, m.Unused)
	writeConfig(&b, m.UnusedConfig)
	writeUnusedList(&b, "🧩 **Generated code no longer used** — remove it from its source (.sql, .proto, interface) and regenerate", m.UnusedGenerated)
	writeRegistrations(&b, "🔌 **Registrations removed, no replacement** — these calls no longer get a handler; check each is intended", "base", m.RegRemoved)
	writeRewired(&b, m.RegRewired)
	writeRegistrations(&b, "🔌 **Registrations added**", "head", m.RegAdded)

	// With no rated functions there is nothing for the table, diagram or
	// caller list to show; only the context lines and limits remain.
	if len(funcs) > 0 {
		writeTable(&b, funcs)
	}

	writeDeleted(&b, m.Deleted, m.DeletedFiles)
	writeDecls(&b, m.Decls)
	switch n := len(m.TestsTouched); {
	case n == 0:
	case n <= maxInlineList:
		b.WriteString("🧪 Tests touched in this MR: " + codeList(m.TestsTouched) + "\n\n")
	default:
		fmt.Fprintf(&b, "🧪 Tests touched in this MR: %s\n\n", count(n, "file"))
		writeGrouped(&b, fmt.Sprintf("Test files (%d)", n), m.TestsTouched, func(s string) (string, string) { return path.Dir(s), path.Base(s) })
	}

	if len(funcs) > 0 {
		writeDiagram(&b, funcs)
		writeCallers(&b, funcs)
	}

	b.WriteString("<details><summary>⚠️ Limits of this analysis</summary>\n\n")
	b.WriteString("- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.\n")
	b.WriteString("- This repo only: callers in other services (HTTP/gRPC) are not visible.\n")
	b.WriteString("- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers; where such a function is registered is listed under Registered.\n")
	b.WriteString("- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.\n")
	for _, s := range m.NotAnalyzed {
		b.WriteString("- Not analyzed: " + s + "\n")
	}
	for _, s := range m.Warnings {
		b.WriteString("- " + s + "\n")
	}
	b.WriteString("</details>\n\n")

	writeFooter(&b, m)
	return b.String()
}

func writeTable(b *strings.Builder, funcs []FuncRow) {
	withReg := false
	for i, f := range funcs {
		if i < maxTableRows && len(f.Registered) > 0 {
			withReg = true
		}
	}
	if withReg {
		b.WriteString("| Risk | Function | Change | Callers | Registered | Added-line coverage |\n|---|---|---|---|---|---|\n")
	} else {
		b.WriteString("| Risk | Function | Change | Callers | Added-line coverage |\n|---|---|---|---|---|\n")
	}
	for i, f := range funcs {
		if i == maxTableRows {
			break
		}
		fmt.Fprintf(b, "| %s | `%s` | %s | %s | ", f.Level.Emoji(), cell(f.Display), cell(f.Kind), callerCell(f.Callers))
		if withReg {
			b.WriteString(registeredCell(f.Registered) + " | ")
		}
		fmt.Fprintf(b, "%s |\n", cell(f.CoverText))
	}
	if len(funcs) > maxTableRows {
		fmt.Fprintf(b, "\n_… and %d more_\n", len(funcs)-maxTableRows)
	}
	b.WriteString("\n")
}

// registeredCell shows the first registration site and how many more.
func registeredCell(rs []callgraph.Site) string {
	if len(rs) == 0 {
		return ""
	}
	out := "`" + cell(siteText(rs[0])) + "`"
	if len(rs) > 1 {
		out += fmt.Sprintf(" +%d", len(rs)-1)
	}
	return out
}

func siteText(s callgraph.Site) string {
	if s.Call == "" {
		return fmt.Sprintf("%s:%d", s.File, s.Line)
	}
	return strings.ReplaceAll(s.Call, "`", "'")
}

// writeUnused lists the functions the MR left without callers: dead code,
// or code that only tests still exercise.
func writeUnused(b *strings.Builder, us []callgraph.Unused) {
	var dead, tests []callgraph.Unused
	for _, u := range us {
		if u.TestOnly {
			tests = append(tests, u)
		} else {
			dead = append(dead, u)
		}
	}
	writeUnusedList(b, "🪦 **Now unused** — nothing reachable calls these after this MR", dead)
	writeUnusedList(b, "🧫 **Now only used by tests** — no production path reaches these after this MR", tests)
}

func writeConfig(b *strings.Builder, fs []configfields.Field) {
	if len(fs) == 0 {
		return
	}
	fmt.Fprintf(b, "⚙️ **Config no longer read** — remove the field and the setting from deployments (%d):\n\n", len(fs))
	for i, f := range fs {
		if i == maxUnusedList {
			fmt.Fprintf(b, "- … and %d more\n", len(fs)-maxUnusedList)
			break
		}
		fmt.Fprintf(b, "- `%s` `%s` — `%s:%d`", f.Display, strings.ReplaceAll(f.Tag, "`", "'"), f.File, f.Line)
		if f.TestOnly {
			b.WriteString(" (read only by tests)")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func writeUnusedList(b *strings.Builder, title string, items []callgraph.Unused) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s (%d):\n\n", title, len(items))
	for i, u := range items {
		if i == maxUnusedList {
			fmt.Fprintf(b, "- … and %d more\n", len(items)-maxUnusedList)
			break
		}
		fmt.Fprintf(b, "- `%s` — `%s:%d`", u.Display, u.File, u.Line)
		switch {
		case u.Via != "":
			fmt.Fprintf(b, " (only reached through `%s`)", u.Via)
		case len(u.DeadCallers) > 0:
			fmt.Fprintf(b, " (its remaining callers are unreachable too: %s)", codeList(u.DeadCallers))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func writeRegistrations(b *strings.Builder, title, side string, rs []registrations.Registration) {
	if len(rs) == 0 {
		return
	}
	line := func(r registrations.Registration) string {
		return fmt.Sprintf("- `%s` → `%s` — `%s:%d`\n", strings.ReplaceAll(r.Call, "`", "'"), r.Handler, r.File, r.Line)
	}
	fmt.Fprintf(b, "%s (%d, positions in %s):\n\n", title, len(rs), side)
	if len(rs) <= maxUnusedList {
		for _, r := range rs {
			b.WriteString(line(r))
		}
		b.WriteString("\n")
		return
	}
	byCallee := map[string][]registrations.Registration{}
	var callees []string
	for _, r := range rs {
		if _, ok := byCallee[r.Callee]; !ok {
			callees = append(callees, r.Callee)
		}
		byCallee[r.Callee] = append(byCallee[r.Callee], r)
	}
	sort.Slice(callees, func(i, j int) bool {
		a, c := len(byCallee[callees[i]]), len(byCallee[callees[j]])
		if a != c {
			return a > c
		}
		return callees[i] < callees[j]
	})
	var parts []string
	for _, c := range callees {
		parts = append(parts, fmt.Sprintf("`%s` %d", c, len(byCallee[c])))
	}
	b.WriteString("By call: " + strings.Join(parts, " · ") + "\n\n")
	fmt.Fprintf(b, "<details><summary>All %d</summary>\n\n", len(rs))
	for _, c := range callees {
		fmt.Fprintf(b, "**`%s`** (%d)\n\n", c, len(byCallee[c]))
		for _, r := range byCallee[c] {
			b.WriteString(line(r))
		}
		b.WriteString("\n")
	}
	b.WriteString("</details>\n\n")
}

func writeRemovedCases(b *strings.Builder, cs []RemovedCase) {
	if len(cs) == 0 {
		return
	}
	fmt.Fprintf(b, "↪️ **Removed `case`, value still exists** — these values now take another path (%d):\n\n", len(cs))
	for i, c := range cs {
		if i == maxUnusedList {
			fmt.Fprintf(b, "- … and %d more\n", len(cs)-maxUnusedList)
			break
		}
		path := "now matches no case"
		if c.HasDefault {
			path = "now goes to `default`"
		}
		fmt.Fprintf(b, "- `case %s` in `%s` — %s — `%s:%d`\n", strings.ReplaceAll(c.Case, "`", "'"), c.Func, path, c.File, c.Line)
	}
	b.WriteString("\n")
}

func writeRewired(b *strings.Builder, rs []registrations.Rewired) {
	if len(rs) == 0 {
		return
	}
	fmt.Fprintf(b, "🔁 **Registrations rewired** — same call, another handler (%d):\n\n", len(rs))
	for i, r := range rs {
		if i == maxUnusedList {
			fmt.Fprintf(b, "- … and %d more\n", len(rs)-maxUnusedList)
			break
		}
		fmt.Fprintf(b, "- `%s`: `%s` → `%s` — `%s:%d`\n", strings.ReplaceAll(r.To.Call, "`", "'"), r.From.Handler, r.To.Handler, r.To.File, r.To.Line)
	}
	b.WriteString("\n")
}

func writeDeleted(b *strings.Builder, ds []DeletedFunc, files []string) {
	if len(ds) == 0 && len(files) == 0 {
		return
	}
	b.WriteString("➖ Deleted: ")
	var parts []string
	switch {
	case len(ds) == 0:
	case len(ds) <= maxInlineList:
		names := make([]string, len(ds))
		for i, d := range ds {
			names[i] = d.Display
		}
		parts = append(parts, codeList(names))
	default:
		dirs := map[string]bool{}
		for _, d := range ds {
			dirs[path.Dir(d.File)] = true
		}
		p := count(len(ds), "function")
		if len(dirs) > 1 {
			p += fmt.Sprintf(" in %d packages", len(dirs))
		}
		parts = append(parts, p)
	}
	if len(files) > 0 {
		parts = append(parts, count(len(files), "file")+" removed")
	}
	b.WriteString(strings.Join(parts, " · ") + "\n\n")
	if len(ds) <= maxInlineList {
		return
	}
	items := make([]string, len(ds))
	for i, d := range ds {
		items[i] = d.Display + "\x00" + d.File
	}
	writeGrouped(b, fmt.Sprintf("Deleted functions (%d)", len(ds)), items, func(s string) (string, string) {
		display, f, _ := strings.Cut(s, "\x00")
		return path.Dir(f), unqualified(display)
	})
}

func writeDecls(b *strings.Builder, ds []Decl) {
	if len(ds) == 0 {
		return
	}
	files := map[string]bool{}
	items := make([]string, len(ds))
	for i, d := range ds {
		files[d.File] = true
		items[i] = d.What + "\x00" + d.File
	}
	if len(ds) <= maxInlineList {
		names := make([]string, len(ds))
		for i, d := range ds {
			names[i] = d.What + " in " + d.File
		}
		b.WriteString("📝 Declarations changed: " + codeList(names) + "\n\n")
		return
	}
	fmt.Fprintf(b, "📝 Declarations changed: %d in %s\n\n", len(ds), count(len(files), "file"))
	writeGrouped(b, fmt.Sprintf("Declarations (%d)", len(ds)), items, func(s string) (string, string) {
		what, f, _ := strings.Cut(s, "\x00")
		return f, what
	})
}

// writeGrouped writes items as a collapsed list, one line per group (in
// sorted order), each group's names in input order.
func writeGrouped(b *strings.Builder, summary string, items []string, split func(string) (group, name string)) {
	var order []string
	byGroup := map[string][]string{}
	for _, it := range items {
		g, n := split(it)
		if _, ok := byGroup[g]; !ok {
			order = append(order, g)
		}
		byGroup[g] = append(byGroup[g], n)
	}
	sort.Strings(order)
	fmt.Fprintf(b, "<details><summary>%s</summary>\n\n", summary)
	for _, g := range order {
		names := byGroup[g]
		fmt.Fprintf(b, "- `%s` (%d): %s\n", g, len(names), codeList(names))
	}
	b.WriteString("\n</details>\n\n")
}

// unqualified drops the package-clause prefix of a Display name; the group
// already names the package.
func unqualified(display string) string {
	if _, rest, ok := strings.Cut(display, "."); ok {
		return rest
	}
	return display
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func writeFooter(b *strings.Builder, m Model) {
	fmt.Fprintf(b, "<sub>Rules: %s · mrimpact v%s · %.1f s</sub>\n", risk.Legend, m.Version, m.Duration.Seconds())
}

func sortedFuncs(in []FuncRow) []FuncRow {
	out := append([]FuncRow(nil), in...)
	for i := range out {
		out[i].Callers = dedupeCallers(out[i].Callers)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Level != b.Level {
			return a.Level > b.Level
		}
		if len(a.Callers) != len(b.Callers) {
			return len(a.Callers) > len(b.Callers)
		}
		return a.Display < b.Display
	})
	return out
}

// dedupeCallers returns callers sorted by (Depth, Display, Pkg), one per
// (Display, Pkg) (the nearest occurrence wins): same-named functions in
// different packages, e.g. main.run in two commands, are distinct callers.
func dedupeCallers(cs []callgraph.Caller) []callgraph.Caller {
	s := append([]callgraph.Caller(nil), cs...)
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].Depth != s[j].Depth {
			return s[i].Depth < s[j].Depth
		}
		if s[i].Display != s[j].Display {
			return s[i].Display < s[j].Display
		}
		return s[i].Pkg < s[j].Pkg
	})
	seen := map[string]bool{}
	out := s[:0]
	for _, c := range s {
		if k := c.Display + "\x00" + c.Pkg; !seen[k] {
			seen[k] = true
			out = append(out, c)
		}
	}
	return out
}

func callerCell(cs []callgraph.Caller) string {
	if len(cs) == 0 {
		return "0"
	}
	pkgs := map[string]bool{}
	for _, c := range cs {
		pkgs[c.Pkg] = true
	}
	if len(pkgs) > 1 {
		return fmt.Sprintf("%d (%d pkgs)", len(cs), len(pkgs))
	}
	return fmt.Sprint(len(cs))
}

const maxDiagramCallers = 3 // per function; the rest become one "+N more callers" node

// writeDiagram draws the changed functions (riskiest first) and, compactly,
// who reaches them: up to maxDiagramCallers each, the code that registered
// a callback first (one dashed edge, skipping the generic dispatcher), then
// direct callers; the rest as one "+N" node. Everything is in the caller
// list below.
func writeDiagram(b *strings.Builder, funcs []FuncRow) {
	type node struct {
		id, label, class, shape string
	}
	var order []string
	nodes := map[string]*node{}
	add := func(key, label, class, shape string) *node {
		if n := nodes[key]; n != nil {
			return n
		}
		n := &node{id: fmt.Sprintf("n%d", len(order)), label: quote(label), class: class, shape: shape}
		nodes[key] = n
		order = append(order, key)
		return n
	}
	changed := map[string]bool{}
	for _, f := range funcs {
		changed[f.Display] = true
	}
	var edges []string
	seenEdge := map[string]bool{}
	edge := func(from, to *node, style string) {
		e := from.id + " " + style + " " + to.id
		if from != to && !seenEdge[e] {
			seenEdge[e] = true
			edges = append(edges, e)
		}
	}

	shownFuncs := 0
	for _, f := range funcs {
		if len(order) >= maxDiagramNode {
			break
		}
		shownFuncs++
		fn := add(f.Display, f.Level.Emoji()+" "+f.Display, className(f.Level), "")
		fn.label, fn.class = quote(f.Level.Emoji()+" "+f.Display), className(f.Level) // it may exist as a caller

		// Candidates: registrants, then direct callers and dispatchers
		// nobody registered through.
		registrantVia := map[string]bool{}
		var picks []callgraph.Caller
		for _, c := range f.Callers {
			if c.Kind == callgraph.Registers {
				picks = append(picks, c)
				registrantVia[c.Via] = true
			}
		}
		for _, c := range f.Callers {
			if c.Depth == 1 && c.Kind != callgraph.Registers && !(c.Kind == callgraph.Dispatch && registrantVia[c.Display]) {
				picks = append(picks, c)
			}
		}
		shown := 0
		for _, c := range picks {
			if shown == maxDiagramCallers || len(order) >= maxDiagramNode {
				break
			}
			class := ""
			label := c.Display
			if changed[c.Display] {
				continue // drawn as a changed function with its own risk
			}
			cn := add(c.Display, label, class, "")
			switch c.Kind {
			case callgraph.Registers:
				edge(cn, fn, `-. "registers via `+shortName(c.Via)+`" .->`)
			case callgraph.Dispatch:
				edge(cn, fn, `-. "calls the callback" .->`)
			default:
				edge(cn, fn, "-->")
			}
			shown++
		}
		// "+N": direct callers not drawn; not the dispatchers a
		// registrant's edge stands for, nor changed callers (drawn as
		// themselves, linked below). Callers' callers are in the list.
		rest := 0
		for _, c := range f.Callers {
			if c.Depth == 1 && !changed[c.Display] && !(c.Kind == callgraph.Dispatch && registrantVia[c.Display]) {
				rest++
			}
		}
		for _, c := range picks {
			if c.Depth == 1 && nodes[c.Display] != nil && !changed[c.Display] {
				rest--
			}
		}
		if rest > 0 && len(order) < maxDiagramNode {
			more := add("+more "+f.Display, "+"+count(rest, "more caller"), "", "round")
			edge(more, fn, "-.->")
		}
	}
	// Calls and registrations between changed functions, once every drawn
	// one has a node.
	for _, f := range funcs {
		for _, c := range f.Callers {
			from, to := nodes[c.Display], nodes[f.Display]
			if !changed[c.Display] || from == nil || to == nil {
				continue
			}
			switch {
			case c.Kind == callgraph.Registers:
				edge(from, to, `-. "registers via `+shortName(c.Via)+`" .->`)
			case c.Depth == 1 && c.Kind == callgraph.Dispatch:
				edge(from, to, `-. "calls the callback" .->`)
			case c.Depth == 1:
				edge(from, to, "-->")
			}
		}
	}

	b.WriteString("**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested or a branch removed · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.\n\n")
	b.WriteString("```mermaid\nflowchart RL\n")
	usedClass := map[string]bool{}
	for _, k := range order {
		n := nodes[k]
		open, closing := "[", "]"
		if n.shape == "round" {
			open, closing = "([", "])"
		}
		if n.class != "" {
			usedClass[n.class] = true
			fmt.Fprintf(b, "  %s%s%s%s:::%s\n", n.id, open, n.label, closing, n.class)
		} else {
			fmt.Fprintf(b, "  %s%s%s%s\n", n.id, open, n.label, closing)
		}
	}
	for _, e := range edges {
		fmt.Fprintf(b, "  %s\n", e)
	}
	b.WriteString("  subgraph legend [\"Legend\"]\n    direction LR\n")
	for _, l := range []struct{ id, label, class string }{
		{"lr", "🔴 changed, not tested", "red"},
		{"ly", "🟡 partly tested / branch removed", "yellow"},
		{"lg", "🟢 changed, tested", "green"},
		{"lc", "caller, not changed", ""},
	} {
		if l.class != "" {
			usedClass[l.class] = true
			fmt.Fprintf(b, "    %s[%s]:::%s\n", l.id, quote(l.label), l.class)
		} else {
			fmt.Fprintf(b, "    %s[%s]\n", l.id, quote(l.label))
		}
	}
	b.WriteString("  end\n")
	for _, cl := range []struct{ name, style string }{
		{"red", "fill:#fdd,stroke:#c00"},
		{"yellow", "fill:#ffd,stroke:#c90"},
		{"green", "fill:#dfd,stroke:#2a2"},
	} {
		if usedClass[cl.name] {
			fmt.Fprintf(b, "  classDef %s %s\n", cl.name, cl.style)
		}
	}
	b.WriteString("```\n\n")
	if hidden := len(funcs) - shownFuncs; hidden > 0 {
		fmt.Fprintf(b, "%d more changed functions are not drawn; see the table.\n\n", hidden)
	}
}

// shortName drops the package and receiver: "events.(*service).SubscribeAsync" -> "SubscribeAsync".
func shortName(display string) string {
	if i := strings.LastIndex(display, "."); i >= 0 {
		return display[i+1:]
	}
	return display
}

func className(l risk.Level) string {
	switch l {
	case risk.Red:
		return "red"
	case risk.Yellow:
		return "yellow"
	default:
		return "green"
	}
}

// quote wraps a Mermaid label in double quotes, escaping embedded quotes.
func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, "#quot;") + `"`
}

func writeCallers(b *strings.Builder, funcs []FuncRow) {
	total := 0
	for _, f := range funcs {
		total += len(f.Callers)
	}
	fmt.Fprintf(b, "<details><summary>All callers (%d)</summary>\n\n", total)
	for _, f := range funcs {
		if len(f.Callers) == 0 && len(f.Registered) == 0 {
			continue
		}
		fmt.Fprintf(b, "**`%s`** (%d)\n\n", f.Display, len(f.Callers))
		for _, r := range f.Registered {
			fmt.Fprintf(b, "- registered: `%s` — `%s:%d`\n", siteText(r), r.File, r.Line)
		}
		shown := f.Callers
		if len(shown) > maxCallerList {
			shown = shown[:maxCallerList]
		}
		for i := 0; i < len(shown); {
			j := i
			var names []string
			for ; j < len(shown) && shown[j].Depth == shown[i].Depth; j++ {
				names = append(names, "`"+shown[j].Display+"`")
			}
			fmt.Fprintf(b, "- depth %d: %s\n", shown[i].Depth, strings.Join(names, ", "))
			i = j
		}
		if more := len(f.Callers) - len(shown); more > 0 {
			fmt.Fprintf(b, "- and %d more\n", more)
		}
		b.WriteString("\n")
	}
	b.WriteString("</details>\n\n")
}

func codeList(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = "`" + s + "`"
	}
	return strings.Join(q, ", ")
}

func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
