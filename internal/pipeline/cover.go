package pipeline

import (
	"fmt"
	"strings"
)

// crossPkgTests returns the test packages other than fnPkg. An external test
// package (<pkg>_test) is the same package for this comparison; the full
// paths are kept.
func crossPkgTests(fnPkg string, testPkgs []string) []string {
	var out []string
	for _, p := range testPkgs {
		if strings.TrimSuffix(p, "_test") != fnPkg {
			out = append(out, p)
		}
	}
	return out
}

// coverText is the "Added-line coverage" cell. crossPkgs are the packages of
// tests (other than the function's own) that reach it; they only matter when
// the profile shows added lines and none covered.
func coverText(prof bool, covered, total, added int, graph bool, testReached bool, crossPkgs []string, mod string) string {
	switch {
	case prof:
		switch {
		case total > 0 && covered == 0 && len(crossPkgs) > 0:
			return fmt.Sprintf("0 %% in profile · reached by tests in %s", pkgList(crossPkgs, mod))
		case total > 0:
			return fmt.Sprintf("%d %% (%d/%d)", covered*100/total, covered, total)
		case !graph:
			return "no added lines"
		case testReached:
			return "no added lines · reached by test"
		default:
			return "no added lines · not reached"
		}
	case !graph:
		return "n/a"
	case testReached && added > 0:
		return "reached by test · added lines not measured"
	case testReached:
		return "no added lines · reached by test"
	default:
		return "not reached"
	}
}

// pkgList names at most two packages relative to the module path, then +N.
// A package and its external test package (<pkg>_test) are one name.
func pkgList(pkgs []string, mod string) string {
	var names []string
	seen := map[string]bool{}
	for _, p := range pkgs {
		p = strings.TrimSuffix(p, "_test")
		p = strings.TrimPrefix(strings.TrimPrefix(p, mod), "/")
		if p == "" {
			p = "."
		}
		if !seen[p] {
			seen[p] = true
			names = append(names, p)
		}
	}
	s := strings.Join(names[:min(2, len(names))], ", ")
	if len(names) > 2 {
		s += fmt.Sprintf(" +%d", len(names)-2)
	}
	return s
}
