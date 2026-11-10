package report

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gitlab.com/wh1t3hat/mrimpact/internal/arch"
	"gitlab.com/wh1t3hat/mrimpact/internal/callgraph"
	"gitlab.com/wh1t3hat/mrimpact/internal/config"
	"gitlab.com/wh1t3hat/mrimpact/internal/registrations"
	"gitlab.com/wh1t3hat/mrimpact/internal/risk"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden.md")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s (run with -update to rewrite)\n--- got ---\n%s", path, got)
	}
}

// callers builds n depth-1 callers of via, named <pkg>.F0.. in package pkg.
func callers(pkg, via string, n int) []callgraph.Caller {
	var cs []callgraph.Caller
	for i := 0; i < n; i++ {
		cs = append(cs, callgraph.Caller{Display: fmt.Sprintf("%s.F%02d", pkg, i), Pkg: "ex.com/" + pkg, Depth: 1, Via: via})
	}
	return cs
}

func specModel() Model {
	total := []callgraph.Caller{
		{Display: "checkout.Handle", Pkg: "ex.com/checkout", Depth: 1, Via: "pricing.(*Calculator).Total"},
		{Display: "billing.Generate", Pkg: "ex.com/billing", Depth: 1, Via: "pricing.(*Calculator).Total"},
		{Display: "orders.applyDiscount", Pkg: "ex.com/orders", Depth: 1, Via: "pricing.(*Calculator).Total"},
	}
	total = append(total, callers("reports", "pricing.(*Calculator).Total", 11)...)
	disc := []callgraph.Caller{
		{Display: "orders.Create", Pkg: "ex.com/orders", Depth: 1, Via: "orders.applyDiscount"},
		{Display: "orders.Update", Pkg: "ex.com/orders", Depth: 1, Via: "orders.applyDiscount"},
		{Display: "orders.Retry", Pkg: "ex.com/orders", Depth: 1, Via: "orders.applyDiscount"},
		{Display: "orders.Replay", Pkg: "ex.com/orders", Depth: 1, Via: "orders.applyDiscount"},
		{Display: "orders.Import", Pkg: "ex.com/orders", Depth: 1, Via: "orders.applyDiscount"},
	}
	return Model{
		Funcs: []FuncRow{
			{Level: risk.Green, Display: "httpapi.formatDate", Kind: "new", Callers: callers("httpapi", "httpapi.formatDate", 1), CoverText: "100 % (4/4)"},
			{Level: risk.Red, Display: "orders.applyDiscount", Kind: "body", Callers: disc, CoverText: "0 % (0/7)"},
			{Level: risk.Yellow, Display: "orders.Validate", Kind: "body", Callers: callers("orders", "orders.Validate", 2), CoverText: "40 % (2/5)"},
			{Level: risk.Red, Display: "pricing.(*Calculator).Total", Kind: "signature (exported)", Callers: total, CoverText: "0 % (0/12)"},
		},
		Deleted:      []DeletedFunc{{Display: "legacy.OldTotal", File: "legacy/legacy.go"}},
		TestsTouched: []string{"orders/validate_test.go"},
		Coverage:     "coverprofile",
		BaseSHA:      "a1b2c3d4e5f6",
		Version:      "0.1.0",
		Duration:     3200 * time.Millisecond,
	}
}

