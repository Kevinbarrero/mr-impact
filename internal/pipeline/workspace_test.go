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
