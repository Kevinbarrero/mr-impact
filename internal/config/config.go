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
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrConfig wraps every error Load returns, so the CLI can map it to exit 2.
var ErrConfig = errors.New("config")

// Config is the parsed .mrimpact.yml. The zero value means defaults.
type Config struct {
	Critical        []string // package globs as written
	SensitiveAdd    []string // patterns added to the default sensitive table
	SensitiveRemove []string // patterns removed from the default sensitive table
}

type fileShape struct {
	Critical  []string `yaml:"critical"`
	Sensitive struct {
		Add    []string `yaml:"add"`
		Remove []string `yaml:"remove"`
	} `yaml:"sensitive"`
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

	var raw fileShape
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%w: %s: %v", ErrConfig, p, err)
	}
	for _, g := range raw.Critical {
		if err := checkGlob(g); err != nil {
			return Config{}, fmt.Errorf("%w: %s: critical %q: %v", ErrConfig, p, g, err)
		}
	}
	return Config{
		Critical:        raw.Critical,
		SensitiveAdd:    raw.Sensitive.Add,
		SensitiveRemove: raw.Sensitive.Remove,
	}, nil
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