func TestRenderSpecExample(t *testing.T) {
	out := Render(specModel())
	for _, want := range []string{
		"<!-- mrimpact:v1 -->\n### 🔍 MR impact — 4 functions changed\n",
		"**🔴 2 · 🟡 1 · 🟢 1** · coverage: `coverprofile` · base `a1b2c3d`",
		"| Risk | Function | Change | Callers | Added-line coverage |",
		"| 🔴 | `pricing.(*Calculator).Total` | signature (exported) | 14 (4 pkgs) | 0 % (0/12) |",
		"➖ Deleted: `legacy.OldTotal`",
		"🧪 Tests touched in this MR: `orders/validate_test.go`",
		"```mermaid\nflowchart RL\n",
		"**How to read the diagram:**",
		"<details><summary>All callers (22)</summary>",
		"<details><summary>⚠️ Limits of this analysis</summary>",
		"<sub>Rules: " + risk.Legend + " · mrimpact v0.1.0 · 3.2 s</sub>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	// order: red/most callers first
	if i, j := strings.Index(out, "`pricing.(*Calculator).Total` |"), strings.Index(out, "`orders.applyDiscount` |"); i > j {
		t.Error("table not sorted by risk then callers")
	}
	golden(t, "spec_example", out)
}

func TestRenderFailure(t *testing.T) {
	golden(t, "failure", Render(Model{Failure: "load error: boom", Version: "0.1.0", Duration: 1500 * time.Millisecond}))
}

func TestRenderNoGoChanges(t *testing.T) {
	golden(t, "no_go_changes", Render(Model{NoGoChanges: true, Version: "0.1.0", Duration: 100 * time.Millisecond}))
}

func TestRenderNoFuncs(t *testing.T) {
	out := Render(Model{
		Deleted:      []DeletedFunc{{Display: "legacy.OldTotal", File: "legacy/legacy.go"}},
		Decls:        []Decl{{What: "const Max", File: "limits/limits.go"}},
		TestsTouched: []string{"limits/limits_test.go"},
		Coverage:     "static", BaseSHA: "abcdef0123", Version: "0.1.0", Duration: time.Second,
	})
	for _, absent := range []string{"| Risk |", "```mermaid", "All callers"} {
		if strings.Contains(out, absent) {
			t.Errorf("output contains %q with no functions", absent)
		}
	}
	golden(t, "no_funcs", out)
}

func TestRenderDegradedWithLimits(t *testing.T) {
	m := specModel()
	m.Degraded = true
	m.Coverage = "static"
	m.NotAnalyzed = []string{"orders.Special (build tags)", "pkg/x (load error)"}
	m.Warnings = []string{"coverage profile is older than the base commit"}
	out := Render(m)
	if !strings.Contains(out, " · ⚠️ degraded") {
		t.Error("missing degraded marker")
	}
	golden(t, "degraded_limits", out)
}

func TestRenderDiagramCap(t *testing.T) {
	m := Model{
		Funcs:    []FuncRow{{Level: risk.Red, Display: "a.Target", Kind: "body", Callers: callers("c", "a.Target", 30), CoverText: "n/a"}},
		Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second,
	}
	out := Render(m)
	block := out[strings.Index(out, "```mermaid"):]
	block = block[:strings.Index(block[10:], "```")+10]
	// The function, 3 callers and one "+27 more callers" node.
	if n := len(regexp.MustCompile(`(?m)^  n\d+[\[(]`).FindAllString(block, -1)); n != 5 {
		t.Errorf("nodes = %d, want 5:\n%s", n, block)
	}
	if !strings.Contains(block, `n4(["+27 more callers"])`) || !strings.Contains(block, "n4 -.-> n0") {
		t.Errorf("missing the +27 node:\n%s", block)
	}
	if !strings.Contains(block, "subgraph legend") {
		t.Error("missing legend")
	}
	golden(t, "diagram_cap", out)
}

func TestRenderTableCap(t *testing.T) {
	m := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second}
	for i := 0; i < 60; i++ {
		m.Funcs = append(m.Funcs, FuncRow{Level: risk.Green, Display: fmt.Sprintf("p.F%02d", i), Kind: "body", CoverText: "n/a"})
	}
	out := Render(m)
	if n := strings.Count(out, "\n| 🟢 |"); n != 50 {
		t.Errorf("rows = %d, want 50", n)
	}
	if !strings.Contains(out, "10 more") {
		t.Error("missing 10 more")
	}
	golden(t, "table_cap", out)
}

func TestMermaidEscaping(t *testing.T) {
	m := Model{
		Funcs: []FuncRow{
			{Level: risk.Red, Display: `list.(*List[T]).Push`, Kind: "body", CoverText: "n/a", Callers: []callgraph.Caller{
				{Display: `x.say("hi")`, Pkg: "ex.com/x", Depth: 1, Via: `list.(*List[T]).Push`},
			}},
		},
		Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0",
	}
	out := Render(m)
	if !strings.Contains(out, `n0["🔴 list.(*List[T]).Push"]:::red`) {
		t.Errorf("label not quoted:\n%s", out)
	}
	if !strings.Contains(out, `x.say(#quot;hi#quot;)`) {
		t.Errorf("quote not escaped:\n%s", out)
	}
	for _, id := range regexp.MustCompile(`(?m)^  (\S+?)(\[| -->)`).FindAllStringSubmatch(out, -1) {
		if strings.ContainsAny(id[1], "([.") {
			t.Errorf("bad node id %q", id[1])
		}
	}
}

// A registrant gets one dashed edge to the handler; the generic dispatcher
// it registered through is not drawn. Plain depth-2 callers are not drawn.
func TestRegistrantEdge(t *testing.T) {
	m := Model{
		Funcs: []FuncRow{{Level: risk.Red, Display: "a.T", Kind: "body", CoverText: "n/a", Callers: []callgraph.Caller{
			{Display: "bus.(*Bus).Subscribe", Pkg: "bus", Depth: 1, Via: "a.T", Kind: callgraph.Dispatch},
			{Display: "a.New", Pkg: "a", Depth: 2, Via: "bus.(*Bus).Subscribe", Kind: callgraph.Registers},
			{Display: "b.One", Pkg: "b", Depth: 1, Via: "a.T"},
			{Display: "c.Two", Pkg: "c", Depth: 2, Via: "b.One"},
		}}},
		Coverage: "static", Version: "0.1.0",
	}
	out := Render(m)
	for _, want := range []string{`n1["a.New"]`, `n1 -. "registers via Subscribe" .-> n0`, `n2["b.One"]`, "n2 --> n0"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `["bus.(*Bus).Subscribe"]`) || strings.Contains(out, `["c.Two"]`) || strings.Contains(out, "more caller") {
		t.Errorf("dispatcher, depth-2 caller or a +N node drawn:\n%s", out)
	}
}

