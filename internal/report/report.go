// Package report renders the MR note: a pure Model-to-Markdown function.
package report

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Kevinbarrero/mr-impact/internal/arch"
	"github.com/Kevinbarrero/mr-impact/internal/callgraph"
	"github.com/Kevinbarrero/mr-impact/internal/config"
	"github.com/Kevinbarrero/mr-impact/internal/configfields"
	"github.com/Kevinbarrero/mr-impact/internal/registrations"
	"github.com/Kevinbarrero/mr-impact/internal/risk"
)

// Marker identifies the note so it can be found and updated.
const Marker = "<!-- mrimpact:v1 -->"

const (
	maxTableRows   = 50
	maxDiagramNode = 20
	compactAbove   = 20 // table rows above which the layout is compact
	compactYellow  = 10 // yellow rows the compact layout keeps visible
	lookFirstN     = 3  // rows named by the "Look at first" line
	maxCallerList  = 10
	maxUnusedList  = 15
	maxInlineList  = 5 // longer lists are counted and collapsed
	maxArchList    = 15

	maxUncoveredFuncs  = 15
	maxUncoveredRanges = 5
)

// FuncRow is one changed function.
type FuncRow struct {
	Level      risk.Level
	Reason     string // the rule that set Level ("untested, 3 callers"); empty for 🟢
	Display    string
	Kind       string // "new" | "body" | "signature" | "signature (exported)"
	Callers    []callgraph.Caller
	CoverText  string            // "0 % (0/12)", "no added lines · reached by test", "reached by test", "not reached", "n/a"
	Registered []callgraph.Site  // where the function is passed as a value (handler/callback registration)
	Entries    []callgraph.Entry // entry points reaching it (main/init/api/registration); nil in a degraded run
	File       string            // repo-relative; shown only in a group's members list
	// Uncovered: the added lines the coverprofile marks as statements with
	// count 0, ascending; filled only in coverprofile mode.
	Uncovered []int
	// Members: the new functions folded into this row (only new code calls
	// them), sorted by Display; non-nil only on a group row.
	Members []FuncRow
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

// SensitiveRow is one changed function that touches a sensitive API or lies
// in a critical package.
type SensitiveRow struct {
	Level     risk.Level
	Display   string
	APIs      []string // matched APIs, first-occurrence order
	Critical  string   // the critical glob it matched; "" if none
	CoverText string
	Untested  bool // the untested-sensitive rule rated it red; listed first, not shown
}

// ConfigNote is the effective config file as the note states it: shown when
// it removes defaults or names critical globs, or when the MR changed it.
type ConfigNote struct {
	File                    string   // as named: ".mrimpact.yml" or the --config path
	Critical, Removes, Adds []string // head (effective) values
	Architecture            string   // preset or "custom"; "" when the block is unset
	Changed                 bool     // head differs from base
	BaseUnreadable          bool     // the base file did not parse
	Changes                 []config.Change
}

// Model is everything the note shows.
type Model struct {
	Funcs        []FuncRow          // unsorted; Render sorts by Level desc, len(Callers) desc, Display
	Sensitive    []SensitiveRow     // unsorted; Render lists untested first, then by Display
	Config       *ConfigNote        // nil: nothing to say about the config file
	Arch         *arch.Result       // nil: no architecture analysis (e.g. an early failure)
	RemovedCases []RemovedCase      // behaviour changes: first in the note
	Unused       []callgraph.Unused // functions this MR left without callers
	// UnusedGenerated: generated functions (sqlc, protobuf, mocks) nothing
	// uses any more; to remove from their source and regenerate.
	UnusedGenerated []callgraph.Unused
	// UnusedConfig: configuration fields (often env vars) nothing reads any
	// more: deployments still set them for nothing.
	UnusedConfig []configfields.Field
	// Registrations removed with no replacement (base positions), added
	// (head positions), rewired to another handler, and changed (same call
	// and handler, other attributes such as middleware): routes,
	// subscriptions, hooks the MR unwires, wires, swaps or rewraps.
	RegRemoved, RegAdded []registrations.Registration
	RegRewired           []registrations.Rewired
	RegChanged           []registrations.Changed // same call and handler, other attributes (head positions)
	Deleted              []DeletedFunc
	DeletedFiles         []string
	Decls                []Decl
	TestsTouched         []string
	NotAnalyzed          []string
	Warnings             []string
	Coverage             string // "coverprofile" | "static"
	BaseSHA              string // rendered as first 7 chars
	Degraded             bool
	CrossPkgOnly         int    // functions covered only by tests in other packages: renders the coverpkg hint
	Folded               int    // new functions folded into group rows (FuncRow.Members); counted in the header
	Groups               int    // group rows that folded any
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

	fmt.Fprintf(&b, "### 🔍 MR impact — %d functions changed\n", len(funcs)+m.Folded)
	fmt.Fprintf(&b, "**🔴 %d · 🟡 %d · 🟢 %d** · coverage: `%s` · base `%s`", red, yellow, green, m.Coverage, shortSHA(m.BaseSHA))
	if m.Degraded {
		b.WriteString(" · ⚠️ degraded")
	}
	if m.Folded > 0 {
		// A trailing backslash is a hard line break: the fold line renders
		// directly under the tally, in the same paragraph.
		groups := "groups"
		if m.Groups == 1 {
			groups = "group"
		}
		fmt.Fprintf(&b, "\\\n+%d new functions folded into %d %s", m.Folded, m.Groups, groups)
	}
	b.WriteString("\n\n")
	writeLookFirst(&b, funcs)
	writeConfigNote(&b, m.Config)

	if m.CrossPkgOnly > 0 {
		fmt.Fprintf(&b, "ℹ️ %d functions are covered only by tests in other packages; if the coverprofile was made without -coverpkg it counts same-package tests only. Run `go test -coverpkg=./... -coverprofile=…` for line coverage.\n\n", m.CrossPkgOnly)
	}
	writeRemovedCases(&b, m.RemovedCases)
	writeUnused(&b, m.Unused)
	writeConfig(&b, m.UnusedConfig)
	writeUnusedList(&b, "🧩 **Generated code no longer used** — remove it from its source (.sql, .proto, interface) and regenerate", m.UnusedGenerated)
	writeRegistrations(&b, "🔌 **Registrations removed, no replacement** — these calls no longer get a handler; check each is intended", "base", m.RegRemoved)
	writeRewired(&b, m.RegRewired)
	writeChanged(&b, m.RegChanged)
	writeRegistrations(&b, "🔌 **Registrations added**", "head", m.RegAdded)
	writeSensitive(&b, m.Sensitive)
	writeArch(&b, m.Arch)

	// With no rated functions there is nothing for the table, diagram or
	// caller list to show; only the context lines and limits remain.
	// shown: the rows the table, uncovered lines, diagram and caller list
	// cover. Compact layout: only the visible ones.
	shown := funcs
	if len(funcs) > 0 {
		if len(funcs) > compactAbove {
			var hidden []FuncRow
			shown, hidden = splitCompact(funcs)
			writeCompactTable(&b, funcs, shown, hidden)
		} else {
			writeTable(&b, funcs)
			shown = funcs[:min(maxTableRows, len(funcs))]
		}
		if m.Coverage == "coverprofile" {
			writeUncovered(&b, shown)
		}
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
		if len(funcs) > compactAbove {
			if len(shown) > 0 {
				writeDiagram(&b, shown)
				writeCallers(&b, shown)
			}
		} else {
			writeDiagram(&b, funcs)
			writeCallers(&b, funcs)
		}
	}

	b.WriteString("<details><summary>⚠️ Limits of this analysis</summary>\n\n")
	b.WriteString("- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.\n")
	b.WriteString("- This repo only: callers in other services (HTTP/gRPC) are not visible.\n")
	b.WriteString("- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers (methods the standard library calls on interface values, such as `Error`, `Unwrap` or `ServeHTTP`, count as reached from the code that converts their type to an interface; see the list in the reference); where such a function is registered is listed under Registered.\n")
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

// writeConfigNote writes the one config line under the header: the
// effective settings that narrow or widen detection, and what the MR changed.
func writeConfigNote(b *strings.Builder, c *ConfigNote) {
	if c == nil {
		return
	}
	var parts []string
	if len(c.Critical) > 0 {
		parts = append(parts, "critical "+codeJoin(c.Critical))
	}
	if len(c.Removes) > 0 {
		parts = append(parts, "removes "+codeJoin(c.Removes))
	}
	if len(c.Adds) > 0 {
		parts = append(parts, "adds "+codeJoin(c.Adds))
	}
	if c.Architecture != "" {
		parts = append(parts, "architecture "+c.Architecture)
	}
	if len(parts) == 0 {
		parts = []string{"defaults"}
	}
	b.WriteString("⚙️ " + c.File + ": " + strings.Join(parts, " · "))
	if c.Changed {
		b.WriteString(" — changed in this MR")
		switch {
		case c.BaseUnreadable:
			b.WriteString(" (base config unreadable)")
		case len(c.Changes) > 0:
			var cs []string
			for _, ch := range c.Changes {
				cs = append(cs, "MR "+ch.What+": "+codeJoin(ch.Items))
			}
			b.WriteString(" (" + strings.Join(cs, "; ") + ")")
		}
	}
	b.WriteString("\n\n")
}

// codeJoin renders items as comma-separated code spans.
func codeJoin(items []string) string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = codeSpan(s)
	}
	return strings.Join(out, ", ")
}

