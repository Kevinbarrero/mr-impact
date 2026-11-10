// Package config loads the optional .mrimpact.yml. Zero-config is the
// default: without a file every setting keeps its built-in value.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrConfig wraps every error Load returns, so the CLI can map it to exit 2.
var ErrConfig = errors.New("config")

// Config is the parsed .mrimpact.yml. The zero value means defaults.
type Config struct {
	Critical        []string // package globs, cleaned (path.Clean, no leading "./")
	SensitiveAdd    []string // patterns added to the default sensitive table
	SensitiveRemove []string // patterns removed from the default sensitive table
	Architecture    Architecture
}

// LayerGlobs is one configured layer: its name and cleaned globs.
type LayerGlobs struct {
	Name  string
	Globs []string
}

// Architecture is the `architecture` block. Set is false when absent.
type Architecture struct {
	Set         bool
	Preset      string
	Layers      []LayerGlobs // file order
	Allow       []string     // "<a> -> <b>"
	Isolated    []string
	Stages      []string
	ModulesRoot string
}

// presetNames mirrors arch.PresetNames (config cannot import arch); a test in
// internal/arch keeps the two equal.
var presetNames = []string{"clean", "onion", "hexagonal", "ddd", "layered", "mvc", "pipeline", "modular"}

// PresetNames lists the accepted architecture presets.
func PresetNames() []string { return slices.Clone(presetNames) }

var (
	allowRE = regexp.MustCompile(`^\s*[A-Za-z0-9_-]+\s+->\s+[A-Za-z0-9_-]+\s*$`)
	nameRE  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

type archShape struct {
	Preset      string    `yaml:"preset"`
	Layers      yaml.Node `yaml:"layers"`
	Allow       []string  `yaml:"allow"`
	Isolated    []string  `yaml:"isolated"`
	Stages      []string  `yaml:"stages"`
	ModulesRoot string    `yaml:"modules_root"`
}

type fileShape struct {
	Critical  []string `yaml:"critical"`
	Sensitive struct {
		Add    []string `yaml:"add"`
		Remove []string `yaml:"remove"`
	} `yaml:"sensitive"`
	Architecture *archShape `yaml:"architecture"`
}

// Load reads explicitPath when non-empty (it must exist), else
// <moduleRoot>/.mrimpact.yml when it exists, else returns the zero Config.
// Parsing is strict: unknown keys and wrong types are errors. Only the YAML
// shape and the globs are validated here; pattern syntax and remove
// membership belong to sensitive.NewTable.
func Load(moduleRoot, explicitPath string) (Config, error) {
	p := explicitPath
	if p == "" {
		p = filepath.Join(moduleRoot, ".mrimpact.yml")
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
	}
	f, err := os.Open(p)
	if err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrConfig, err)
	}
	defer f.Close()
	return Parse(p, f)
}

// Parse parses a config file's content; name labels its errors.
func Parse(name string, r io.Reader) (Config, error) {
	var raw fileShape
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%w: %s: %v", ErrConfig, name, err)
	}
	for i, g := range raw.Critical {
		if err := checkGlob(g); err != nil {
			return Config{}, fmt.Errorf("%w: %s: critical %q: %v", ErrConfig, name, g, err)
		}
		raw.Critical[i] = cleanGlob(g)
	}
	arch, err := parseArchitecture(name, raw.Architecture)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Critical:        raw.Critical,
		SensitiveAdd:    raw.Sensitive.Add,
		SensitiveRemove: raw.Sensitive.Remove,
		Architecture:    arch,
	}, nil
}