// Callers are distinct per (Display, Pkg): main.run in two commands is two
// callers, while a repeat within one package is still deduplicated.
func TestCallerDedupKeepsPackages(t *testing.T) {
	m := Model{
		Funcs: []FuncRow{{Level: risk.Red, Display: "a.T", Kind: "body", CoverText: "n/a", Callers: []callgraph.Caller{
			{Display: "main.run", Pkg: "ex.com/cmd/api", Depth: 1, Via: "a.T"},
			{Display: "main.run", Pkg: "ex.com/cmd/worker", Depth: 1, Via: "a.T"},
			{Display: "main.run", Pkg: "ex.com/cmd/worker", Depth: 2, Via: "a.T"},
		}}},
		Coverage: "static", Version: "0.1.0",
	}
	out := Render(m)
	if !strings.Contains(out, "| 🔴 | `a.T` | body | 2 (2 pkgs) | n/a |") {
		t.Errorf("callers cell wrong:\n%s", out)
	}
	if !strings.Contains(out, "All callers (2)") || !strings.Contains(out, "**`a.T`** (2)") {
		t.Errorf("all-callers counts wrong:\n%s", out)
	}
	if n := strings.Count(out, `["main.run"]`); n != 1 {
		t.Errorf("diagram has %d main.run nodes, want 1:\n%s", n, out)
	}
}

// A large removal is counted and collapsed, grouped by package directory.
func TestRenderLargeDeletionCollapsed(t *testing.T) {
	m := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second,
		DeletedFiles: []string{"a/x.go", "b/y.go"}}
	for i := 0; i < 4; i++ {
		m.Deleted = append(m.Deleted, DeletedFunc{Display: fmt.Sprintf("a.F%d", i), File: "a/x.go"})
		m.Deleted = append(m.Deleted, DeletedFunc{Display: fmt.Sprintf("b.(*T).M%d", i), File: "b/y.go"})
	}
	for i := 0; i < 7; i++ {
		m.Decls = append(m.Decls, Decl{What: fmt.Sprintf("var v%d", i), File: "a/z.go"})
	}
	out := Render(m)
	for _, want := range []string{
		"➖ Deleted: 8 functions in 2 packages · 2 files removed\n",
		"<details><summary>Deleted functions (8)</summary>",
		"- `a` (4): `F0`, `F1`, `F2`, `F3`\n",
		"- `b` (4): `(*T).M0`, `(*T).M1`, `(*T).M2`, `(*T).M3`\n",
		"📝 Declarations changed: 7 in 1 file\n",
		"- `a/z.go` (7): `var v0`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestRenderUnusedAndRegistered(t *testing.T) {
	m := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second,
		Unused: []callgraph.Unused{
			{Display: "x.lost", File: "x/x.go", Line: 3},
			{Display: "x.helper", File: "x/x.go", Line: 9, TestOnly: true},
		},
		Funcs: []FuncRow{{Level: risk.Yellow, Display: "x.(*svc).handle", Kind: "body (branch removed)", CoverText: "no added lines · reached by test",
			Callers:    []callgraph.Caller{{Display: "bus.Subscribe", Pkg: "ex.com/bus", Depth: 1, Via: "x.(*svc).handle"}},
			Registered: []callgraph.Site{{File: "x/x.go", Line: 20, Call: "Subscribe(topic.A.EventTopic(), …)"}, {File: "x/y.go", Line: 4}}}},
	}
	out := Render(m)
	for _, want := range []string{
		"🪦 **Now unused** — nothing reachable calls these after this MR (1):\n\n- `x.lost` — `x/x.go:3`\n",
		"🧫 **Now only used by tests** — no production path reaches these after this MR (1):\n\n- `x.helper` — `x/x.go:9`\n",
		"| Risk | Function | Change | Callers | Registered | Added-line coverage |",
		"| 🟡 | `x.(*svc).handle` | body (branch removed) | 1 | `Subscribe(topic.A.EventTopic(), …)` +1 | no added lines · reached by test |",
		"- registered: `x/y.go:4` — `x/y.go:4`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Index(out, "Now unused") > strings.Index(out, "| Risk |") {
		t.Error("unused section must come before the table")
	}
}

