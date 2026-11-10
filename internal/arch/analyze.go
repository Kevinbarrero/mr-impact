package arch

import (
	"math"
	"sort"
)

// MetricDelta is a package's metrics at base and head.
type MetricDelta struct {
	Pkg        string
	Base, Head Metric
	New        bool // absent at base: Base is zero and means nothing
}

// PkgLayer is a touched package and its layer.
type PkgLayer struct{ Pkg, Layer string }

// Result is the architecture verdict of one merge request.
type Result struct {
	Model              string
	Module             string // head module path; shown paths are relative to it
	Inferred           bool
	Classified, Total  int      // head
	Layered            int      // head: classified packages that are not entry
	BaseConf, HeadConf *float64 // nil = n/a (or base missing)
	New                []Violation
	Existing           int
	BaseMissing        bool
	Coupling           []MetricDelta // capped at 10
	CouplingTotal      int           // changed touched packages before the cap
	TouchedLayers      []PkgLayer
}

const maxListed = 10

// Analyze judges head (and base, when loaded) against m. A nil base sets
// BaseMissing and computes no new/existing split and no coupling deltas.
func Analyze(m Model, base *Graph, head Graph, touched []string) Result {
	r := Result{Model: m.Name, Module: head.Module, Inferred: m.Inferred, Total: len(head.Packages), BaseMissing: base == nil}
	for _, p := range head.Packages {
		if _, ok := m.ClassifyNamed(head.Module, p, head.Names[p]); ok {
			r.Classified++
			if l, _ := m.ClassifyNamed(head.Module, p, head.Names[p]); l != EntryLayer {
				r.Layered++
			}
		}
	}
	judged, viols := m.Judge(head)
	if pct, ok := Conformance(judged, len(viols)); ok {
		r.HeadConf = &pct
	}

	// Touched layers: only classified packages, sorted, capped.
	ts := append([]string(nil), touched...)
	sort.Strings(ts)
	for _, p := range ts {
		if len(r.TouchedLayers) == maxListed {
			break
		}
		if l, ok := m.ClassifyNamed(head.Module, p, head.Names[p]); ok {
			if n := len(r.TouchedLayers); n > 0 && r.TouchedLayers[n-1].Pkg == p {
				continue
			}
			r.TouchedLayers = append(r.TouchedLayers, PkgLayer{p, l})
		}
	}
	if base == nil {
		return r
	}

	bj, bv := m.Judge(*base)
	if pct, ok := Conformance(bj, len(bv)); ok {
		r.BaseConf = &pct
	}
	inBase := map[[2]string]bool{}
	for _, e := range base.Edges {
		inBase[[2]string{e.From, e.To}] = true
	}
	for _, v := range viols {
		if inBase[[2]string{v.Edge.From, v.Edge.To}] {
			r.Existing++
		} else {
			r.New = append(r.New, v)
		}
	}

	// Coupling: touched packages present at head whose metrics visibly
	// changed (A and D compared at the 2 decimals they are rendered with).
	bm, hm := Metrics(*base), Metrics(head)
	round := func(x float64) float64 { return math.Round(x*100) / 100 }
	for _, p := range ts {
		h, ok := hm[p]
		if !ok {
			continue
		}
		if n := len(r.Coupling); n > 0 && r.Coupling[n-1].Pkg == p {
			continue
		}
		b, existed := bm[p] // a package new at head is always listed
		if existed && b.Ca == h.Ca && b.Ce == h.Ce && round(b.A) == round(h.A) && round(b.D) == round(h.D) {
			continue
		}
		r.Coupling = append(r.Coupling, MetricDelta{Pkg: p, Base: b, Head: h, New: !existed})
	}
	sort.SliceStable(r.Coupling, func(i, j int) bool {
		a, b := r.Coupling[i], r.Coupling[j]
		da, db := math.Abs(a.Head.D-a.Base.D), math.Abs(b.Head.D-b.Base.D)
		if da != db {
			return da > db
		}
		return a.Pkg < b.Pkg
	})
	r.CouplingTotal = len(r.Coupling)
	if len(r.Coupling) > maxListed {
		r.Coupling = r.Coupling[:maxListed]
	}
	return r
}