// codeSpan wraps s in a code span; one holding a backtick takes a double
// backtick fence padded with spaces.
func codeSpan(s string) string {
	if strings.Contains(s, "`") {
		return "`` " + s + " ``"
	}
	return "`" + s + "`"
}

func writeTable(b *strings.Builder, funcs []FuncRow) {
	cut := funcs[:min(maxTableRows, len(funcs))]
	writeRows(b, cut, nil, true)
	if len(funcs) > maxTableRows {
		fmt.Fprintf(b, "\n_… and %d more_\n", len(funcs)-maxTableRows)
	}
	b.WriteString("\n")
	writeMembers(b, cut)
}

// writeRows writes one table of rows: the columns the rows need, the "All
// changes shown are reached from" line when stated is set, and the rows. The
// common entry points are computed over rows unless given (the collapsed
// table omits the visible table's); it returns the ones used.
func writeRows(b *strings.Builder, rows []FuncRow, given []string, stated bool) []string {
	withReg, withEntries := false, false
	for _, f := range rows {
		if len(f.Registered) > 0 {
			withReg = true
		}
		if len(f.Entries) > 0 {
			withEntries = true
		}
	}
	head, sep := "| Risk | Function | Change | Callers |", "|---|---|---|---|"
	if withReg {
		head, sep = head+" Registered |", sep+"---|"
	}
	if withEntries {
		head, sep = head+" Entry points |", sep+"---|"
	}
	common := given
	if stated && withEntries {
		common = commonEntries(rows)
	}
	if stated && len(common) > 0 {
		b.WriteString("All changes shown are reached from: " + strings.Join(common, ", ") + "\n\n")
	}
	b.WriteString(head + " Added-line coverage |\n" + sep + "---|\n")
	for _, f := range rows {
		fmt.Fprintf(b, "| %s | `%s` | %s | %s | ", f.Level.Emoji(), cell(f.Display), cell(f.Kind), callerCell(f.Callers))
		if withReg {
			b.WriteString(registeredCell(f.Registered) + " | ")
		}
		if withEntries {
			b.WriteString(entryCell(without(entryLabels(f.Entries), common)) + " | ")
		}
		fmt.Fprintf(b, "%s |\n", cell(f.CoverText))
	}
	return common
}

