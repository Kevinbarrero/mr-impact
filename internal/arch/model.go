// Package arch models architecture rules (layers) and judges import edges
// against them.
package arch

import (
	"strings"

	"gitlab.com/wh1t3hat/mrimpact/internal/sensitive"
)

// Edge is a module-internal import between package paths; File is repo-relative.
type Edge struct {
	From, To, File string
	Line           int
}

// Graph is the package import graph of one revision.
type Graph struct {
	Module     string
	Packages   []string          // sorted package paths
	Edges      []Edge            // sorted by From, To
	Named      map[string]int    // named types declared per package
	Interfaces map[string]int    // interface types declared per package
	Names      map[string]string // package path -> package name
}

// Kind selects how a model classifies packages.
type Kind int

const (
	Layers Kind = iota
	Pipeline
	Modular
)

// Layer is one architecture layer.
type Layer struct {
	Name      string
	Dirs      []string // directory names
	Globs     []string // module-relative or full import path
	Isolated  bool
	MayImport []string // layer names
}

// Model is a set of layers plus the rules between them.
type Model struct {
	Name        string // preset name, "custom", or "inferred"
	Inferred    bool
	Kind        Kind
	Layers      []Layer // declaration order = glob precedence
	ModulesRoot string  // Modular
}

// EntryLayer is the implicit layer of executables.
const EntryLayer = "entry"

// Classify is ClassifyNamed without a package name.
func (m Model) Classify(module, pkgPath string) (layer string, ok bool) {
	return m.ClassifyNamed(module, pkgPath, "")
}

// ClassifyNamed returns the layer of a package: config globs (declaration
// order), then entry (package main or a cmd segment), then the layer owning
// the shallowest module-relative segment equal to one of its Dirs. In Modular
// models the layer is the module name under ModulesRoot.
func (m Model) ClassifyNamed(module, pkgPath, pkgName string) (string, bool) {
	for _, l := range m.Layers {
		if _, ok := sensitive.Critical(pkgPath, module, l.Globs); ok {
			return l.Name, true
		}
	}
	segs := segments(module, pkgPath)
	if pkgName == "main" {
		return EntryLayer, true
	}
	for _, s := range segs {
		if s == "cmd" {
			return EntryLayer, true
		}
	}
	if m.Kind == Modular {
		rest, ok := strings.CutPrefix(strings.Join(segs, "/"), m.ModulesRoot+"/")
		if !ok || rest == "" {
			return "", false
		}
		return strings.SplitN(rest, "/", 2)[0], true
	}
	for _, s := range segs {
		for _, l := range m.Layers {
			for _, d := range l.Dirs {
				if d == s {
					return l.Name, true
				}
			}
		}
	}
	return "", false
}

func segments(module, pkgPath string) []string {
	rest, ok := strings.CutPrefix(pkgPath, module+"/")
	if !ok || rest == "" {
		return nil
	}
	return strings.Split(rest, "/")
}
