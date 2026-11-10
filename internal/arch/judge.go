package arch

import "sort"

// Violation is an edge that breaks the model.
type Violation struct {
	Edge               Edge
	FromLayer, ToLayer string
	Isolated           bool
}

// Judge classifies both ends of every edge; edges with an unclassified end are
// not judged. Violations are sorted by file, line, target.
func (m Model) Judge(g Graph) (judged int, violations []Violation) {
	for _, e := range g.Edges {
		fl, ok1 := m.ClassifyNamed(g.Module, e.From, g.Names[e.From])
		tl, ok2 := m.ClassifyNamed(g.Module, e.To, g.Names[e.To])
		if !ok1 || !ok2 {
			continue
		}
		judged++
		okEdge, isolated := m.allowed(g.Module, e, fl, tl)
		if !okEdge {
			violations = append(violations, Violation{Edge: e, FromLayer: fl, ToLayer: tl, Isolated: isolated})
		}
	}
	sort.SliceStable(violations, func(i, j int) bool {
		a, b := violations[i].Edge, violations[j].Edge
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.To < b.To
	})
	return judged, violations
}

// allowed reports whether the edge conforms; isolated is set when the
// isolation rule was the one broken. entry -> entry is allowed (same layer).
func (m Model) allowed(module string, e Edge, from, to string) (ok, isolated bool) {
	if from == EntryLayer && to == EntryLayer {
		return true, false
	}
	if from == EntryLayer {
		return true, false
	}
	if to == EntryLayer {
		return false, false
	}
	if m.Kind == Modular {
		if from == to {
			return true, false
		}
		return e.To == module+"/"+m.ModulesRoot+"/"+to, false
	}
	if from == to {
		for _, l := range m.Layers {
			if l.Name == from && l.Isolated && e.From != e.To &&
				!hasDirPrefix(e.From, e.To) && !hasDirPrefix(e.To, e.From) {
				return false, true
			}
		}
		return true, false
	}
	for _, l := range m.Layers {
		if l.Name != from {
			continue
		}
		for _, t := range l.MayImport {
			if t == to {
				return true, false
			}
		}
	}
	return false, false
}

func hasDirPrefix(path, ancestor string) bool {
	return len(path) > len(ancestor) && path[:len(ancestor)] == ancestor && path[len(ancestor)] == '/'
}

// Conformance returns conforming/judged*100; ok is false when nothing was judged.
func Conformance(judged, violating int) (pct float64, ok bool) {
	if judged == 0 {
		return 0, false
	}
	return float64(judged-violating) / float64(judged) * 100, true
}