func parseArchitecture(name string, a *archShape) (Architecture, error) {
	if a == nil {
		return Architecture{}, nil
	}
	bad := func(format string, args ...any) (Architecture, error) {
		return Architecture{}, fmt.Errorf("%w: %s: architecture: %s", ErrConfig, name, fmt.Sprintf(format, args...))
	}
	out := Architecture{
		Set: true, Preset: a.Preset, Allow: a.Allow, Isolated: a.Isolated,
		Stages: a.Stages, ModulesRoot: a.ModulesRoot,
	}
	if a.Preset != "" && !slices.Contains(presetNames, a.Preset) {
		return bad("unknown preset %q (want one of %s)", a.Preset, strings.Join(presetNames, ", "))
	}
	if n := &a.Layers; n.Kind != 0 {
		if n.Kind != yaml.MappingNode {
			return bad("layers must be a mapping of layer name to globs")
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			lname := k.Value
			if k.Kind != yaml.ScalarNode || !nameRE.MatchString(lname) {
				return bad("invalid layer name %q", lname)
			}
			for _, l := range out.Layers {
				if l.Name == lname {
					return bad("duplicate layer %q", lname)
				}
			}
			if v.Kind != yaml.SequenceNode {
				return bad("layer %q: globs must be a list of strings", lname)
			}
			lg := LayerGlobs{Name: lname}
			for _, e := range v.Content {
				if e.Kind != yaml.ScalarNode || e.Tag != "!!str" {
					return bad("layer %q: globs must be a list of strings", lname)
				}
				if err := checkGlob(e.Value); err != nil {
					return bad("layer %q glob %q: %v", lname, e.Value, err)
				}
				lg.Globs = append(lg.Globs, cleanGlob(e.Value))
			}
			if len(lg.Globs) == 0 {
				return bad("layer %q has no globs", lname)
			}
			out.Layers = append(out.Layers, lg)
		}
	}
	if a.Preset == "" && len(out.Layers) == 0 {
		return bad("needs a preset or layers")
	}
	for _, s := range a.Allow {
		if !allowRE.MatchString(s) {
			return bad("allow %q is not \"<layer> -> <layer>\"", s)
		}
	}
	if a.Preset == "pipeline" {
		if len(a.Stages) == 0 {
			return bad("preset pipeline requires stages")
		}
		seen := map[string]bool{}
		for _, s := range a.Stages {
			if !nameRE.MatchString(s) {
				return bad("invalid stage name %q", s)
			}
			if seen[s] {
				return bad("duplicate stage %q", s)
			}
			seen[s] = true
		}
	} else if len(a.Stages) > 0 {
		return bad("stages %v need preset pipeline", a.Stages)
	}
	if a.ModulesRoot != "" && a.Preset != "modular" {
		return bad("modules_root %q needs preset modular", a.ModulesRoot)
	}
	if a.ModulesRoot != "" {
		r := cleanGlob(a.ModulesRoot)
		r = strings.TrimSuffix(r, "/")
		if r == "." || path.IsAbs(r) || slices.Contains(strings.Split(r, "/"), "..") {
			return bad("modules_root %q must be a relative directory inside the module", a.ModulesRoot)
		}
		out.ModulesRoot = r
	}
	if a.Preset == "modular" && (len(out.Layers) > 0 || len(a.Allow) > 0 || len(a.Isolated) > 0) {
		return bad("preset modular takes no layers, allow or isolated")
	}
	return out, nil
}

// Change is one difference between two configs: What the MR does ("adds
// critical", "removes from defaults", …) to Items, in head (or base) order.
type Change struct {
	What  string
	Items []string
}

// Changes lists what head changes relative to base, per key: critical
// added/dropped, sensitive.remove added/dropped, sensitive.add added/dropped.
// Order within a key and duplicates do not count. nil when equal.
func Changes(base, head Config) []Change {
	var out []Change
	pair := func(b, h []string, added, dropped string) {
		if d := missing(h, b); len(d) > 0 {
			out = append(out, Change{added, d})
		}
		if d := missing(b, h); len(d) > 0 {
			out = append(out, Change{dropped, d})
		}
	}
	pair(base.Critical, head.Critical, "adds critical", "drops critical")
	pair(base.SensitiveRemove, head.SensitiveRemove, "removes from defaults", "restores defaults")
	pair(base.SensitiveAdd, head.SensitiveAdd, "adds", "drops added")
	out = append(out, archChanges(base.Architecture, head.Architecture)...)
	return out
}

// Name is the preset, or "custom" for a block without one.
func (a Architecture) Name() string {
	if a.Preset == "" {
		return "custom"
	}
	return a.Preset
}

// archChanges reports the architecture block going from base to head.
func archChanges(b, h Architecture) []Change {
	switch {
	case !b.Set && !h.Set:
		return nil
	case !b.Set:
		return []Change{{"adds architecture", []string{h.Name()}}}
	case !h.Set:
		return []Change{{"drops architecture", []string{b.Name()}}}
	}
	var keys []string
	if b.Preset != h.Preset {
		keys = append(keys, "preset")
	}
	if !slices.EqualFunc(b.Layers, h.Layers, func(x, y LayerGlobs) bool {
		return x.Name == y.Name && slices.Equal(x.Globs, y.Globs)
	}) {
		keys = append(keys, "layers")
	}
	if !slices.Equal(b.Allow, h.Allow) {
		keys = append(keys, "allow")
	}
	if !slices.Equal(b.Isolated, h.Isolated) {
		keys = append(keys, "isolated")
	}
	if !slices.Equal(b.Stages, h.Stages) {
		keys = append(keys, "stages")
	}
	if b.ModulesRoot != h.ModulesRoot {
		keys = append(keys, "modules_root")
	}
	if len(keys) == 0 {
		return nil
	}
	return []Change{{"changes architecture", keys}}
}

// missing returns the distinct entries of a not in b, in a's order.
func missing(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// cleanGlob is path.Clean without a leading "./": "./internal/x/" reads as
// "internal/x".
func cleanGlob(g string) string {
	return strings.TrimPrefix(path.Clean(g), "./")
}

// checkGlob validates every path segment with path.Match after stripping a
// trailing "/...".
func checkGlob(g string) error {
	g = strings.TrimSuffix(g, "/...")
	if g == "" {
		return errors.New("empty glob")
	}
	for _, seg := range strings.Split(g, "/") {
		if _, err := path.Match(seg, ""); err != nil {
			return err
		}
	}
	return nil
}