// splitCompact splits the table's rows (the first maxTableRows of funcs, in
// order) for the compact layout: visible is every red row then the first
// compactYellow yellow ones; hidden is the rest, in order.
func splitCompact(funcs []FuncRow) (visible, hidden []FuncRow) {
	yellow := 0
	for _, f := range funcs[:min(maxTableRows, len(funcs))] {
		switch {
		case f.Level == risk.Red:
			visible = append(visible, f)
		case f.Level == risk.Yellow && yellow < compactYellow:
			yellow++
			visible = append(visible, f)
		default:
			hidden = append(hidden, f)
		}
	}
	return visible, hidden
}

// writeCompactTable writes the visible table, then the collapsed one with the
// rest. The summary counts every row not visible, including those past the
// row cap; the overflow line for those sits inside the collapsed block.
func writeCompactTable(b *strings.Builder, funcs, visible, hidden []FuncRow) {
	var common []string
	if len(visible) > 0 { // all green: nothing to show above the fold
		common = writeRows(b, visible, nil, true)
		b.WriteString("\n")
		writeMembers(b, visible)
	}

	var yellow, green int
	for _, f := range funcs {
		if f.Level == risk.Green {
			green++
		} else if f.Level == risk.Yellow {
			yellow++
		}
	}
	for _, f := range visible {
		if f.Level == risk.Yellow {
			yellow--
		}
	}
	var parts []string
	if yellow > 0 {
		parts = append(parts, fmt.Sprintf("🟡 %d", yellow))
	}
	if green > 0 {
		parts = append(parts, fmt.Sprintf("🟢 %d", green))
	}
	// A red row past the cap is hidden too: it counts, in its own part.
	if red := len(funcs) - len(visible) - yellow - green; red > 0 {
		parts = append([]string{fmt.Sprintf("🔴 %d", red)}, parts...)
	}
	if len(funcs) == len(visible) { // nothing hidden: no collapsed block
		return
	}
	more := "more "
	if len(visible) == 0 { // nothing above the fold: the block is the whole table
		more = ""
	}
	fmt.Fprintf(b, "<details><summary>%d %schanged functions (%s)</summary>\n\n", len(funcs)-len(visible), more, strings.Join(parts, " · "))
	if len(hidden) > 0 {
		writeRows(b, hidden, common, false)
	}
	if len(funcs) > maxTableRows {
		fmt.Fprintf(b, "\n_… and %d more_\n", len(funcs)-maxTableRows)
	}
	b.WriteString("\n")
	writeMembers(b, hidden)
	b.WriteString("</details>\n\n")
}

