// Package testutil holds helpers shared by tests across packages.
package testutil

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// MakeRepo copies baseDir into a temp dir, commits "base", replaces the tree
// with headDir (everything but .git is removed first), and commits "head".
// It returns the repo dir and the two commit SHAs.
func MakeRepo(t testing.TB, baseDir, headDir string) (repo, baseSHA, headSHA string) {
	t.Helper()
	repo = t.TempDir()
	git(t, repo, "init", "-q")
	copyTree(t, baseDir, repo)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	baseSHA = strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	entries, err := os.ReadDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(repo, e.Name())); err != nil {
			t.Fatal(err)
		}
	}
	copyTree(t, headDir, repo)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "head")
	headSHA = strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	return repo, baseSHA, headSHA
}

func git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func copyTree(t testing.TB, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
