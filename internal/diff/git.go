package diff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// ErrShallow is returned when the base commit is not available locally and
// cannot be fetched.
var ErrShallow = errors.New(`base commit not in clone: fetch full history (GitLab: GIT_DEPTH: 0; GitHub: actions/checkout with fetch-depth: 0)`)

var generatedRe = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// IsGenerated reports whether src has a "Code generated ... DO NOT EDIT."
// comment line before its package clause.
func IsGenerated(src []byte) bool {
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "package ") || line == "package" {
			return false
		}
		if generatedRe.MatchString(line) {
			return true
		}
	}
	return false
}

func runGit(ctx context.Context, repoDir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoDir}, args...)...)
	// Untranslated messages: ShowFile matches git's English error text.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Run diffs base..head in repoDir and returns the changed Go files, excluding
// vendor/ paths and generated files. Generated-ness is checked on HEAD content
// (BASE content for deleted files).
func Run(ctx context.Context, repoDir, base, head string) ([]FileChange, error) {
	out, err := runGit(ctx, repoDir, "-c", "core.quotepath=false", "diff", "-M", "--unified=0",
		"--no-color", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/", base, head)
	if err != nil {
		return nil, err
	}
	all, err := Parse(bytes.NewReader(out))
	if err != nil {
		return nil, err
	}
	var kept []FileChange
	for _, fc := range all {
		name, rev := fc.Path, head
		if fc.Status == Deleted {
			name, rev = fc.OldPath, base
		}
		if !strings.HasSuffix(name, ".go") || hasVendorSegment(name) {
			continue
		}
		src, err := ShowFile(ctx, repoDir, rev, name)
		if err != nil {
			return nil, err
		}
		if IsGenerated(src) {
			continue
		}
		kept = append(kept, fc)
	}
	return kept, nil
}

func hasVendorSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "vendor" {
			return true
		}
	}
	return false
}

// ShowFile returns the content of path at rev. A missing path yields an error
// wrapping fs.ErrNotExist.
func ShowFile(ctx context.Context, repoDir, rev, path string) ([]byte, error) {
	out, err := runGit(ctx, repoDir, "show", rev+":"+path)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "does not exist") || strings.Contains(msg, "exists on disk, but not in") {
			return nil, fmt.Errorf("%s:%s: %w", rev, path, fs.ErrNotExist)
		}
		return nil, err
	}
	return out, nil
}

// EnsureCommit verifies sha is present, fetching it from origin if not. A
// failed fetch returns an error wrapping ErrShallow that also carries git's
// message (for the job log only; it may contain credentialed URLs).
func EnsureCommit(ctx context.Context, repoDir, sha string) error {
	if _, err := runGit(ctx, repoDir, "cat-file", "-e", sha+"^{commit}"); err == nil {
		return nil
	}
	if _, err := runGit(ctx, repoDir, "fetch", "origin", sha); err != nil {
		return fmt.Errorf("%w: %v", ErrShallow, err)
	}
	return nil
}

// ErrNoMergeBase is returned by MergeBase when the two revisions share no
// ancestor in the local history (unrelated, or cut off by a shallow clone).
var ErrNoMergeBase = errors.New("no merge-base")

// MergeBase returns the SHA of the best common ancestor of a and b.
func MergeBase(ctx context.Context, repoDir, a, b string) (string, error) {
	out, err := runGit(ctx, repoDir, "merge-base", a, b)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 && ctx.Err() == nil {
			return "", ErrNoMergeBase
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
