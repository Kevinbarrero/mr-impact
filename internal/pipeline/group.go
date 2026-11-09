package pipeline

import (
	"fmt"
	"sort"

	"gitlab.com/wh1t3hat/mrimpact/internal/report"
	"gitlab.com/wh1t3hat/mrimpact/internal/risk"
)

// ratedFunc is one rated, non-deleted changed function as the pipeline built it.
type ratedFunc struct {
	Row       report.FuncRow
	Key       string     // Display + "\x00" + package path: matches Caller.Display/Pkg
	New       bool       // symbols.New
	In        risk.Input // the input its Row.Level came from
	Callers1  []string   // Keys of depth-1 non-test callers, registrants (Kind Registers) included
	CrossPkgs []string   // packages of other-package tests reaching it (the cover text's); nil if none
}

// group folds members into entries; rows keep input order for entries/others.
//
// A member is a new function with at least one depth-1 non-test caller or
// registrant, every one of them a new function in fs. Each member goes to
// the smallest-Display entry it is reachable from through members only; a
// member reachable from no entry (a cycle with no outside caller) goes to the
// smallest Display of its connected unclaimed set, which becomes the entry.
// folded counts members, groups the entries that got any.
func group(fs []ratedFunc, mod string) (rows []report.FuncRow, folded, groups int) {
	idx := make(map[string]int, len(fs))
	for i, f := range fs {
		if f.New {
			idx[f.Key] = i
		}
	}
	member := make([]bool, len(fs))
	for i, f := range fs {
		if !f.New || len(f.Callers1) == 0 {
			continue
		}
		member[i] = true
		for _, c := range f.Callers1 {
			if _, ok := idx[c]; !ok {
				member[i] = false
				break
			}
		}
	}
	// children: caller → member callees; neigh: member edges both ways.
	children := make([][]int, len(fs))
	neigh := make([][]int, len(fs))
	for i, f := range fs {
		if !member[i] {
			continue
		}
		for _, c := range f.Callers1 {
			j := idx[c]
			children[j] = append(children[j], i)
			if member[j] {
				neigh[i] = append(neigh[i], j)
				neigh[j] = append(neigh[j], i)
			}
		}
	}

	byDisplay := make([]int, len(fs))
	for i := range fs {
		byDisplay[i] = i
	}
	sort.SliceStable(byDisplay, func(a, b int) bool {
		x, y := fs[byDisplay[a]], fs[byDisplay[b]]
		if x.Row.Display != y.Row.Display {
			return x.Row.Display < y.Row.Display
		}
		return x.Key < y.Key
	})

	owner := make([]int, len(fs)) // entry index of a claimed member, else -1
	for i := range owner {
		owner[i] = -1
	}
	claim := func(entry int, next [][]int) {
		queue := []int{entry}
		for len(queue) > 0 {
			u := queue[0]
			queue = queue[1:]
			for _, v := range next[u] {
				if member[v] && v != entry && owner[v] < 0 {
					owner[v] = entry
					queue = append(queue, v)
				}
			}
		}
	}
	for _, i := range byDisplay {
		if fs[i].New && !member[i] {
			claim(i, children)
		}
	}
	for _, i := range byDisplay {
		if member[i] && owner[i] < 0 {
			member[i] = false // promoted to entry
			claim(i, neigh)
		}
	}

	membersOf := make(map[int][]int)
	for _, i := range byDisplay {
		if member[i] {
			membersOf[owner[i]] = append(membersOf[owner[i]], i)
		}
	}
	for i, f := range fs {
		switch {
		case member[i]:
			folded++
		case len(membersOf[i]) > 0:
			groups++
			rows = append(rows, groupRow(f, fs, membersOf[i], mod))
		default:
			rows = append(rows, f.Row)
		}
	}
	return rows, folded, groups
}

// groupRow is the entry's row with the figures summed over entry and members.
func groupRow(entry ratedFunc, fs []ratedFunc, members []int, mod string) report.FuncRow {
	in := entry.In
	cross := entry.In.CrossPkgTestReached
	crossPkgs := append([]string(nil), entry.CrossPkgs...)
	untested := 0
	row := entry.Row
	row.Members = make([]report.FuncRow, 0, len(members))
	for _, m := range members {
		f := fs[m]
		in.Covered += f.In.Covered
		in.Total += f.In.Total
		in.TestReached = in.TestReached || f.In.TestReached
		in.Wiring = in.Wiring && f.In.Wiring
		cross = cross || f.In.CrossPkgTestReached
		crossPkgs = append(crossPkgs, f.CrossPkgs...)
		if f.In.Total > 0 && f.In.Covered == 0 {
			untested++
		}
		row.Members = append(row.Members, f.Row)
	}
	in.CrossPkgTestReached = cross && in.Covered == 0 && in.Total > 0
	if !in.CrossPkgTestReached {
		crossPkgs = nil
	} else {
		crossPkgs = sortedUnique(crossPkgs)
	}
	row.Level = risk.Assess(in)
	row.Kind = fmt.Sprintf("new · +%d", len(members))
	row.CoverText = coverText(in.HasProfile, in.Covered, in.Total, true, in.TestReached, crossPkgs, mod)
	if untested > 0 {
		row.CoverText += fmt.Sprintf(" · %d untested", untested)
	}
	return row
}

func sortedUnique(s []string) []string {
	sort.Strings(s)
	out := s[:0]
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}