// writeLookFirst names the first lookFirstN red or yellow rows in table
// order, with the rule that rated each, and counts the rest.
func writeLookFirst(b *strings.Builder, funcs []FuncRow) {
	var items []string
	n := 0
	for _, f := range funcs {
		if f.Level == risk.Green {
			continue
		}
		n++
		if n > lookFirstN {
			continue
		}
		item := "`" + f.Display + "`"
		if f.Reason != "" {
			item += " (" + f.Reason + ")"
		}
		items = append(items, item)
	}
	if n == 0 {
		return
	}
	b.WriteString("👉 **Look at first:** " + strings.Join(items, " · "))
	if n > lookFirstN {
		fmt.Fprintf(b, " +%d", n-lookFirstN)
	}
	b.WriteString("\n\n")
}

// writeUncovered lists, per red or yellow row in table order, the added lines
// no test covers: the row's function, then each member folded into it that
// has any (a group row sums its members' lines). funcs are the rows the
// table shows.
func writeUncovered(b *strings.Builder, funcs []FuncRow) {
	var rows []FuncRow
	for _, f := range funcs {
		if f.Level == risk.Green {
			continue
		}
		if len(f.Uncovered) > 0 {
			rows = append(rows, f)
		}
		for _, mr := range f.Members {
			if len(mr.Uncovered) > 0 {
				rows = append(rows, mr)
			}
		}
	}
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(b, "<details><summary>Uncovered added lines (%d functions)</summary>\n\n", len(rows))
	for i, f := range rows {
		if i == maxUncoveredFuncs {
			fmt.Fprintf(b, "- … and %d more\n", len(rows)-i)
			break
		}
		fmt.Fprintf(b, "- `%s` — `%s:%s`\n", f.Display, f.File, lineRanges(f.Uncovered))
	}
	b.WriteString("</details>\n\n")
}

// lineRanges merges consecutive ascending lines into "a-b, c", at most
// maxUncoveredRanges ranges, then " +N more".
func lineRanges(lines []int) string {
	var rs []string
	for i := 0; i < len(lines); {
		j := i
		for j+1 < len(lines) && lines[j+1] == lines[j]+1 {
			j++
		}
		if j > i {
			rs = append(rs, fmt.Sprintf("%d-%d", lines[i], lines[j]))
		} else {
			rs = append(rs, fmt.Sprint(lines[i]))
		}
		i = j + 1
	}
	more := ""
	if len(rs) > maxUncoveredRanges {
		more = fmt.Sprintf(" +%d more", len(rs)-maxUncoveredRanges)
		rs = rs[:maxUncoveredRanges]
	}
	return strings.Join(rs, ", ") + more
}

// writeMembers lists, per group row in table order, the new functions folded
// into it, each with its own rating, kind, cover and file. funcs are the rows
// the table shows.
func writeMembers(b *strings.Builder, funcs []FuncRow) {
	for _, f := range funcs {
		if len(f.Members) == 0 {
			continue
		}
		fmt.Fprintf(b, "<details><summary>%s +%d new</summary>\n\n", f.Display, len(f.Members))
		for _, mf := range f.Members {
			fmt.Fprintf(b, "- %s `%s` · %s · %s — `%s`\n", mf.Level.Emoji(), mf.Display, mf.Kind, mf.CoverText, mf.File)
		}
		b.WriteString("\n</details>\n\n")
	}
}

