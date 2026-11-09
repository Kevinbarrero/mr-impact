package sensitive

import (
	"errors"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"testing"

	"gitlab.com/wh1t3hat/mrimpact/internal/config"
)

// The source importer caches every package it type-checks, so one instance
// keeps the stdlib work shared across cases.
var (
	fset   = token.NewFileSet()
	stdImp = importer.ForCompiler(fset, "source", nil)
)

// ledgerSrc is a non-stdlib package, for the add case.
const ledgerSrc = `package l

type Ledger struct{}

func (l *Ledger) Post(n int) {}
`

type mapImporter map[string]*types.Package

func (m mapImporter) Import(p string) (*types.Package, error) {
	if pkg, ok := m[p]; ok {
		return pkg, nil
	}
	return stdImp.Import(p)
}

func check(t *testing.T, path, src string, imp types.Importer) (*ast.File, *types.Package, *types.Info) {
	t.Helper()
	f, err := parser.ParseFile(fset, path+".go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Instances:  map[*ast.Ident]types.Instance{},
	}
	conf := types.Config{Importer: imp}
	pkg, err := conf.Check(path, fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	return f, pkg, info
}

// funcF type-checks src as package example.com/p and returns its func F.
func funcF(t *testing.T, src string) (*ast.FuncDecl, *types.Info) {
	t.Helper()
	_, lpkg, _ := check(t, "example.com/l", ledgerSrc, stdImp)
	f, _, info := check(t, "example.com/p", src, mapImporter{"example.com/l": lpkg})
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "F" {
			return fd, info
		}
	}
	t.Fatal("no func F")
	return nil, nil
}

