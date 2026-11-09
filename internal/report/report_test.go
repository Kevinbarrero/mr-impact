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

	"gitlab.com/wh1t3hat/mrimpact/internal/callgraph"
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
		"### 🔍 MR impact — 4 functions changed\n**🔴 0 · 🟡 1 · 🟢 1** · coverage: `coverprofile` · base `abcdef0`\\\n+2 new functions folded into 1 groups\n\n",
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
