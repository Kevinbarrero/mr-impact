package risk

import (
	"strings"
	"testing"

	"gitlab.com/wh1t3hat/mrimpact/internal/symbols"
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
	const want = "🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, or a branch removed · 🟢 covered (no added lines: reached by a test)"
	if Legend != want {
		t.Errorf("Legend = %q, want %q", Legend, want)
	}
}
