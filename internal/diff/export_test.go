package diff

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/wh1t3hat/mrimpact/internal/testutil"
)

func TestExportWritesBaseTree(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	write := func(dir, p, s string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(base, "go.mod", "module ex.com/m\n")
	write(base, "a/a.go", "package a\n// base\n")
	write(head, "go.mod", "module ex.com/m\n")
	write(head, "b/b.go", "package b\n")
	repo, baseSHA, _ := testutil.MakeRepo(t, base, head)

	dst := t.TempDir()
	if err := Export(context.Background(), repo, baseSHA, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "a", "a.go"))
	if err != nil || string(got) != "package a\n// base\n" {
		t.Fatalf("a/a.go = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "b")); !os.IsNotExist(err) {
		t.Errorf("head-only b/ exported: %v", err)
	}
}