func TestRender_Groups(t *testing.T) {
	m := Model{Coverage: "coverprofile", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second,
		Folded: 2, Groups: 1,
		Funcs: []FuncRow{
			{Level: risk.Yellow, Display: "svc.New", Kind: "new · +2", File: "svc/svc.go",
				Callers:   []callgraph.Caller{{Display: "main.main", Pkg: "ex.com/cmd", Depth: 1, Via: "svc.New"}},
				CoverText: "67 % (4/6) · 1 untested",
				Members: []FuncRow{
					{Level: risk.Green, Display: "svc.load", Kind: "new", CoverText: "100 % (2/2)", File: "svc/load.go"},
					{Level: risk.Yellow, Display: "svc.fmt", Kind: "new", CoverText: "0 % (0/2)", File: "svc/fmt.go"},
				}},
			{Level: risk.Green, Display: "svc.Old", Kind: "body", File: "svc/old.go", CoverText: "100 % (1/1)",
				Callers: []callgraph.Caller{{Display: "main.main", Pkg: "ex.com/cmd", Depth: 1, Via: "svc.Old"}}},
		},
	}
	out := Render(m)
	for _, want := range []string{
		"### 🔍 MR impact — 4 functions changed\n**🔴 0 · 🟡 1 · 🟢 1** · coverage: `coverprofile` · base `abcdef0`\\\n+2 new functions folded into 1 group\n\n",
		"| 🟡 | `svc.New` | new · +2 | 1 | 67 % (4/6) · 1 untested |\n",
		"<details><summary>svc.New +2 new</summary>\n\n- 🟢 `svc.load` · new · 100 % (2/2) — `svc/load.go`\n- 🟡 `svc.fmt` · new · 0 % (0/2) — `svc/fmt.go`\n\n</details>\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, absent := range []string{"svc/svc.go", "svc/old.go", "\"🟢 svc.load\"", "\"🟡 svc.fmt\""} {
		if strings.Contains(out, absent) {
			t.Errorf("output contains %q", absent)
		}
	}
	golden(t, "groups", out)
}

func TestRender_FoldLinePlural(t *testing.T) {
	m := Model{Coverage: "coverprofile", BaseSHA: "abcdef0", Folded: 3, Groups: 2,
		Funcs: []FuncRow{{Level: risk.Green, Display: "p.A", Kind: "new · +3", CoverText: "100 % (1/1)"}}}
	if out := Render(m); !strings.Contains(out, "\\\n+3 new functions folded into 2 groups\n\n") {
		t.Errorf("missing plural fold line in\n%s", out)
	}
}

func TestRender_CrossPkgHint(t *testing.T) {
	m := Model{Coverage: "coverprofile", BaseSHA: "abcdef0", CrossPkgOnly: 2,
		Funcs: []FuncRow{{Level: risk.Yellow, Display: "p.A", Kind: "new", CoverText: "0 % in profile · reached by tests in q"}}}
	want := "ℹ️ 2 functions are covered only by tests in other packages; if the coverprofile was made without -coverpkg it counts same-package tests only. Run `go test -coverpkg=./... -coverprofile=…` for line coverage.\n\n"
	if out := Render(m); !strings.Contains(out, want) {
		t.Errorf("missing %q in\n%s", want, out)
	}
}

// Members are listed only for group rows the table shows.
func TestRender_MembersOnlyForShownRows(t *testing.T) {
	var funcs []FuncRow
	for i := 0; i < maxTableRows; i++ {
		funcs = append(funcs, FuncRow{Level: risk.Yellow, Display: fmt.Sprintf("p.f%02d", i), Kind: "body", CoverText: "100 % (1/1)"})
	}
	funcs = append(funcs, FuncRow{Level: risk.Yellow, Display: "p.zHidden", Kind: "new · +1", CoverText: "50 % (1/2)",
		Members: []FuncRow{{Level: risk.Yellow, Display: "p.h", Kind: "new", CoverText: "0 % (0/1)", File: "p/h.go"}}})
	funcs[0].Kind, funcs[0].Members = "new · +1", []FuncRow{{Level: risk.Green, Display: "p.g", Kind: "new", CoverText: "100 % (1/1)", File: "p/g.go"}}
	out := Render(Model{Coverage: "coverprofile", BaseSHA: "abcdef0", Folded: 2, Groups: 2, Funcs: funcs})
	if !strings.Contains(out, "<summary>p.f00 +1 new</summary>") {
		t.Errorf("shown group's members missing")
	}
	if strings.Contains(out, "p.zHidden +1 new") || strings.Contains(out, "p/h.go") {
		t.Errorf("members of a row past the table are listed:\n%s", out)
	}
}

func TestRender_SensitiveSection(t *testing.T) {
	m := Model{Coverage: "coverprofile", BaseSHA: "abcdef0",
		Funcs:    []FuncRow{{Level: risk.Red, Display: "pkg.F", Kind: "body · sensitive", CoverText: "0 % (0/7)"}},
		RegAdded: []registrations.Registration{{Call: `GET("/x", …)`, Handler: "h.X", Callee: "GET", File: "h/h.go", Line: 3}},
		Sensitive: []SensitiveRow{
			{Level: risk.Green, Display: "a.Many", APIs: []string{"os.Remove", "os.Rename", "reflect.TypeOf", "unsafe.Pointer"}, CoverText: "100 % (3/3)"},
			{Level: risk.Yellow, Display: "a.Partial", APIs: []string{"os/exec.Command"}, Critical: "internal/a/...", CoverText: "50 % (1/2)"},
			{Level: risk.Red, Display: "pay.Settle", Critical: "internal/pay/...", CoverText: "not reached", Untested: true},
			{Level: risk.Red, Display: "pkg.F", APIs: []string{"os/exec.Command", "database/sql.(*Tx).Exec"}, CoverText: "0 % (0/7)", Untested: true},
		},
	}
	out := Render(m)
	want := "🔐 **Sensitive code changed** (4):\n\n" +
		"- 🔴 `pay.Settle` — critical: internal/pay/... · not reached\n" +
		"- 🔴 `pkg.F` — os/exec.Command, database/sql.(*Tx).Exec · 0 % (0/7)\n" +
		"- 🟢 `a.Many` — os.Remove, os.Rename, reflect.TypeOf +1 · 100 % (3/3)\n" +
		"- 🟡 `a.Partial` — os/exec.Command, critical: internal/a/... · 50 % (1/2)\n\n"
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in\n%s", want, out)
	}
	reg, sec, table := strings.Index(out, "Registrations added"), strings.Index(out, "🔐"), strings.Index(out, "| Risk |")
	if !(reg < sec && sec < table) {
		t.Errorf("section must come after the registration sections and before the table:\n%s", out)
	}
	if strings.Contains(Render(Model{Coverage: "static", BaseSHA: "abcdef0"}), "🔐") {
		t.Error("empty section rendered")
	}
}

