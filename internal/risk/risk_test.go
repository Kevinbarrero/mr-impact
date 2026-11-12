package risk

import (
	"strings"
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/symbols"
)

func TestAssess(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Level
	}{
		{
			name: "exported signature change fully covered is red",
			in: Input{Kind: symbols.Signature, Exported: true, Importable: true, NonTestCallers: 2,
				HasProfile: true, Covered: 5, Total: 5},
			want: Red,
		},
		{
			name: "exported signature change in a non-importable package is rated like a body change",
			in: Input{Kind: symbols.Signature, Exported: true, Importable: false, NonTestCallers: 2,
				HasProfile: true, Covered: 5, Total: 5},
			want: Green,
		},
		{
			name: "removed branch with no added lines, reached by a test, is yellow",
			in:   Input{Kind: symbols.Body, NonTestCallers: 3, HasProfile: true, TestReached: true, BranchesRemoved: true},
			want: Yellow,
		},
		{
			name: "removed branch never hides red",
			in:   Input{Kind: symbols.Body, NonTestCallers: 3, HasProfile: true, BranchesRemoved: true},
			want: Red,
		},
		{
			name: "unexported signature change covered is green",
			in: Input{Kind: symbols.Signature, Exported: false, NonTestCallers: 2,
				HasProfile: true, Covered: 5, Total: 5},
			want: Green,
		},
		{
			name: "0 of 5 covered with 3 callers is red",
			in: Input{Kind: symbols.Body, NonTestCallers: 3,
				HasProfile: true, Covered: 0, Total: 5},
			want: Red,
		},
		{
			name: "2 of 5 covered is yellow",
			in: Input{Kind: symbols.Body, NonTestCallers: 3,
				HasProfile: true, Covered: 2, Total: 5},
			want: Yellow,
		},
		{
			name: "0 of 5 covered with 0 callers is yellow",
			in: Input{Kind: symbols.Body, NonTestCallers: 0,
				HasProfile: true, Covered: 0, Total: 5},
			want: Yellow,
		},
		{
			name: "5 of 5 covered is green",
			in: Input{Kind: symbols.Body, NonTestCallers: 3,
				HasProfile: true, Covered: 5, Total: 5},
			want: Green,
		},
		{
			name: "static not reached with callers is red",
			in: Input{Kind: symbols.Body, NonTestCallers: 3,
				HasProfile: false, TestReached: false},
			want: Red,
		},
		{
			name: "static reached is green",
			in: Input{Kind: symbols.Body, NonTestCallers: 3,
				HasProfile: false, TestReached: true},
			want: Green,
		},
		{
			name: "profile with total 0 and test reached with callers is green",
			in: Input{Kind: symbols.Body, NonTestCallers: 4,
				HasProfile: true, Covered: 0, Total: 0, TestReached: true},
			want: Green,
		},
		{
			name: "profile with total 0 and test not reached with callers is red",
			in: Input{Kind: symbols.Body, NonTestCallers: 4,
				HasProfile: true, Covered: 0, Total: 0, TestReached: false},
			want: Red,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Assess(tt.in); got != tt.want {
				t.Errorf("Assess(%+v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestAssessPanicsOnDeleted(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Assess with symbols.Deleted did not panic")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "deleted") {
			t.Fatalf("panic message = %v, want a message mentioning deleted", r)
		}
	}()
	Assess(Input{Kind: symbols.Deleted})
}

func TestLevelEmoji(t *testing.T) {
	tests := map[Level]string{Green: "🟢", Yellow: "🟡", Red: "🔴"}
	for l, want := range tests {
		if got := l.Emoji(); got != want {
			t.Errorf("Level(%d).Emoji() = %q, want %q", l, got, want)
		}
	}
}

func TestLegend(t *testing.T) {
	const want = "🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, a branch removed, or added lines not measured · 🟢 covered, or no added lines and reached by a test · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴"
	if Legend != want {
		t.Errorf("Legend = %q, want %q", Legend, want)
	}
}

func TestAssess_CrossPkgIsPartial(t *testing.T) {
	in := Input{Kind: symbols.Body, NonTestCallers: 2, HasProfile: true, Covered: 0, Total: 5}
	if got := Assess(in); got != Red {
		t.Fatalf("without flag = %v, want Red", got)
	}
	in.CrossPkgTestReached = true
	if got := Assess(in); got != Yellow {
		t.Fatalf("with flag = %v, want Yellow", got)
	}
}

