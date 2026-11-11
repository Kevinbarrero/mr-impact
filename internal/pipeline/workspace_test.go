package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/report"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkspaceNote(t *testing.T) {
	ctx := context.Background()

	t.Run("no go.work", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"app/go.mod": "module example.com/app\n\ngo 1.26.0\n"})
		t.Setenv("GOWORK", "")
		t.Setenv("GOFLAGS", "")
		if n := workspaceNote(ctx, filepath.Join(root, "app")); n != nil {
			t.Errorf("want nil, got %+v", n)
		}
	})

	t.Run("GOWORK=off", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"app/go.mod": "module example.com/app\n\ngo 1.26.0\n",
			"go.work":    "go 1.26.0\n\nuse ./app\n",
		})
		t.Setenv("GOWORK", "off")
		if n := workspaceNote(ctx, filepath.Join(root, "app")); n != nil {
			t.Errorf("want nil, got %+v", n)
		}
	})

	t.Run("workspace", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"app/go.mod":      "module example.com/app\n\ngo 1.26.0\n\nrequire example.com/patterns v0.1.5\n\nreplace example.com/lib => ./lib\n",
			"app/lib/go.mod":  "module example.com/lib\n\ngo 1.26.0\n",
			"patterns/go.mod": "module example.com/patterns\n\ngo 1.26.0\n",
			"app/go.work":     "go 1.26.0\n\nuse (\n\t.\n\t./lib/\n\t../patterns\n)\n",
		})
		t.Setenv("GOWORK", "")
		dir := filepath.Join(root, "app")
		n := workspaceNote(ctx, dir)
		if n == nil {
			t.Fatal("want a note")
		}
		if n.Unreadable || n.Path != "go.work" {
			t.Errorf("Path/Unreadable = %q/%v", n.Path, n.Unreadable)
		}
		want := []report.WorkspaceModule{
			{Path: "example.com/lib", Dir: "lib", Required: "", Replace: "./lib"},
			{Path: "example.com/patterns", Dir: "../patterns", Required: "v0.1.5", Replace: ""},
		}
		if !reflect.DeepEqual(n.Modules, want) {
			t.Errorf("modules = %+v\nwant %+v", n.Modules, want)
		}
	})

	t.Run("symlinked dir", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"app/go.mod":     "module example.com/app\n\ngo 1.26.0\n",
			"app/lib/go.mod": "module example.com/lib\n\ngo 1.26.0\n",
			"app/go.work":    "go 1.26.0\n\nuse (\n\t.\n\t./lib\n)\n",
		})
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(root, link); err != nil {
			t.Skip("symlinks unsupported:", err)
		}
		t.Setenv("GOWORK", "")
		n := workspaceNote(ctx, filepath.Join(link, "app"))
		if n == nil {
			t.Fatal("want a note")
		}
		if n.Unreadable || n.Path != "go.work" {
			t.Errorf("Path/Unreadable = %q/%v", n.Path, n.Unreadable)
		}
		want := []report.WorkspaceModule{{Path: "example.com/lib", Dir: "lib"}}
		if !reflect.DeepEqual(n.Modules, want) {
			t.Errorf("modules = %+v\nwant %+v", n.Modules, want)
		}
	})

	t.Run("use . only", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"app/go.mod":  "module example.com/app\n\ngo 1.26.0\n",
			"app/go.work": "go 1.26.0\n\nuse .\n",
		})
		t.Setenv("GOWORK", "")
		n := workspaceNote(ctx, filepath.Join(root, "app"))
		if n == nil || n.Unreadable || n.Path != "go.work" || len(n.Modules) != 0 {
			t.Errorf("want a readable note with no modules, got %+v", n)
		}
	})

	t.Run("version replace", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"app/go.mod":      "module example.com/app\n\ngo 1.26.0\n\nrequire example.com/patterns v0.1.5\n\nreplace example.com/patterns v0.1.5 => ../patterns\n\nreplace example.com/patterns v0.0.1 => ../old\n",
			"patterns/go.mod": "module example.com/patterns\n\ngo 1.26.0\n",
			"app/go.work":     "go 1.26.0\n\nuse (\n\t.\n\t../patterns\n)\n",
		})
		t.Setenv("GOWORK", "")
		n := workspaceNote(ctx, filepath.Join(root, "app"))
		want := []report.WorkspaceModule{{Path: "example.com/patterns", Dir: "../patterns", Required: "v0.1.5", Replace: "../patterns"}}
		if n == nil || !reflect.DeepEqual(n.Modules, want) {
			t.Errorf("got %+v\nwant modules %+v", n, want)
		}
	})

	t.Run("unparsable", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"app/go.mod":  "module example.com/app\n\ngo 1.26.0\n",
			"app/go.work": "go 1.26.0\n\nuse (\n",
		})
		t.Setenv("GOWORK", "")
		n := workspaceNote(ctx, filepath.Join(root, "app"))
		if n == nil || !n.Unreadable || n.Path == "" {
			t.Errorf("want unreadable note with Path, got %+v", n)
		}
	})
}

func TestRelPath(t *testing.T) {
	cases := []struct{ dir, p, want string }{
		{"/a/b", "/a/b/go.work", "go.work"},
		{"/a/b", "/a/go.work", "../go.work"},
		{"/a/b", "/a/..x/go.work", "../..x/go.work"}, // a directory named "..x" is beside, not beyond
		{"/a/b/c", "/a/go.work", "/a/go.work"},
		{"/a/b", "/go.work", "/go.work"},
	}
	for _, c := range cases {
		if got := relPath(c.dir, c.p); got != c.want {
			t.Errorf("relPath(%q, %q) = %q, want %q", c.dir, c.p, got, c.want)
		}
	}
}