func TestRender_EntryPointsColumn(t *testing.T) {
	base := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second}
	with := base
	with.Funcs = []FuncRow{
		{Level: risk.Yellow, Display: "x.A", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{
			{Kind: "registered", Display: `GET("/x", …)`}, {Kind: "api", Display: "x.Do"}, {Kind: "main", Display: "main.main"}}},
		{Level: risk.Yellow, Display: "x.B", Kind: "body", CoverText: "n/a"},
		{Level: risk.Yellow, Display: "x.C", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{
			{Kind: "registered", Display: `GET("/a|b", …)`}, {Kind: "init", Display: "x.init"}}},
	}
	out := Render(with)
	for _, want := range []string{
		"| Risk | Function | Change | Callers | Entry points | Added-line coverage |",
		"| 🟡 | `x.A` | body | 0 | `GET(\"/x\", …)`, api `x.Do` +1 | n/a |",
		"| 🟡 | `x.B` | body | 0 | — | n/a |",
		"| `GET(\"/a\\|b\", …)`, init | n/a |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	none := base
	none.Funcs = []FuncRow{{Level: risk.Yellow, Display: "x.B", Kind: "body", CoverText: "n/a"}}
	if strings.Contains(Render(none), "Entry points") {
		t.Error("column present with no entries")
	}
}

// main is dropped from the cells when every shown row is reached from main:
// it says nothing there. A cell left empty renders a dash.
func TestRender_EntryPointsDropsCommonMain(t *testing.T) {
	m := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second}
	m.Funcs = []FuncRow{
		{Level: risk.Yellow, Display: "x.A", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{
			{Kind: "api", Display: "x.Do"}, {Kind: "main", Display: "main.main"}, {Kind: "init", Display: "x.init"}}},
		{Level: risk.Yellow, Display: "x.B", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{
			{Kind: "main", Display: "main.main"}}},
	}
	out := Render(m)
	for _, want := range []string{"| `x.A` | body | 0 | api `x.Do`, init | n/a |", "| `x.B` | body | 0 | — | n/a |",
		"All changes shown are reached from: main\n\n| Risk |"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	// One shown row not reached from main: main stays everywhere.
	m.Funcs = append(m.Funcs, FuncRow{Level: risk.Yellow, Display: "x.C", Kind: "body", CoverText: "n/a"})
	out = Render(m)
	for _, want := range []string{"| `x.A` | body | 0 | api `x.Do`, main +1 | n/a |", "| `x.B` | body | 0 | main | n/a |"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "All changes shown") {
		t.Errorf("common line with a row reached from nothing:\n%s", out)
	}
}

// (a) A registration every shown row has is left out of the cells and
// stated once above the table.
func TestRender_EntryPointsCommonRegistration(t *testing.T) {
	run := callgraph.Entry{Kind: "registered", Display: "RunContext(…)"}
	m := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second}
	m.Funcs = []FuncRow{
		{Level: risk.Yellow, Display: "x.A", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{
			run, {Kind: "registered", Display: `Sub("a", …)`}}},
		{Level: risk.Yellow, Display: "x.B", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{run}},
	}
	out := Render(m)
	for _, want := range []string{
		"All changes shown are reached from: `RunContext(…)`\n\n| Risk |",
		"| `x.A` | body | 0 | `Sub(\"a\", …)` | n/a |",
		"| `x.B` | body | 0 | — | n/a |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Count(out, "RunContext") != 1 {
		t.Errorf("RunContext stated more than once:\n%s", out)
	}
}

