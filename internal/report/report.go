// Package report renders the MR note: a pure Model-to-Markdown function.
package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gitlab.com/wh1t3hat/mrimpact/internal/callgraph"
	"gitlab.com/wh1t3hat/mrimpact/internal/risk"
)

// Marker identifies the note so it can be found and updated.
const Marker = "<!-- mrimpact:v1 -->"

const (
	maxTableRows   = 50
	maxDiagramNode = 20
	maxCallerList  = 10
)

// FuncRow is one changed function.
type FuncRow struct {
	Level     risk.Level
	Display   string
	Kind      string // "new" | "body" | "signature" | "signature (exported)"
	Callers   []callgraph.Caller
	CoverText string // "0 % (0/12)", "n/a", "reached by test", "not reached"
}

// Model is everything the note shows.
type Model struct {
	Funcs        []FuncRow // unsorted; Render sorts by Level desc, len(Callers) desc, Display
	Deleted      []string
	Decls        []string
	TestsTouched []string
	NotAnalyzed  []string
	Warnings     []string
	Coverage     string // "coverprofile" | "static"
	BaseSHA      string // rendered as first 7 chars
	Degraded     bool
	Failure      string // non-empty: only marker + failure line + footer
	NoGoChanges  bool   // marker + "No Go code changes" + footer
	Version      string
	Duration     time.Duration
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

	// With no rated functions there is nothing for the table, diagram or
	// caller list to show; only the context lines and limits remain.
	if len(funcs) > 0 {
		writeTable(&b, funcs)
	}

	for _, l := range []struct {
		prefix string
		items  []string
	}{
		{"➖ Deleted: ", m.Deleted},
		{"📝 Declarations changed: ", m.Decls},
		{"🧪 Tests touched in this MR: ", m.TestsTouched},
	} {
		if len(l.items) > 0 {
			b.WriteString(l.prefix + codeList(l.items) + "\n")
		}
	}
	if len(m.Deleted)+len(m.Decls)+len(m.TestsTouched) > 0 {
		b.WriteString("\n")
	}

	if len(funcs) > 0 {
		writeDiagram(&b, funcs)
		writeCallers(&b, funcs)
	}

	b.WriteString("<details><summary>⚠️ Limits of this analysis</summary>\n\n")
	b.WriteString("- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.\n")
	b.WriteString("- This repo only: callers in other services (HTTP/gRPC) are not visible.\n")
	b.WriteString("- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers.\n")
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
	b.WriteString("| Risk | Function | Change | Callers | Added-line coverage |\n|---|---|---|---|---|\n")
	for i, f := range funcs {
		if i == maxTableRows {
			break
		}
		fmt.Fprintf(b, "| %s | `%s` | %s | %s | %s |\n", f.Level.Emoji(), cell(f.Display), cell(f.Kind), callerCell(f.Callers), cell(f.CoverText))
	}
	if len(funcs) > maxTableRows {
		fmt.Fprintf(b, "\n_… and %d more_\n", len(funcs)-maxTableRows)
	}
	b.WriteString("\n")
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

// dedupeCallers returns callers sorted by (Depth, Display), one per Display
// (the nearest occurrence wins).
func dedupeCallers(cs []callgraph.Caller) []callgraph.Caller {
	s := append([]callgraph.Caller(nil), cs...)
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].Depth != s[j].Depth {
			return s[i].Depth < s[j].Depth
		}
		return s[i].Display < s[j].Display
	})
	seen := map[string]bool{}
	out := s[:0]
	for _, c := range s {
		if !seen[c.Display] {
			seen[c.Display] = true
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

func writeDiagram(b *strings.Builder, funcs []FuncRow) {
	type node struct {
		id    string
		label string
		class string
	}
	var order []string
	nodes := map[string]*node{}
	all := map[string]bool{} // every distinct Display, for the not-shown count
	add := func(display, emoji, class string) {
		if nodes[display] != nil {
			return
		}
		n := &node{id: fmt.Sprintf("n%d", len(order)), class: class, label: quote(display)}
		if emoji != "" {
			n.label = quote(emoji + " " + display)
		}
		nodes[display] = n
		order = append(order, display)
	}
	for _, f := range funcs {
		all[f.Display] = true
		for _, c := range f.Callers {
			all[c.Display] = true
		}
	}
	for _, f := range funcs {
		if len(order) >= maxDiagramNode {
			break
		}
		add(f.Display, f.Level.Emoji(), className(f.Level))
		for _, c := range f.Callers {
			if len(order) >= maxDiagramNode {
				break
			}
			add(c.Display, "", "")
		}
	}
	// A changed function listed later may already exist as a plain caller node.
	for _, f := range funcs {
		if n := nodes[f.Display]; n != nil && n.class == "" {
			n.label, n.class = quote(f.Level.Emoji()+" "+f.Display), className(f.Level)
		}
	}

	hidden := len(all) - len(order)
	b.WriteString("```mermaid\ngraph RL\n")
	usedClass := map[string]bool{}
	for _, d := range order {
		n := nodes[d]
		if n.class != "" {
			usedClass[n.class] = true
			fmt.Fprintf(b, "  %s[%s]:::%s\n", n.id, n.label, n.class)
		} else {
			fmt.Fprintf(b, "  %s[%s]\n", n.id, n.label)
		}
	}
	seenEdge := map[string]bool{}
	for _, f := range funcs {
		for _, c := range f.Callers {
			via := c.Via
			if via == "" {
				via = f.Display
			}
			from, to := nodes[c.Display], nodes[via]
			if from == nil || to == nil || from == to {
				continue
			}
			e := from.id + " --> " + to.id
			if !seenEdge[e] {
				seenEdge[e] = true
				fmt.Fprintf(b, "  %s\n", e)
			}
		}
	}
	for _, cl := range []struct{ name, style string }{
		{"red", "fill:#fdd,stroke:#c00"},
		{"yellow", "fill:#ffd,stroke:#c90"},
		{"green", "fill:#dfd,stroke:#2a2"},
	} {
		if usedClass[cl.name] {
			fmt.Fprintf(b, "  classDef %s %s\n", cl.name, cl.style)
		}
	}
	if hidden > 0 {
		fmt.Fprintf(b, "  %%%% +%d not shown\n", hidden)
	}
	b.WriteString("```\n\n")
	if hidden > 0 {
		fmt.Fprintf(b, "+%d nodes not shown\n\n", hidden)
	}
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
		if len(f.Callers) == 0 {
			continue
		}
		fmt.Fprintf(b, "**`%s`** (%d)\n\n", f.Display, len(f.Callers))
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
