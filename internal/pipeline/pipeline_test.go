package pipeline

import (
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/diff"
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

// TestDeletedErrCheckNotMoved (C1): F drops `if err != nil { return err }`
// while an unrelated new G adds its own; the row keeps "branch removed" and
// the branch-removed rating rule.
func TestDeletedErrCheckNotMoved(t *testing.T) {
	base := "package p\n\nfunc F() error {\n\terr := f()\n\tif err != nil {\n\t\treturn err\n\t}\n\treturn nil\n}\n"
	head := "package p\n\nfunc F() error {\n\t_ = f()\n\treturn nil\n}\n\nfunc G() error {\n\terr := g()\n\tif err != nil {\n\t\treturn err\n\t}\n\treturn nil\n}\n"
	src := func(s string) symbols.Source {
		return func(string) ([]byte, error) { return []byte(s), nil }
	}
	ch := []diff.FileChange{{Path: "p.go", OldPath: "p.go", Status: diff.Modified,
		AddedLines: []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}, DeletedLines: []int{4, 5, 6, 7, 8}}}
	r, err := symbols.Find(ch, src(head), src(base))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Funcs {
		if f.Display != "p.F" {
			continue
		}
		f.Wiring = false // straight-line F; only the branch suffix is under test
		if got := kindText(f, false); got != "body (branch removed)" {
			t.Errorf("kind = %q, want \"body (branch removed)\"", got)
		}
		if !branchRuleApplies(f) {
			t.Error("the branch-removed rule must apply")
		}
		return
	}
	t.Fatal("no p.F")
}
