package diff

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Export writes the tree of rev in repoDir into dst (an existing, empty
// directory) with `git archive`, leaving the repository and its worktree
// untouched. Symlinks and submodules are skipped.
func Export(ctx context.Context, repoDir, rev, dst string) error {
	// Archive the subtree repoDir is in (the module may not be the repo root).
	prefix, err := runGit(ctx, repoDir, "rev-parse", "--show-prefix")
	if err != nil {
		return err
	}
	tree := rev
	if p := strings.TrimSpace(string(prefix)); p != "" {
		tree = rev + ":" + strings.TrimSuffix(p, "/")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "archive", "--format=tar", tree)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("git archive %s: %w", rev, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git archive %s: %w", rev, err)
	}
	extractErr := extract(tar.NewReader(out), dst)
	if extractErr != nil {
		_, _ = io.Copy(io.Discard, out) // let git finish writing before Wait
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive %s: %w: %s", rev, err, strings.TrimSpace(stderr.String()))
	}
	return extractErr
}

func extract(tr *tar.Reader, dst string) error {
	root, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("export: %w", err)
		}
		target := filepath.Join(root, filepath.FromSlash(h.Name))
		if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
			return fmt.Errorf("export: entry %q escapes the destination", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("export: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("export: %w", err)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return fmt.Errorf("export: %w", err)
			}
			_, err = io.Copy(f, tr)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return fmt.Errorf("export: %w", err)
			}
		}
	}
}