// commonEntries returns the entry-point labels every row has, in the first
// row's (the cells') order: they tell the rows apart in no way, so the table
// states them once above it and the cells leave them out. With fewer than two
// rows nothing is common.
func commonEntries(rows []FuncRow) []string {
	if len(rows) < 2 {
		return nil
	}
	var out []string
	for _, l := range entryLabels(rows[0].Entries) {
		shared := true
		for _, f := range rows[1:] {
			if !slices.Contains(entryLabels(f.Entries), l) {
				shared = false
				break
			}
		}
		if shared {
			out = append(out, l)
		}
	}
	return out
}

// entryLabels renders es in order, each label once: every main and every
// init render alike, so several of them are one label.
func entryLabels(es []callgraph.Entry) []string {
	var out []string
	for _, e := range es {
		if l := entryText(e); !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// without returns ls minus the labels in drop.
func without(ls, drop []string) []string {
	var out []string
	for _, l := range ls {
		if !slices.Contains(drop, l) {
			out = append(out, l)
		}
	}
	return out
}

// entryText renders one entry point: a registration and an api function as
// code spans, main and init by kind.
func entryText(e callgraph.Entry) string {
	switch e.Kind {
	case "api":
		return "api " + codeSpan(e.Display)
	case "registered":
		return codeSpan(e.Display)
	default: // main, init
		return e.Kind
	}
}

// entryCell shows the first two entry-point labels and how many more, or a
// dash.
func entryCell(ls []string) string {
	if len(ls) == 0 {
		return "—"
	}
	out := cell(strings.Join(ls[:min(2, len(ls))], ", "))
	if len(ls) > 2 {
		out += fmt.Sprintf(" +%d", len(ls)-2)
	}
	return out
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
	return strings.ReplaceAll(s.Text(), "`", "'")
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
		return fmt.Sprintf("- `%s` → `%s` — `%s:%d`\n", regText(r), r.Handler, r.File, r.Line)
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

// writeSensitive lists every sensitive changed function, untested first,
// then by Display.
func writeSensitive(b *strings.Builder, rs []SensitiveRow) {
	if len(rs) == 0 {
		return
	}
	rs = append([]SensitiveRow(nil), rs...)
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].Untested != rs[j].Untested {
			return rs[i].Untested
		}
		return rs[i].Display < rs[j].Display
	})
	fmt.Fprintf(b, "🔐 **Sensitive code changed** (%d):\n\n", len(rs))
	for _, r := range rs {
		var parts []string
		if len(r.APIs) > 0 {
			apis := strings.Join(r.APIs[:min(3, len(r.APIs))], ", ")
			if len(r.APIs) > 3 {
				apis += fmt.Sprintf(" +%d", len(r.APIs)-3)
			}
			parts = append(parts, apis)
		}
		if r.Critical != "" {
			parts = append(parts, "critical: "+r.Critical)
		}
		fmt.Fprintf(b, "- %s `%s` — %s · %s\n", r.Level.Emoji(), r.Display, strings.Join(parts, ", "), r.CoverText)
	}
	b.WriteString("\n")
}

// archShown reports whether the 🏛 section has anything to say: a new
// violation, a conformance change (nil vs value counts), or a coupling change.
// Without a base only head conformance is known: shown when anything was judged.
func archShown(a *arch.Result) bool {
	switch {
	case a == nil:
		return false
	case a.Inferred && a.Layered == 0:
		return false // the inferred map placed nothing in a layer (entry alone is no hint)
	case a.BaseMissing:
		return a.HeadConf != nil
	}
	confChanged := (a.BaseConf == nil) != (a.HeadConf == nil) ||
		(a.BaseConf != nil && conf(a.BaseConf) != conf(a.HeadConf))
	return len(a.New) > 0 || confChanged || len(a.Coupling) > 0
}

// conf renders a conformance percentage (the precision the section shows).
func conf(c *float64) string {
	if c == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f %%", *c)
}