func TestAssess_WiringCapsRed(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Level
	}{
		{"untested with callers", Input{Kind: symbols.Body, Wiring: true, NonTestCallers: 2, HasProfile: true, Covered: 0, Total: 5}, Yellow},
		{"covered", Input{Kind: symbols.Body, Wiring: true, NonTestCallers: 2, HasProfile: true, Covered: 5, Total: 5}, Green},
		{"partial", Input{Kind: symbols.Body, Wiring: true, NonTestCallers: 2, HasProfile: true, Covered: 2, Total: 5}, Yellow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Assess(tt.in); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssess_WiringDoesNotCapExportedSignature(t *testing.T) {
	in := Input{Kind: symbols.Signature, Exported: true, Importable: true, Wiring: true,
		NonTestCallers: 2, HasProfile: true, Covered: 5, Total: 5}
	if got := Assess(in); got != Red {
		t.Fatalf("got %v, want Red", got)
	}
}

func TestLegend_MentionsWiring(t *testing.T) {
	if !strings.Contains(Legend, "· wiring (straight-line setup) at most 🟡") {
		t.Fatalf("Legend = %q", Legend)
	}
}

func TestAssess_SensitiveUntestedNoCallers(t *testing.T) {
	in := Input{Kind: symbols.Body, Sensitive: true, HasProfile: true, Covered: 0, Total: 5, AddedLines: 5}
	if got := Assess(in); got != Red {
		t.Fatalf("got %v, want Red", got)
	}
}

// Wiring, main and init are exempt from the untested-sensitive rule: they
// keep today's rating (wiring stays capped at yellow).
func TestAssess_SensitiveExemptions(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Level
	}{
		{"wiring, callers", Input{Kind: symbols.Body, Sensitive: true, Wiring: true, HasProfile: true, Total: 5, AddedLines: 5, NonTestCallers: 2}, Yellow},
		{"wiring, no callers", Input{Kind: symbols.Body, Sensitive: true, Wiring: true, HasProfile: true, Total: 5, AddedLines: 5}, Yellow},
		{"root, no callers, static", Input{Kind: symbols.Body, Sensitive: true, Root: true, AddedLines: 5}, Yellow},
		{"root, partial", Input{Kind: symbols.Body, Sensitive: true, Root: true, HasProfile: true, Covered: 2, Total: 5, AddedLines: 5}, Yellow},
		{"root, untested profile", Input{Kind: symbols.Body, Sensitive: true, Root: true, HasProfile: true, Total: 5, AddedLines: 5}, Yellow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.in.SensitiveUntested() {
				t.Error("SensitiveUntested = true")
			}
			if got := Assess(tt.in); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssess_SensitiveBeatsCrossPkg(t *testing.T) {
	in := Input{Kind: symbols.Body, Sensitive: true, CrossPkgTestReached: true, HasProfile: true, Covered: 0, Total: 5, AddedLines: 5}
	if got := Assess(in); got != Red {
		t.Fatalf("got %v, want Red", got)
	}
}

func TestAssess_SensitiveStatic(t *testing.T) {
	in := Input{Kind: symbols.Body, Sensitive: true, AddedLines: 3}
	if got := Assess(in); got != Red {
		t.Fatalf("not reached = %v, want Red", got)
	}
	in.TestReached = true
	// Reached by a test: not the red sensitive-untested rule, but without a
	// profile the added lines are not measured, so at most yellow.
	if got := Rate(in); got.Level != Yellow || got.Reason != "added lines not measured" {
		t.Fatalf("reached = %+v, want Yellow \"added lines not measured\"", got)
	}
}

func TestAssess_SensitivePartial(t *testing.T) {
	in := Input{Kind: symbols.Body, Sensitive: true, HasProfile: true, Covered: 2, Total: 5}
	if got := Assess(in); got != Yellow {
		t.Fatalf("got %v, want Yellow", got)
	}
}

func TestAssess_SensitiveCovered(t *testing.T) {
	in := Input{Kind: symbols.Body, Sensitive: true, HasProfile: true, Covered: 5, Total: 5}
	if got := Assess(in); got != Green {
		t.Fatalf("got %v, want Green", got)
	}
}

func TestAssess_SensitiveNoAddedLines(t *testing.T) {
	in := Input{Kind: symbols.Body, Sensitive: true, HasProfile: true, Total: 0, TestReached: true}
	if got := Assess(in); got != Green {
		t.Fatalf("got %v, want Green", got)
	}
}

func TestAssess_SensitiveProfileNoAddedLinesKeepsTodaysRating(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Level
	}{
		{"no callers", Input{Kind: symbols.Body, Sensitive: true, HasProfile: true, Total: 0}, Yellow},
		{"callers, wiring", Input{Kind: symbols.Body, Sensitive: true, HasProfile: true, Total: 0, NonTestCallers: 2, Wiring: true}, Yellow},
		{"callers", Input{Kind: symbols.Body, Sensitive: true, HasProfile: true, Total: 0, NonTestCallers: 2}, Red},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Assess(tt.in); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// Without a profile, a sensitive function that only lost lines keeps
// today's rating; one with added lines no test reaches is red.
func TestAssess_SensitiveStaticAddedLines(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Level
	}{
		{"lines only removed", Input{Kind: symbols.Body, Sensitive: true}, Yellow},
		{"lines added", Input{Kind: symbols.Body, Sensitive: true, AddedLines: 3}, Red},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Assess(tt.in); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			if got := tt.in.SensitiveUntested(); got != (tt.want == Red) {
				t.Fatalf("SensitiveUntested = %v", got)
			}
		})
	}
}

// With no call graph and no profile, test reach is unknown: the
// untested-sensitive rule does not fire. A profile still decides.
func TestAssess_SensitiveReachUnknown(t *testing.T) {
	in := Input{Kind: symbols.Body, Sensitive: true, AddedLines: 3, ReachUnknown: true}
	if in.SensitiveUntested() {
		t.Error("static, reach unknown: SensitiveUntested = true")
	}
	if got := Assess(in); got != Yellow {
		t.Errorf("static, reach unknown = %v, want Yellow (today's rule)", got)
	}
	in.HasProfile, in.Total = true, 3
	if !in.SensitiveUntested() || Assess(in) != Red {
		t.Errorf("profile 0/3, reach unknown: want the rule to fire")
	}
}

func TestRate_Reasons(t *testing.T) {
	tests := []struct {
		name   string
		in     Input
		level  Level
		reason string
	}{
		{"signature", Input{Kind: symbols.Signature, Exported: true, Importable: true}, Red, "exported signature changed"},
		{"sensitive", Input{Sensitive: true, AddedLines: 3, HasProfile: true, Total: 3, Covered: 0}, Red, "sensitive, untested"},
		{"callers", Input{NonTestCallers: 12, HasProfile: true, Total: 4}, Red, "untested, 12 callers"},
		{"one caller", Input{NonTestCallers: 1, HasProfile: true, Total: 4}, Red, "untested, 1 caller"},
		{"wiring", Input{NonTestCallers: 2, Wiring: true, HasProfile: true, Total: 4}, Yellow, "wiring, untested"},
		{"partial", Input{HasProfile: true, Covered: 2, Total: 5}, Yellow, "partly tested, 40 %"},
		{"cross", Input{HasProfile: true, Total: 3, CrossPkgTestReached: true}, Yellow, "tested only from other packages"},
		{"no callers", Input{HasProfile: true, Total: 3}, Yellow, "untested, no callers"},
		{"unmeasured", Input{TestReached: true, AddedLines: 2}, Yellow, "added lines not measured"},
		{"branch", Input{TestReached: true, BranchesRemoved: true, HasProfile: true}, Yellow, "branch removed"},
		{"branch beats unmeasured", Input{TestReached: true, AddedLines: 2, BranchesRemoved: true}, Yellow, "branch removed"},
		{"green", Input{TestReached: true}, Green, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Rate(tt.in)
			if got.Level != tt.level || got.Reason != tt.reason {
				t.Errorf("Rate = %+v, want %v %q", got, tt.level, tt.reason)
			}
			if Assess(tt.in) != got.Level {
				t.Errorf("Assess != Rate.Level")
			}
		})
	}
}

func TestRate_StaticRedRulesWin(t *testing.T) {
	got := Rate(Input{NonTestCallers: 3, TestReached: false, AddedLines: 2})
	if got.Level != Red || got.Reason != "untested, 3 callers" {
		t.Errorf("got %+v", got)
	}
}

// A static run without a call graph keeps its rating and says nothing about
// added lines.
func TestRate_StaticUnknownReach(t *testing.T) {
	got := Rate(Input{ReachUnknown: true, AddedLines: 2})
	if got.Reason == "added lines not measured" {
		t.Errorf("reason = %q", got.Reason)
	}
	if got.Level != Yellow {
		t.Errorf("level = %v, want Yellow (today's value)", got.Level)
	}
}