// (b) main and a registration both shared: both stated, in the cells' order.
func TestRender_EntryPointsCommonMainAndRegistration(t *testing.T) {
	run := callgraph.Entry{Kind: "registered", Display: "RunContext(…)"}
	main := callgraph.Entry{Kind: "main", Display: "main.main"}
	m := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second}
	m.Funcs = []FuncRow{
		{Level: risk.Yellow, Display: "x.A", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{
			run, {Kind: "api", Display: "x.Do"}, main}},
		{Level: risk.Yellow, Display: "x.B", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{run, main, {Kind: "init", Display: "x.init"}}},
	}
	out := Render(m)
	for _, want := range []string{
		"All changes shown are reached from: `RunContext(…)`, main\n\n| Risk |",
		"| `x.A` | body | 0 | api `x.Do` | n/a |",
		"| `x.B` | body | 0 | init | n/a |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

// (c) With a single row nothing is common: the table is unchanged.
func TestRender_EntryPointsSingleRowUnchanged(t *testing.T) {
	m := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second}
	m.Funcs = []FuncRow{{Level: risk.Yellow, Display: "x.A", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{
		{Kind: "registered", Display: "RunContext(…)"}, {Kind: "main", Display: "main.main"}}}}
	out := Render(m)
	if !strings.Contains(out, "| `x.A` | body | 0 | `RunContext(…)`, main | n/a |") || strings.Contains(out, "All changes shown") {
		t.Errorf("single row changed:\n%s", out)
	}
}

func TestRender_ConfigLine(t *testing.T) {
	base := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second,
		Funcs: []FuncRow{{Level: risk.Yellow, Display: "x.A", Kind: "body", CoverText: "n/a"}}}
	cases := []struct {
		name string
		cfg  *ConfigNote
		want string // "" = no config line
	}{
		{"none", nil, ""},
		{"settings", &ConfigNote{File: ".mrimpact.yml", Critical: []string{"internal/pay/...", "x/*"}, Removes: []string{"os/exec.*"}, Adds: []string{"l.(*L).Post"}},
			"⚙️ .mrimpact.yml: critical `internal/pay/...`, `x/*` · removes `os/exec.*` · adds `l.(*L).Post`"},
		{"critical only", &ConfigNote{File: ".mrimpact.yml", Critical: []string{"a/..."}},
			"⚙️ .mrimpact.yml: critical `a/...`"},
		{"changed", &ConfigNote{File: ".mrimpact.yml", Critical: []string{"a/..."}, Removes: []string{"os/exec.*"}, Changed: true,
			Changes: []config.Change{{What: "removes from defaults", Items: []string{"os/exec.*"}}, {What: "drops critical", Items: []string{"b"}}}},
			"⚙️ .mrimpact.yml: critical `a/...` · removes `os/exec.*` — changed in this MR (MR removes from defaults: `os/exec.*`; MR drops critical: `b`)"},
		{"changed to defaults", &ConfigNote{File: ".mrimpact.yml", Changed: true,
			Changes: []config.Change{{What: "drops critical", Items: []string{"b"}}}},
			"⚙️ .mrimpact.yml: defaults — changed in this MR (MR drops critical: `b`)"},
		{"base unreadable", &ConfigNote{File: ".mrimpact.yml", Removes: []string{"os/exec.*"}, Changed: true, BaseUnreadable: true},
			"⚙️ .mrimpact.yml: removes `os/exec.*` — changed in this MR (base config unreadable)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := base
			m.Config = c.cfg
			out := Render(m)
			if c.want == "" {
				if strings.Contains(out, "⚙️") {
					t.Errorf("unexpected config line:\n%s", out)
				}
				return
			}
			// One line right after the header block.
			if !strings.Contains(out, "base `abcdef0`\n\n"+c.want+"\n\n") {
				t.Errorf("want line %q after the header in\n%s", c.want, out)
			}
		})
	}
}

