// Package diff parses `git diff -M --unified=0 --no-color` output into
// per-file change records with line numbers.
package diff

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type Status string

const (
	Added    Status = "added"
	Modified Status = "modified"
	Deleted  Status = "deleted"
	Renamed  Status = "renamed"
)

type FileChange struct {
	Path         string // HEAD path ("" if Deleted)
	OldPath      string // BASE path ("" if Added)
	Status       Status
	AddedLines   []int // HEAD line numbers, ascending
	DeletedLines []int // BASE line numbers, ascending
	IsTest       bool  // Path (or OldPath if deleted) ends in _test.go
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// Parse reads `git diff -M --unified=0 --no-color` output.
func Parse(r io.Reader) ([]FileChange, error) {
	var (
		files   []FileChange
		cur     *FileChange
		oldLeft int // body lines still expected in current hunk
		newLeft int
		oldPath string
		newPath string
		isNew   bool
		isDel   bool
		isRen   bool
	)

	finish := func() {
		if cur == nil {
			return
		}
		switch {
		case isNew:
			cur.Status, cur.OldPath, cur.Path = Added, "", newPath
		case isDel:
			cur.Status, cur.OldPath, cur.Path = Deleted, oldPath, ""
		case isRen:
			cur.Status, cur.OldPath, cur.Path = Renamed, oldPath, newPath
		default:
			cur.Status, cur.OldPath, cur.Path = Modified, oldPath, newPath
		}
		name := cur.Path
		if name == "" {
			name = cur.OldPath
		}
		cur.IsTest = strings.HasSuffix(name, "_test.go")
		files = append(files, *cur)
		cur = nil
	}

	br := bufio.NewReader(r)
	var oldLine, newLine int
	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		atEOF := err == io.EOF
		line = strings.TrimRight(line, "\r\n")
		if line == "" && atEOF {
			break
		}

		inHunk := oldLeft > 0 || newLeft > 0
		switch {
		case inHunk && strings.HasPrefix(line, "-"):
			cur.DeletedLines = append(cur.DeletedLines, oldLine)
			oldLine++
			oldLeft--
		case inHunk && strings.HasPrefix(line, "+"):
			cur.AddedLines = append(cur.AddedLines, newLine)
			newLine++
			newLeft--
		case inHunk && strings.HasPrefix(line, `\`):
			// "\ No newline at end of file"
		case strings.HasPrefix(line, "diff --git "):
			finish()
			cur = &FileChange{}
			oldLeft, newLeft = 0, 0
			isNew, isDel, isRen = false, false, false
			oldPath, newPath = splitGitHeader(strings.TrimPrefix(line, "diff --git "))
		case cur == nil:
			// preamble before the first file
		case strings.HasPrefix(line, "new file mode"):
			isNew = true
		case strings.HasPrefix(line, "deleted file mode"):
			isDel = true
		case strings.HasPrefix(line, "rename from "):
			isRen = true
			oldPath = unquote(strings.TrimPrefix(line, "rename from "))
		case strings.HasPrefix(line, "rename to "):
			isRen = true
			newPath = unquote(strings.TrimPrefix(line, "rename to "))
		case strings.HasPrefix(line, "--- "):
			if p := unquote(strings.TrimPrefix(line, "--- ")); p == "/dev/null" {
				isNew = true
			} else {
				oldPath = strings.TrimPrefix(p, "a/")
			}
		case strings.HasPrefix(line, "+++ "):
			if p := unquote(strings.TrimPrefix(line, "+++ ")); p == "/dev/null" {
				isDel = true
			} else {
				newPath = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "@@ "):
			m := hunkRe.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("diff: malformed hunk header %q", line)
			}
			oldLine, _ = strconv.Atoi(m[1])
			oldLeft = count(m[2])
			newLine, _ = strconv.Atoi(m[3])
			newLeft = count(m[4])
		}
		if atEOF {
			break
		}
	}
	finish()
	return files, nil
}

// count converts an omitted hunk count to 1; "0" stays 0.
func count(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

// splitGitHeader extracts paths from `a/X b/Y` (used only as a fallback for
// diffs with no ---/+++ or rename lines, e.g. mode changes).
func splitGitHeader(s string) (string, string) {
	if i := strings.Index(s, " b/"); i >= 0 {
		return strings.TrimPrefix(unquote(s[:i]), "a/"), unquote(s[i+3:])
	}
	return "", ""
}

// unquote handles git's C-style quoting of unusual paths.
func unquote(s string) string {
	if strings.HasPrefix(s, `"`) {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
	}
	return s
}
