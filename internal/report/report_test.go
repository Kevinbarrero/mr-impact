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
		Deleted:      []string{"legacy.OldTotal"},
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
		"```mermaid\ngraph RL\n",
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
		Deleted:      []string{"legacy.OldTotal"},
		Decls:        []string{"const Max in limits/limits.go"},
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
	if n := len(regexp.MustCompile(`(?m)^  n\d+\[`).FindAllString(block, -1)); n != 20 {
		t.Errorf("nodes = %d, want 20", n)
	}
	if !strings.Contains(block, "%% +11 not shown") || !strings.Contains(out, "+11 nodes not shown") {
		t.Errorf("missing not-shown markers")
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

func TestDepth2Edge(t *testing.T) {
	m := Model{
		Funcs: []FuncRow{{Level: risk.Red, Display: "a.T", Kind: "body", CoverText: "n/a", Callers: []callgraph.Caller{
			{Display: "b.One", Pkg: "b", Depth: 1, Via: "a.T"},
			{Display: "c.Two", Pkg: "c", Depth: 2, Via: "b.One"},
		}}},
		Coverage: "static", Version: "0.1.0",
	}
	out := Render(m)
	if !strings.Contains(out, "n2 --> n1") || !strings.Contains(out, "n1 --> n0") {
		t.Errorf("edges wrong:\n%s", out)
	}
}
