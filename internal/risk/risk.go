// Package risk assigns a deterministic red/yellow/green rating to a changed
// Go function from its kind, export status, caller count and test coverage.
package risk

import (
	"gitlab.com/wh1t3hat/mrimpact/internal/symbols"
)

// Level is a risk rating; higher values are riskier.
type Level int

const (
	Green Level = iota
	Yellow
	Red
)

// Emoji returns the marker used for the level in the report.
func (l Level) Emoji() string {
	switch l {
	case Red:
		return "🔴"
	case Yellow:
		return "🟡"
	default:
		return "🟢"
	}
}

// Input is the evidence Assess rates.
type Input struct {
	Kind           symbols.Kind
	Exported       bool
	NonTestCallers int
	HasProfile     bool // coverprofile in use
	Covered, Total int  // added-line coverage (only meaningful if HasProfile)
	TestReached    bool
	// Importable: other modules can import the package (not main, not
	// under internal/). Only then can an exported signature change break
	// callers the compiler doesn't see.
	Importable      bool
	BranchesRemoved bool // control flow removed: never rated green
}

// Legend explains the rating in the report footer.
const Legend = "🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, or a branch removed · 🟢 covered (no added lines: reached by a test)"

// Assess rates a changed function. It must not be called for symbols.Deleted,
// which has no code left to rate; doing so panics.
func Assess(in Input) Level {
	if in.Kind == symbols.Deleted {
		panic("risk.Assess: called with symbols.Deleted; deleted functions must not be rated")
	}

	var untested, partial bool
	if in.HasProfile && in.Total > 0 {
		untested = in.Covered == 0
		partial = in.Covered > 0 && in.Covered < in.Total
	} else {
		untested = !in.TestReached // static, or 0 added lines
	}

	switch {
	case in.Exported && in.Kind == symbols.Signature && in.Importable:
		return Red
	case untested && in.NonTestCallers >= 1:
		return Red
	case partial:
		return Yellow
	case untested && in.NonTestCallers == 0:
		return Yellow
	case in.BranchesRemoved:
		return Yellow
	default:
		return Green
	}
}
