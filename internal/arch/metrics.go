package arch

import "math"

// Metric holds Martin's package metrics: afferent/efferent coupling,
// instability, abstractness and distance from the main sequence.
type Metric struct {
	Ca, Ce  int
	I, A, D float64
}

// Metrics computes the metrics of every package of g.
func Metrics(g Graph) map[string]Metric {
	out := make(map[string]Metric, len(g.Packages))
	for _, p := range g.Packages {
		out[p] = Metric{}
	}
	for _, e := range g.Edges {
		m := out[e.From]
		m.Ce++
		out[e.From] = m
		m = out[e.To]
		m.Ca++
		out[e.To] = m
	}
	for p, m := range out {
		if n := m.Ca + m.Ce; n > 0 {
			m.I = float64(m.Ce) / float64(n)
		}
		if n := g.Named[p]; n > 0 {
			m.A = float64(g.Interfaces[p]) / float64(n)
		}
		m.D = math.Abs(m.A + m.I - 1)
		out[p] = m
	}
	return out
}
