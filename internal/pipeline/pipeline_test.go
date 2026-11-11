package pipeline

import (
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/symbols"
)

func TestKindTextMoved(t *testing.T) {
	f := func(removed, moved int, to ...string) symbols.ChangedFunc {
		return symbols.ChangedFunc{Kind: symbols.Body, BranchesRemoved: true,
			RemovedBranches: removed, MovedBranches: moved, MovedTo: to}
	}
	withSuffix := f(2, 2, "p.a")
	withSuffix.Wiring = true
	cases := []struct {
		name string
		in   symbols.ChangedFunc
		sens bool
		want string
	}{
		{"all moved", f(3, 3, "p.checkLimit"), false, "body (branch moved to `p.checkLimit`)"},
		{"three destinations", f(3, 3, "p.a", "p.b", "p.c"), false, "body (branch moved to `p.a`, `p.b` +1)"},
		{"partly", f(3, 1, "p.a"), false, "body (branch removed · 1 of 3 moved to `p.a`)"},
		{"none", f(3, 0), false, "body (branch removed)"},
		{"suffixes", withSuffix, true, "body (branch moved to `p.a`) · wiring · sensitive"},
	}
	for _, c := range cases {
		if got := kindText(c.in, c.sens); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