func defaults(t *testing.T) Table {
	t.Helper()
	tab, err := NewTable(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

func TestCalls(t *testing.T) {
	addLedger, err := NewTable([]string{"example.com/l.(*Ledger).Post"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	noReflect, err := NewTable(nil, []string{"reflect.*"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		table Table
		src   string
		want  []string
	}{
		{"alias", defaults(t), `package p
import x "os/exec"
func F() { x.Command("ls") }`, []string{"os/exec.Command"}},
		{"sql-exec", defaults(t), `package p
import ("context"; "database/sql")
func F(ctx context.Context, db *sql.DB, q string) { db.ExecContext(ctx, q) }`,
			[]string{"database/sql.(*DB).ExecContext"}},
		{"sql-commit", defaults(t), `package p
import "database/sql"
func F(tx *sql.Tx) { tx.Commit() }`, []string{"database/sql.(*Tx).Commit"}},
		{"method-value", defaults(t), `package p
import ("context"; "database/sql")
func F(ctx context.Context, db *sql.DB, q string) { run := db.ExecContext; run(ctx, q) }`,
			[]string{"database/sql.(*DB).ExecContext"}},
		{"method-expr", defaults(t), `package p
import ("context"; "database/sql")
func F(ctx context.Context, db *sql.DB, q string) { (*sql.DB).ExecContext(db, ctx, q) }`,
			[]string{"database/sql.(*DB).ExecContext"}},
		{"closure", defaults(t), `package p
import "os"
func F(p string) { func() { os.Remove(p) }() }`, []string{"os.Remove"}},
		{"nested-closure", defaults(t), `package p
import "os"
func F(p string) { go func() { defer func() { os.Remove(p) }() }() }`, []string{"os.Remove"}},
		{"tls-literal", defaults(t), `package p
import "crypto/tls"
func F() { _ = &tls.Config{} }`, []string{"crypto/tls.Config"}},
		{"crypto-tree", defaults(t), `package p
import "crypto/sha256"
func F(b []byte) { sha256.Sum256(b) }`, []string{"crypto/sha256.Sum256"}},
		{"template", defaults(t), `package p
import ("html/template"; "io")
func F(t *template.Template, w io.Writer, d any) { t.ExecuteTemplate(w, "x", d) }`,
			[]string{"html/template.(*Template).ExecuteTemplate"}},
		{"template-other-method", defaults(t), `package p
import "html/template"
func F(t *template.Template) { t.Name() }`, nil},
		{"plain", defaults(t), `package p
import ("fmt"; "strings")
func F(a []string) { fmt.Println(strings.Join(a, ",")) }`, nil},
		{"foreign-interface", defaults(t), `package p
type execer interface{ Exec(string) }
func F(e execer) { e.Exec("x") }`, nil},
		{"sensitive-interface", mustTable(t, []string{"database/sql/driver.(Execer).*"}), `package p
import "database/sql/driver"
func F(e driver.Execer) { e.Exec("q", nil) }`, []string{"database/sql/driver.(Execer).Exec"}},
		{"pointer-pattern-not-interface", mustTable(t, []string{"database/sql/driver.(*Execer).*"}), `package p
import "database/sql/driver"
func F(e driver.Execer) { e.Exec("q", nil) }`, nil},
		{"promoted-method", defaults(t), `package p
import "database/sql"
type store struct{ *sql.DB }
func F(s store) { s.Ping() }`, []string{"database/sql.(*DB).Ping"}},
		{"signal-const-not-matched", defaults(t), `package p
import ("os"; "os/signal"; "syscall")
func F(c chan os.Signal) { signal.Notify(c, syscall.SIGTERM) }`, nil},
		{"unsafe-conversion", defaults(t), `package p
import "unsafe"
func F(x *int) uintptr { return uintptr(unsafe.Pointer(x)) + unsafe.Sizeof(*x) }`,
			[]string{"unsafe.Pointer", "unsafe.Sizeof"}},
		{"type-reference-only", defaults(t), `package p
import "os/exec"
func F(c *exec.Cmd) *exec.Cmd { return c }`, nil},
		{"generic-origin", mustTable(t, []string{"example.com/p.Box"}), `package p
type Box[T any] struct{ v T }
func F() { _ = Box[int]{} }`, []string{"example.com/p.Box"}},
		{"add", addLedger, `package p
import "example.com/l"
func F(x *l.Ledger) { x.Post(1) }`, []string{"example.com/l.(*Ledger).Post"}},
		{"add-not-default", defaults(t), `package p
import "example.com/l"
func F(x *l.Ledger) { x.Post(1) }`, nil},
		{"remove", noReflect, `package p
import "reflect"
func F(x any) { reflect.TypeOf(x) }`, nil},
		{"reflect-default", defaults(t), `package p
import "reflect"
func F(x any) { reflect.TypeOf(x) }`, []string{"reflect.TypeOf"}},
		{"order", defaults(t), `package p
import ("os"; "os/exec")
func F(p string) { os.Remove(p); exec.Command("ls"); os.Remove(p) }`,
			[]string{"os.Remove", "os/exec.Command"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fn, info := funcF(t, c.src)
			got := c.table.Calls(fn, info)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Calls = %#v, want %#v", got, c.want)
			}
		})
	}
}

func mustTable(t *testing.T, add []string) Table {
	t.Helper()
	tab, err := NewTable(add, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

func TestNewTable_Errors(t *testing.T) {
	cases := []struct {
		name        string
		add, remove []string
	}{
		{"malformed", []string{"os.(*File"}, nil},
		{"no name", []string{"os."}, nil},
		{"no path", []string{".Remove"}, nil},
		{"bad method", []string{"os.(*File).1x"}, nil},
		{"remove not default", nil, []string{"fmt.Println"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewTable(c.add, c.remove)
			if err == nil {
				t.Fatal("want error")
			}
			if !errors.Is(err, config.ErrConfig) {
				t.Errorf("error %v does not wrap config.ErrConfig", err)
			}
		})
	}
}

func TestNewTable_DefaultsParse(t *testing.T) {
	for _, p := range defaultPatterns {
		if _, err := parsePattern(p); err != nil {
			t.Errorf("default %q: %v", p, err)
		}
	}
}

func TestCritical(t *testing.T) {
	cases := []struct {
		pkg, module string
		globs       []string
		glob        string
		ok          bool
	}{
		{"m/internal/pay/x", "m", []string{"internal/pay/..."}, "internal/pay/...", true},
		{"m/internal/pay", "m", []string{"internal/pay/..."}, "internal/pay/...", true},
		{"m/internal/payx", "m", []string{"internal/pay/..."}, "", false},
		{"m/a/b", "m", []string{"a/*"}, "a/*", true},
		{"m/a/b/c", "m", []string{"a/*"}, "", false},
		{"m/a/b", "m", []string{"m/a/b"}, "m/a/b", true},
		{"m/a/b", "m", []string{"x/...", "m/a/...", "a/b"}, "m/a/...", true},
		{"m/a/b", "m", nil, "", false},
	}
	for _, c := range cases {
		glob, ok := Critical(c.pkg, c.module, c.globs)
		if glob != c.glob || ok != c.ok {
			t.Errorf("Critical(%q, %q, %q) = %q, %v; want %q, %v", c.pkg, c.module, c.globs, glob, ok, c.glob, c.ok)
		}
	}
}
