package pipeline

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/Kevinbarrero/mr-impact/internal/report"
)

// workspaceNote describes the go.work the head is loaded with: nil when
// `go env GOWORK` names none (empty or "off"), an Unreadable note when the
// go.work or the analysed go.mod does not parse.
func workspaceNote(ctx context.Context, dir string) *report.WorkspaceNote {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	// real is --dir with symlinks resolved; every path the line shows is
	// relative to it, so a --dir reached through a symlink reads the same.
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		real = abs
	}
	cmd := exec.CommandContext(ctx, "go", "env", "GOWORK")
	cmd.Dir = abs
	// With Env set, os/exec does not set PWD: go would see the resolved
	// directory, not the path it was given.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=", "PWD="+abs)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}
	gw := strings.TrimSpace(out.String())
	if gw == "" || gw == "off" {
		return nil
	}
	note := &report.WorkspaceNote{Path: relPath(real, resolve(gw))}
	unreadable := func() *report.WorkspaceNote {
		note.Unreadable = true
		note.Modules = nil
		return note
	}

	data, err := os.ReadFile(gw)
	if err != nil {
		return unreadable()
	}
	work, err := modfile.ParseWork(gw, data, nil)
	if err != nil {
		return unreadable()
	}
	modData, err := os.ReadFile(filepath.Join(abs, "go.mod"))
	if err != nil {
		return unreadable()
	}
	mf, err := modfile.Parse("go.mod", modData, nil)
	if err != nil {
		return unreadable()
	}
	required := map[string]string{}
	for _, r := range mf.Require {
		required[r.Mod.Path] = r.Mod.Version
	}
	// A replace of the required version wins over a path-wide one, as in go.
	replaced := map[string]string{}
	for _, r := range mf.Replace {
		if r.Old.Version == "" {
			if _, set := replaced[r.Old.Path]; !set {
				replaced[r.Old.Path] = r.New.Path
			}
		}
	}
	for _, r := range mf.Replace {
		if v := required[r.Old.Path]; r.Old.Version != "" && r.Old.Version == v {
			replaced[r.Old.Path] = r.New.Path
		}
	}
	// The analysed module is recognised by its module path, which holds
	// whatever spelling of its directory go.work and --dir use.
	self := modfile.ModulePath(modData)

	for _, u := range work.Use {
		d := filepath.FromSlash(u.Path)
		if !filepath.IsAbs(d) {
			d = filepath.Join(filepath.Dir(gw), d)
		}
		d = resolve(filepath.Clean(d))
		if d == real {
			continue
		}
		b, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err != nil {
			return unreadable()
		}
		path := modfile.ModulePath(b)
		if path == "" {
			return unreadable()
		}
		if path == self {
			continue
		}
		rel, err := filepath.Rel(real, d)
		if err != nil {
			rel = d
		}
		note.Modules = append(note.Modules, report.WorkspaceModule{
			Path: path, Dir: filepath.ToSlash(rel), Required: required[path], Replace: replaced[path],
		})
	}
	sort.Slice(note.Modules, func(i, j int) bool { return note.Modules[i].Path < note.Modules[j].Path })
	return note
}

// resolve is p with symlinks resolved, or p when that fails.
func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// relPath is p relative to dir when that stays below or beside it (no ".."
// beyond the parent), else p as given.
func relPath(dir, p string) string {
	r, err := filepath.Rel(dir, p)
	if err != nil {
		return p
	}
	r = filepath.ToSlash(r)
	if r == "../.." || strings.HasPrefix(r, "../../") {
		return p
	}
	return r
}
