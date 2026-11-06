// Package coverage loads Go coverprofiles, merges them and answers which
// lines of a file are covered.
package coverage

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/cover"
)

// FileInfo reports the line count of a repo-relative HEAD file; ok=false if it doesn't exist.
type FileInfo func(path string) (lines int, ok bool)

type span struct{ start, end int }

// Profile holds merged coverage blocks per repo-relative file path.
type Profile struct {
	files map[string]*fileBlocks
}

type fileBlocks struct {
	blocks map[[4]int]*block // keyed by start line/col, end line/col
}

type block struct {
	span    span
	covered bool
}

// Load expands each pattern with filepath.Glob, parses the matches (each file
// once) and merges their blocks; a block is covered if any profile has Count>0.
// A stale profile (file outside the module, missing per info, or a block
// ending past EOF) yields (nil, warning, nil). A pattern matching nothing is
// reported in the warning but does not stop the others from loading.
func Load(patterns []string, modulePath string, info FileInfo) (*Profile, string, error) {
	seen := map[string]bool{}
	var matches, unmatched []string
	for _, pat := range patterns {
		m, err := filepath.Glob(pat)
		if err != nil {
			return nil, "", fmt.Errorf("coverage pattern %q: %w", pat, err)
		}
		if len(m) == 0 {
			unmatched = append(unmatched, pat)
		}
		for _, f := range m {
			if !seen[f] {
				seen[f] = true
				matches = append(matches, f)
			}
		}
	}
	if len(matches) == 0 {
		return nil, "no coverprofile matched " + strings.Join(quoteAll(unmatched), ", "), nil
	}
	sort.Strings(matches)

	prefix := modulePath + "/"
	p := &Profile{files: map[string]*fileBlocks{}}
	for _, f := range matches {
		profs, err := cover.ParseProfiles(f)
		if err != nil {
			return nil, "", fmt.Errorf("coverprofile %s: %w", f, err)
		}
		for _, prof := range profs {
			rel, ok := strings.CutPrefix(prof.FileName, prefix)
			if !ok {
				return nil, fmt.Sprintf("coverprofile %s is stale: %s is outside module %s", f, prof.FileName, modulePath), nil
			}
			lines, ok := info(rel)
			if !ok {
				return nil, fmt.Sprintf("coverprofile %s is stale: %s does not exist at HEAD", f, rel), nil
			}
			fb := p.files[rel]
			if fb == nil {
				fb = &fileBlocks{blocks: map[[4]int]*block{}}
				p.files[rel] = fb
			}
			for _, b := range prof.Blocks {
				if b.EndLine > lines {
					return nil, fmt.Sprintf("coverprofile %s is stale: %s block ends at line %d but file has %d lines", f, rel, b.EndLine, lines), nil
				}
				k := [4]int{b.StartLine, b.StartCol, b.EndLine, b.EndCol}
				cur := fb.blocks[k]
				if cur == nil {
					cur = &block{span: span{b.StartLine, b.EndLine}}
					fb.blocks[k] = cur
				}
				if b.Count > 0 {
					cur.covered = true
				}
			}
		}
	}

	warning := ""
	if len(unmatched) > 0 {
		warning = "no coverprofile matched " + strings.Join(quoteAll(unmatched), ", ")
	}
	return p, warning, nil
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

// Lines counts the given lines that fall inside any block (total) and inside
// a block with Count>0 (covered). A nil Profile or unknown file gives (0, 0).
func (p *Profile) Lines(file string, lines []int) (covered, total int) {
	if p == nil {
		return 0, 0
	}
	fb := p.files[file]
	if fb == nil {
		return 0, 0
	}
	for _, l := range lines {
		in, cov := false, false
		for _, b := range fb.blocks {
			if l >= b.span.start && l <= b.span.end {
				in = true
				if b.covered {
					cov = true
					break
				}
			}
		}
		if in {
			total++
		}
		if cov {
			covered++
		}
	}
	return covered, total
}
