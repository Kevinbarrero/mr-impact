package pipeline

import (
	"reflect"
	"testing"

	"gitlab.com/wh1t3hat/mrimpact/internal/callgraph"
	"gitlab.com/wh1t3hat/mrimpact/internal/report"
	"gitlab.com/wh1t3hat/mrimpact/internal/risk"
	"gitlab.com/wh1t3hat/mrimpact/internal/symbols"
)

// rf builds a new function of package m/p with a profile and cov/total
// added-line coverage, called (depth 1) by the given Displays of m/p.
func rf(display string, cov, total int, callers ...string) ratedFunc {
	in := risk.Input{Kind: symbols.New, NonTestCallers: len(callers), HasProfile: true, Covered: cov, Total: total}
	f := ratedFunc{
		Row: report.FuncRow{
			Level: risk.Assess(in), Display: display, Kind: "new",
			CoverText: coverText(true, cov, total, true, false, nil, "m"),
		},
		Key: display + "\x00m/p", New: true, In: in,
	}
	for _, c := range callers {
		f.Row.Callers = append(f.Row.Callers, callgraph.Caller{Display: c, Pkg: "m/p", Depth: 1})
		f.Callers1 = append(f.Callers1, c+"\x00m/p")
	}
	return f
}

func displays(rows []report.FuncRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Display)
	}
	return out
}

func find(t *testing.T, rows []report.FuncRow, display string) report.FuncRow {
	t.Helper()
	for _, r := range rows {
		if r.Display == display {
			return r
		}
	}
	t.Fatalf("no row %s in %v", display, displays(rows))
	return report.FuncRow{}
}

func TestGroup_MemberSharedGoesToSmallerEntry(t *testing.T) {
	fs := []ratedFunc{rf("p.B", 1, 1), rf("p.h", 1, 1, "p.A", "p.B"), rf("p.A", 1, 1)}
	rows, folded, groups := group(fs, "m")
	if got, want := displays(rows), []string{"p.B", "p.A"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if folded != 1 || groups != 1 {
		t.Errorf("folded, groups = %d, %d, want 1, 1", folded, groups)
	}
	if got := displays(find(t, rows, "p.A").Members); !reflect.DeepEqual(got, []string{"p.h"}) {
		t.Errorf("p.A members = %v", got)
	}
	if m := find(t, rows, "p.B").Members; m != nil {
		t.Errorf("p.B members = %v, want nil", displays(m))
	}
}

func TestGroup_MemberCalledByExistingCodeIsEntry(t *testing.T) {
	fs := []ratedFunc{rf("p.A", 1, 1), rf("p.h", 1, 1, "p.A", "p.Old")}
	rows, folded, groups := group(fs, "m")
	if got, want := displays(rows), []string{"p.A", "p.h"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if folded != 0 || groups != 0 {
		t.Errorf("folded, groups = %d, %d, want 0, 0", folded, groups)
	}
}

func TestGroup_CycleWithoutOutsideCaller(t *testing.T) {
	fs := []ratedFunc{rf("p.y", 1, 1, "p.x"), rf("p.x", 1, 1, "p.y")}
	rows, folded, groups := group(fs, "m")
	if got, want := displays(rows), []string{"p.x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if got := displays(rows[0].Members); !reflect.DeepEqual(got, []string{"p.y"}) {
		t.Errorf("members = %v, want [p.y]", got)
	}
	if folded != 1 || groups != 1 {
		t.Errorf("folded, groups = %d, %d, want 1, 1", folded, groups)
	}
}

func TestGroup_EntryWithoutMembersUnchanged(t *testing.T) {
	f := rf("p.A", 2, 5, "p.Old")
	rows, _, _ := group([]ratedFunc{f}, "m")
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], f.Row) || rows[0].Members != nil {
		t.Errorf("rows = %+v, want [%+v]", rows, f.Row)
	}
}

func TestGroup_RegistrationCountsAsCaller(t *testing.T) {
	h := rf("p.h", 1, 1)
	h.Row.Callers = []callgraph.Caller{{Display: "p.A", Pkg: "m/p", Depth: 1, Kind: callgraph.Registers}}
	h.Callers1 = []string{"p.A\x00m/p"}
	rows, folded, _ := group([]ratedFunc{rf("p.A", 1, 1), h}, "m")
	if folded != 1 || len(rows) != 1 || !reflect.DeepEqual(displays(rows[0].Members), []string{"p.h"}) {
		t.Errorf("rows = %v, folded = %d; want p.A with member p.h", displays(rows), folded)
	}
}

