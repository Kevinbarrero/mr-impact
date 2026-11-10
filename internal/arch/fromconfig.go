package arch

import (
	"fmt"
	"slices"
	"strings"

	"gitlab.com/wh1t3hat/mrimpact/internal/config"
)

// FromConfig builds the model of an architecture block: Inferred() when the
// block is absent. Configured layers come first (file order, so their globs
// win), then the remaining preset layers in preset order. Errors wrap
// config.ErrConfig.
func FromConfig(a config.Architecture) (Model, error) {
	if !a.Set {
		return Inferred(), nil
	}
	bad := func(format string, args ...any) (Model, error) {
		return Model{}, fmt.Errorf("%w: architecture: %s", config.ErrConfig, fmt.Sprintf(format, args...))
	}
	var m Model
	switch a.Preset {
	case "":
		if len(a.Layers) == 0 {
			return bad("needs a preset or layers")
		}
		m = Model{Name: "custom", Kind: Layers}
	case "modular":
		if len(a.Layers) > 0 || len(a.Allow) > 0 || len(a.Isolated) > 0 {
			return bad("preset modular takes no layers, allow or isolated")
		}
		return NewModular(a.ModulesRoot), nil
	case "pipeline":
		if len(a.Stages) == 0 {
			return bad("preset pipeline requires stages")
		}
		m = NewPipeline(a.Stages)
	default:
		p, ok := Preset(a.Preset)
		if !ok {
			return bad("unknown preset %q", a.Preset)
		}
		m = p
	}

	base := m.Layers
	m.Layers = nil
	for _, cl := range a.Layers {
		var l Layer
		if i := slices.IndexFunc(base, func(b Layer) bool { return b.Name == cl.Name }); i >= 0 {
			l = base[i]
		} else {
			l = Layer{Name: cl.Name}
		}
		l.Globs = append(slices.Clone(l.Globs), cl.Globs...)
		m.Layers = append(m.Layers, l)
	}
	for _, b := range base {
		if !slices.ContainsFunc(m.Layers, func(l Layer) bool { return l.Name == b.Name }) {
			m.Layers = append(m.Layers, b)
		}
	}

	known := func(n string) bool {
		return n == EntryLayer || slices.ContainsFunc(m.Layers, func(l Layer) bool { return l.Name == n })
	}
	idx := func(n string) int {
		return slices.IndexFunc(m.Layers, func(l Layer) bool { return l.Name == n })
	}
	for _, s := range a.Allow {
		f := strings.Fields(s)
		if len(f) != 3 || f[1] != "->" {
			return bad("allow %q is not \"<layer> -> <layer>\"", s)
		}
		from, to := f[0], f[2]
		for _, n := range []string{from, to} {
			if !known(n) {
				return bad("allow %q names unknown layer %q", s, n)
			}
		}
		if i := idx(from); i >= 0 && !slices.Contains(m.Layers[i].MayImport, to) {
			m.Layers[i].MayImport = append(m.Layers[i].MayImport, to)
		}
	}
	for _, n := range a.Isolated {
		if !known(n) {
			return bad("isolated names unknown layer %q", n)
		}
		if i := idx(n); i >= 0 {
			m.Layers[i].Isolated = true
		}
	}
	return m, nil
}