// writeArch writes the 🏛 section: header, new violations, coupling deltas.
func writeArch(b *strings.Builder, a *arch.Result) {
	if !archShown(a) {
		return
	}
	rel := func(p string) string {
		if p == a.Module {
			return "."
		}
		return strings.TrimPrefix(p, a.Module+"/")
	}
	if a.Inferred {
		b.WriteString("🏛 **Architecture (inferred — hints, not enforced)**")
	} else {
		fmt.Fprintf(b, "🏛 **Architecture** (%s)", a.Model)
	}
	if a.BaseMissing {
		fmt.Fprintf(b, " · conformance %s", conf(a.HeadConf))
	} else {
		fmt.Fprintf(b, " · conformance %s → %s", conf(a.BaseConf), conf(a.HeadConf))
	}
	fmt.Fprintf(b, " · %d/%d packages classified", a.Classified, a.Total)
	if a.Inferred && len(a.TouchedLayers) > 0 {
		ls := make([]string, len(a.TouchedLayers))
		for i, t := range a.TouchedLayers {
			ls[i] = codeSpan(rel(t.Pkg)) + " → " + t.Layer
		}
		b.WriteString(" · layers: " + strings.Join(ls, ", "))
	}
	if a.BaseMissing {
		b.WriteString(" · (base not loaded: new violations not computed)\n\n")
		return
	}
	b.WriteString("\n\n")

	if len(a.New) > 0 {
		for i, v := range a.New {
			if i == maxArchList {
				fmt.Fprintf(b, "- … and %d more\n", len(a.New)-maxArchList)
				break
			}
			fmt.Fprintf(b, "- ❌ new: `%s → %s`", v.FromLayer, v.ToLayer)
			if v.Isolated {
				b.WriteString(" (isolated)")
			}
			fmt.Fprintf(b, " — `%s:%d` imports %s", v.Edge.File, v.Edge.Line, codeSpan(rel(v.Edge.To)))
			if i == 0 && a.Existing > 0 {
				fmt.Fprintf(b, " (+%s already in base)", count(a.Existing, "violation"))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(a.Coupling) > 0 {
		fmt.Fprintf(b, "<details><summary>Coupling (%s changed)</summary>\n\n", count(max(a.CouplingTotal, len(a.Coupling)), "package"))
		b.WriteString("| Package | Ca | Ce | I | A | D |\n|---|---|---|---|---|---|\n")
		for _, c := range a.Coupling {
			// A package new at head has no base value: "—".
			ints := func(base, head int) string {
				if c.New {
					return fmt.Sprintf("— → %d", head)
				}
				return fmt.Sprintf("%d → %d", base, head)
			}
			decs := func(base, head float64) string {
				if c.New {
					return fmt.Sprintf("— → %.2f", head)
				}
				return fmt.Sprintf("%.2f → %.2f", base, head)
			}
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n", cell(codeSpan(rel(c.Pkg))),
				ints(c.Base.Ca, c.Head.Ca), ints(c.Base.Ce, c.Head.Ce),
				decs(c.Base.I, c.Head.I), decs(c.Base.A, c.Head.A), decs(c.Base.D, c.Head.D))
		}
		b.WriteString("\n</details>\n\n")
	}
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
		fmt.Fprintf(b, "- `%s`: `%s` → `%s` — `%s:%d`\n", regText(r.To), r.From.Handler, r.To.Handler, r.To.File, r.To.Line)
	}
	b.WriteString("\n")
}

// writeChanged lists registrations that kept their call and handler and
// gained or lost other arguments, at their head positions.
func writeChanged(b *strings.Builder, cs []registrations.Changed) {
	if len(cs) == 0 {
		return
	}
	fmt.Fprintf(b, "🔌 **Registrations changed** (%d, positions in head):\n\n", len(cs))
	for i, c := range cs {
		if i == maxUnusedList {
			fmt.Fprintf(b, "- … and %d more\n", len(cs)-maxUnusedList)
			break
		}
		var attrs []string
		for _, a := range c.Added {
			attrs = append(attrs, "+ `"+strings.ReplaceAll(registrations.Shorten(a), "`", "'")+"`")
		}
		for _, a := range c.Removed {
			attrs = append(attrs, "− `"+strings.ReplaceAll(registrations.Shorten(a), "`", "'")+"`")
		}
		fmt.Fprintf(b, "- `%s` → `%s`: %s — `%s:%d`\n", regText(c.To), c.To.Handler, strings.Join(attrs, ", "), c.To.File, c.To.Line)
	}
	b.WriteString("\n")
}

// regText is what a registration block shows: the route when one renders,
// else the call text.
func regText(r registrations.Registration) string {
	t := r.Route
	if t == "" {
		t = r.Call
	}
	return strings.ReplaceAll(t, "`", "'")
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
	fmt.Fprintf(b, "<sub>Rules: %s · mrimpact %s · %.1f s</sub>\n", risk.Legend, m.Version, m.Duration.Seconds())
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