func TestGroup_SumsAndUntested(t *testing.T) {
	a := rf("p.A", 10, 10, "p.Old")
	fs := []ratedFunc{a, rf("p.h", 5, 10, "p.A"), rf("p.i", 0, 4, "p.A")}
	rows, folded, groups := group(fs, "m")
	if len(rows) != 1 || folded != 2 || groups != 1 {
		t.Fatalf("rows = %v, folded = %d, groups = %d", displays(rows), folded, groups)
	}
	r := rows[0]
	if want := "62 % (15/24) · 1 untested"; r.CoverText != want {
		t.Errorf("cover = %q, want %q", r.CoverText, want)
	}
	if r.Kind != "new · +2" {
		t.Errorf("kind = %q", r.Kind)
	}
	want := risk.Assess(risk.Input{Kind: symbols.New, NonTestCallers: 1, HasProfile: true, Covered: 15, Total: 24})
	if r.Level != want {
		t.Errorf("level = %v, want %v", r.Level, want)
	}
	if !reflect.DeepEqual(r.Callers, a.Row.Callers) {
		t.Errorf("callers = %v, want the entry's", r.Callers)
	}
	if got := displays(r.Members); !reflect.DeepEqual(got, []string{"p.h", "p.i"}) {
		t.Errorf("members = %v", got)
	}
	if !reflect.DeepEqual(r.Members[1], fs[2].Row) {
		t.Errorf("member row changed: %+v", r.Members[1])
	}
}

func TestGroup_RatingFromSumsNotWorstMember(t *testing.T) {
	h := rf("p.h", 0, 4, "p.A")
	if h.Row.Level != risk.Red {
		t.Fatalf("member alone = %v, want red", h.Row.Level)
	}
	rows, _, _ := group([]ratedFunc{rf("p.A", 10, 10, "p.Old"), h}, "m")
	if rows[0].Level != risk.Yellow {
		t.Errorf("group level = %v, want yellow", rows[0].Level)
	}
}

func TestGroup_PointerReceiverMethodIsMember(t *testing.T) {
	m := rf("p.(*T).m", 1, 1, "p.A")
	if m.Key != "p.(*T).m\x00m/p" {
		t.Fatalf("key = %q", m.Key)
	}
	rows, folded, _ := group([]ratedFunc{rf("p.A", 1, 1), m}, "m")
	if folded != 1 || len(rows) != 1 || !reflect.DeepEqual(displays(rows[0].Members), []string{"p.(*T).m"}) {
		t.Errorf("rows = %v, folded = %d", displays(rows), folded)
	}
}

func TestGroup_NonNewNeverFolded(t *testing.T) {
	b := rf("p.b", 1, 1, "p.A")
	b.New = false
	b.In.Kind = symbols.Body
	b.Row.Kind = "body"
	rows, folded, groups := group([]ratedFunc{rf("p.A", 1, 1), b}, "m")
	if got, want := displays(rows), []string{"p.A", "p.b"}; !reflect.DeepEqual(got, want) || folded != 0 || groups != 0 {
		t.Errorf("rows = %v, folded = %d, groups = %d", got, folded, groups)
	}
}

func TestGroup_NoProfileReachedByAny(t *testing.T) {
	a := rf("p.A", 0, 0, "p.Old")
	h := rf("p.h", 0, 0, "p.A")
	a.In.HasProfile, h.In.HasProfile = false, false
	h.In.TestReached = true
	rows, _, _ := group([]ratedFunc{a, h}, "m")
	if rows[0].CoverText != "reached by test" {
		t.Errorf("cover = %q", rows[0].CoverText)
	}
}

func TestGroup_CrossPkgOnSums(t *testing.T) {
	a := rf("p.A", 0, 3, "p.Old")
	h := rf("p.h", 0, 2, "p.A")
	h.In.CrossPkgTestReached = true
	h.CrossPkgs = []string{"m/q"}
	rows, _, _ := group([]ratedFunc{a, h}, "m")
	if want := "0 % in profile · reached by tests in q · 1 untested"; rows[0].CoverText != want {
		t.Errorf("cover = %q, want %q", rows[0].CoverText, want)
	}
	if rows[0].Level != risk.Yellow {
		t.Errorf("level = %v, want yellow", rows[0].Level)
	}
}

func TestGroup_WiringOnlyIfAll(t *testing.T) {
	for _, all := range []bool{true, false} {
		a := rf("p.A", 0, 3, "p.Old")
		h := rf("p.h", 0, 2, "p.A")
		a.In.Wiring, h.In.Wiring = true, all
		rows, _, _ := group([]ratedFunc{a, h}, "m")
		want := risk.Red
		if all {
			want = risk.Yellow
		}
		if rows[0].Level != want || rows[0].Kind != "new · +1" {
			t.Errorf("all wiring %v: level, kind = %v, %q, want %v, \"new · +1\"", all, rows[0].Level, rows[0].Kind, want)
		}
	}
}