// Registered and api entries render as code spans: *, _ and < stay literal;
// a backtick takes a double-backtick span; | is still escaped.
func TestRender_EntryCodeSpans(t *testing.T) {
	m := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second}
	m.Funcs = []FuncRow{
		{Level: risk.Yellow, Display: "x.A", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{
			{Kind: "registered", Display: "Handle(`a|b`, …)"}, {Kind: "api", Display: "x.(*T).M"}}},
		{Level: risk.Yellow, Display: "x.B", Kind: "body", CoverText: "n/a", Entries: []callgraph.Entry{
			{Kind: "registered", Display: "Use(*_<x>, …)"}, {Kind: "init", Display: "x.init"}}},
	}
	out := Render(m)
	for _, want := range []string{
		"| `x.A` | body | 0 | `` Handle(`a\\|b`, …) ``, api `x.(*T).M` | n/a |",
		"| `x.B` | body | 0 | `Use(*_<x>, …)`, init | n/a |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

const archMod = "example.com/app"

func pct(x float64) *float64 { return &x }

func viol(from, to, file string, line int, fl, tl string, iso bool) arch.Violation {
	return arch.Violation{Edge: arch.Edge{From: archMod + "/" + from, To: archMod + "/" + to, File: file, Line: line},
		FromLayer: fl, ToLayer: tl, Isolated: iso}
}

func archModel(a *arch.Result) Model {
	return Model{Coverage: "static", BaseSHA: "abcdef0",
		Funcs:     []FuncRow{{Level: risk.Green, Display: "order.F", Kind: "body", CoverText: "n/a"}},
		Sensitive: []SensitiveRow{{Level: risk.Green, Display: "order.F", APIs: []string{"os.Remove"}, CoverText: "n/a"}},
		Arch:      a}
}

func TestRender_ArchDeclared(t *testing.T) {
	a := &arch.Result{Model: "hexagonal", Module: archMod, Classified: 214, Total: 225,
		BaseConf: pct(96.0), HeadConf: pct(95.2), Existing: 3,
		New: []arch.Violation{
			viol("internal/domain/order", "internal/adapters/pg", "internal/domain/order/order.go", 7, "domain", "adapters", false),
			viol("internal/adapters/http", "internal/adapters/pg", "internal/adapters/http/h.go", 5, "adapters", "adapters", true),
		},
		Coupling: []arch.MetricDelta{{Pkg: archMod + "/internal/domain/order",
			Base: arch.Metric{Ca: 4, Ce: 1, I: 0.2, A: 0.25, D: 0.55},
			Head: arch.Metric{Ca: 4, Ce: 2, I: 1.0 / 3, A: 0.25, D: 0.4167}}},
		TouchedLayers: []arch.PkgLayer{{Pkg: archMod + "/internal/domain/order", Layer: "domain"}},
	}
	out := Render(archModel(a))
	want := "🏛 **Architecture** (hexagonal) · conformance 96.0 % → 95.2 % · 214/225 packages classified\n\n" +
		"- ❌ new: `domain → adapters` — `internal/domain/order/order.go:7` imports `internal/adapters/pg` (+3 violations already in base)\n" +
		"- ❌ new: `adapters → adapters` (isolated) — `internal/adapters/http/h.go:5` imports `internal/adapters/pg`\n\n" +
		"<details><summary>Coupling (1 package changed)</summary>\n\n" +
		"| Package | Ca | Ce | I | A | D |\n" +
		"|---|---|---|---|---|---|\n" +
		"| `internal/domain/order` | 4 → 4 | 1 → 2 | 0.20 → 0.33 | 0.25 → 0.25 | 0.55 → 0.42 |\n\n" +
		"</details>\n\n"
	if !strings.Contains(out, want) {
		t.Errorf("missing\n%s\nin\n%s", want, out)
	}
	if strings.Contains(out, "layers:") {
		t.Error("declared mode must not list layers")
	}
	sec, arc, table := strings.Index(out, "🔐"), strings.Index(out, "🏛"), strings.Index(out, "| Risk |")
	if !(sec < arc && arc < table) {
		t.Errorf("🏛 must come after 🔐 and before the table:\n%s", out)
	}
	// A custom model names itself.
	a.Model = "custom"
	if out := Render(archModel(a)); !strings.Contains(out, "🏛 **Architecture** (custom) · ") {
		t.Errorf("custom header missing:\n%s", out)
	}
}

func TestRender_ArchInferred(t *testing.T) {
	a := &arch.Result{Model: "inferred", Inferred: true, Module: archMod, Classified: 3, Total: 5,
		BaseConf: nil, HeadConf: pct(50),
		New: []arch.Violation{viol("internal/domain/order", "internal/repository/pg", "internal/domain/order/o.go", 3, "core", "adapters", false)},
		TouchedLayers: []arch.PkgLayer{
			{Pkg: archMod + "/internal/domain/order", Layer: "core"},
			{Pkg: archMod + "/internal/repository/pg", Layer: "adapters"},
		},
	}
	out := Render(archModel(a))
	want := "🏛 **Architecture (inferred — hints, not enforced)** · conformance n/a → 50.0 % · 3/5 packages classified" +
		" · layers: `internal/domain/order` → core, `internal/repository/pg` → adapters\n\n" +
		"- ❌ new: `core → adapters` — `internal/domain/order/o.go:3` imports `internal/repository/pg`\n\n"
	if !strings.Contains(out, want) {
		t.Errorf("missing\n%s\nin\n%s", want, out)
	}
	if strings.Contains(out, "Coupling") {
		t.Error("no coupling block expected")
	}
}

func TestRender_ArchCap(t *testing.T) {
	a := &arch.Result{Model: "layered", Module: archMod, Classified: 2, Total: 2, BaseConf: pct(100), HeadConf: pct(10)}
	for i := range 17 {
		a.New = append(a.New, viol("internal/domain/x", fmt.Sprintf("internal/handler/h%02d", i), "internal/domain/x/x.go", i+1, "domain", "presentation", false))
	}
	out := Render(archModel(a))
	if n := strings.Count(out, "- ❌ new: "); n != 15 {
		t.Errorf("%d violation lines, want 15", n)
	}
	if !strings.Contains(out, "imports `internal/handler/h14`\n- … and 2 more\n\n") {
		t.Errorf("missing cap line after the 15th:\n%s", out)
	}
	if strings.Contains(out, "already in base") {
		t.Error("Existing = 0 must say nothing")
	}
}

func TestRender_ArchOmittedWhenNothing(t *testing.T) {
	for name, a := range map[string]*arch.Result{
		"nil":            nil,
		"same conf":      {Model: "hexagonal", Module: archMod, BaseConf: pct(90), HeadConf: pct(90), Existing: 4},
		"both n/a":       {Model: "inferred", Inferred: true, Module: archMod, TouchedLayers: []arch.PkgLayer{{Pkg: archMod + "/internal/db", Layer: "adapters"}}},
		"base missing 0": {Model: "hexagonal", Module: archMod, BaseMissing: true},
	} {
		if out := Render(archModel(a)); strings.Contains(out, "🏛") {
			t.Errorf("%s: section rendered:\n%s", name, out)
		}
	}
	// Each trigger alone renders it.
	for name, a := range map[string]*arch.Result{
		"conf nil→value": {Model: "hexagonal", Module: archMod, HeadConf: pct(100)},
		"conf changed":   {Model: "hexagonal", Module: archMod, BaseConf: pct(90), HeadConf: pct(80)},
		"coupling":       {Model: "hexagonal", Module: archMod, BaseConf: pct(90), HeadConf: pct(90), Coupling: []arch.MetricDelta{{Pkg: archMod + "/a", Head: arch.Metric{Ce: 1, I: 1}}}},
	} {
		if out := Render(archModel(a)); !strings.Contains(out, "🏛") {
			t.Errorf("%s: section not rendered:\n%s", name, out)
		}
	}
}

func TestRender_ArchBaseMissing(t *testing.T) {
	a := &arch.Result{Model: "hexagonal", Module: archMod, Classified: 4, Total: 4, HeadConf: pct(75), BaseMissing: true,
		TouchedLayers: []arch.PkgLayer{{Pkg: archMod + "/internal/domain", Layer: "domain"}}}
	out := Render(archModel(a))
	want := "🏛 **Architecture** (hexagonal) · conformance 75.0 % · 4/4 packages classified · (base not loaded: new violations not computed)\n\n"
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in\n%s", want, out)
	}
	a.Inferred, a.Model = true, "inferred"
	want = "🏛 **Architecture (inferred — hints, not enforced)** · conformance 75.0 % · 4/4 packages classified · layers: `internal/domain` → domain · (base not loaded: new violations not computed)\n\n"
	if out := Render(archModel(a)); !strings.Contains(out, want) {
		t.Errorf("missing %q in\n%s", want, out)
	}
}

func TestRender_ArchSingularAndNewPackage(t *testing.T) {
	a := &arch.Result{Model: "hexagonal", Module: archMod, Classified: 2, Total: 2, BaseConf: pct(100), HeadConf: pct(50), Existing: 1,
		New: []arch.Violation{viol("internal/domain/a", "internal/adapters/b", "internal/domain/a/a.go", 3, "domain", "adapters", false)},
		Coupling: []arch.MetricDelta{
			{Pkg: archMod + "/internal/domain/a", Base: arch.Metric{Ca: 1, D: 1}, Head: arch.Metric{Ca: 1, Ce: 1, I: 0.5, D: 0.5}},
			{Pkg: archMod + "/internal/adapters/b", New: true, Head: arch.Metric{Ca: 1, D: 1}},
		},
		CouplingTotal: 12,
	}
	out := Render(archModel(a))
	for _, want := range []string{
		"imports `internal/adapters/b` (+1 violation already in base)\n",
		"<details><summary>Coupling (12 packages changed)</summary>\n",
		"| `internal/adapters/b` | — → 1 | — → 0 | — → 0.00 | — → 0.00 | — → 1.00 |\n",
		"| `internal/domain/a` | 1 → 1 | 0 → 1 | 0.00 → 0.50 | 0.00 → 0.00 | 1.00 → 0.50 |\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

// Ruling R17: an inferred model that classifies nothing at head has no hint
// to give, whatever else changed.
func TestRender_ArchInferredNothingClassified(t *testing.T) {
	a := &arch.Result{Model: "inferred", Inferred: true, Module: archMod, Classified: 0, Total: 3,
		Coupling:      []arch.MetricDelta{{Pkg: archMod + "/svc", New: true, Head: arch.Metric{Ca: 1, D: 1}}},
		CouplingTotal: 1,
	}
	if out := Render(archModel(a)); strings.Contains(out, "🏛") {
		t.Errorf("section rendered with nothing classified:\n%s", out)
	}
	a.Classified = 1
	if out := Render(archModel(a)); !strings.Contains(out, "🏛") {
		t.Errorf("section missing once a package is classified:\n%s", out)
	}
}

func TestRender_ConfigLineArchitecture(t *testing.T) {
	m := Model{Coverage: "static", BaseSHA: "abcdef0", Version: "0.1.0", Duration: time.Second,
		Funcs: []FuncRow{{Level: risk.Yellow, Display: "x.A", Kind: "body", CoverText: "n/a"}},
		Config: &ConfigNote{File: ".mrimpact.yml", Critical: []string{"a/..."}, Architecture: "hexagonal", Changed: true,
			Changes: []config.Change{{What: "adds architecture", Items: []string{"hexagonal"}}}}}
	want := "⚙️ .mrimpact.yml: critical `a/...` · architecture hexagonal — changed in this MR (MR adds architecture: `hexagonal`)"
	if got := Render(m); !strings.Contains(got, want+"\n") {
		t.Errorf("missing %q in:\n%s", want, got)
	}
}
