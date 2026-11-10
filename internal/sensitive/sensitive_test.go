package sensitive

import (
	"errors"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/config"
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
	noReflect, err := NewTable(nil, []string{"reflect.(Value).Set*"})
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
func F(v reflect.Value) { v.SetInt(1) }`, nil},
		{"reflect-read-only", defaults(t), `package p
import "reflect"
func F(x, y any) bool {
	v := reflect.ValueOf(x)
	_ = v.Kind(); _ = v.Interface(); _ = reflect.TypeOf(x).Kind()
	return reflect.DeepEqual(x, y)
}`, nil},
		{"reflect-set", defaults(t), `package p
import "reflect"
func F(v reflect.Value) { v.Set(reflect.ValueOf(1)); v.SetInt(2) }`,
			[]string{"reflect.(Value).Set", "reflect.(Value).SetInt"}},
		{"reflect-dynamic", defaults(t), `package p
import "reflect"
func F(v reflect.Value, t reflect.Type) {
	v.MethodByName("X").Call(nil)
	t.MethodByName("X")
	_ = reflect.NewAt(t, nil)
}`, []string{"reflect.(Value).MethodByName", "reflect.(Value).Call", "reflect.(Type).MethodByName", "reflect.NewAt"}},
		{"servemux-not-default", defaults(t), `package p
import "net/http"
func F(m *http.ServeMux, h http.Handler) { m.Handle("/", h) }`, nil},
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

// stubImporter type-checks the library stubs under testdata/stubs (by import
// path) and falls back to the standard library: no network, no real
// dependency.
type stubImporter map[string]*types.Package

func (s stubImporter) Import(p string) (*types.Package, error) {
	if pkg, ok := s[p]; ok {
		return pkg, nil
	}
	dir := filepath.Join("testdata", "stubs", filepath.FromSlash(p))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return stdImp.Import(p)
	}
	var files []*ast.File
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	pkg, err := (&types.Config{Importer: s}).Check(p, fset, files, nil)
	if err != nil {
		return nil, err
	}
	s[p] = pkg
	return pkg, nil
}

// The data-access and crypto libraries of the default table, matched against
// stubs with their real type names (pgx Conn a struct, Tx an interface;
// pgxpool Pool, Conn and Tx structs).
func TestCalls_Libraries(t *testing.T) {
	imp := stubImporter{}
	cases := []struct {
		name, src string
		want      []string
	}{
		{"pgx-v5", `package p
import ("context"; "github.com/jackc/pgx/v5")
func F(ctx context.Context, c *pgx.Conn) error {
	tx, _ := c.Begin(ctx)
	tx.Exec(ctx, "q")
	return tx.Commit(ctx)
}`, []string{"github.com/jackc/pgx/v5.(*Conn).Begin", "github.com/jackc/pgx/v5.(Tx).Exec", "github.com/jackc/pgx/v5.(Tx).Commit"}},
		{"pgx-connect-not-matched", `package p
import ("context"; "github.com/jackc/pgx/v5")
func F(ctx context.Context) { pgx.Connect(ctx, "") }`, nil},
		{"pgxpool-v5", `package p
import ("context"; "github.com/jackc/pgx/v5/pgxpool")
func F(ctx context.Context, p *pgxpool.Pool, t *pgxpool.Tx) {
	c, _ := p.Acquire(ctx)
	c.Exec(ctx, "q")
	t.Commit(ctx)
}`, []string{"github.com/jackc/pgx/v5/pgxpool.(*Pool).Acquire", "github.com/jackc/pgx/v5/pgxpool.(*Conn).Exec", "github.com/jackc/pgx/v5/pgxpool.(*Tx).Commit"}},
		{"pgxpool-begin-tx", `package p
import ("context"; "github.com/jackc/pgx/v5/pgxpool")
func F(ctx context.Context, p *pgxpool.Pool) { tx, _ := p.Begin(ctx); tx.Rollback(ctx) }`,
			[]string{"github.com/jackc/pgx/v5/pgxpool.(*Pool).Begin", "github.com/jackc/pgx/v5.(Tx).Rollback"}},
		{"pgx-v4", `package p
import ("context"; "github.com/jackc/pgx/v4"; "github.com/jackc/pgx/v4/pgxpool")
func F(ctx context.Context, c *pgx.Conn, p *pgxpool.Pool, pc *pgxpool.Conn, pt *pgxpool.Tx, tx pgx.Tx) {
	c.Exec(ctx, "q"); p.Exec(ctx, "q"); pc.Exec(ctx, "q"); pt.Exec(ctx, "q"); tx.Exec(ctx, "q")
}`, []string{"github.com/jackc/pgx/v4.(*Conn).Exec", "github.com/jackc/pgx/v4/pgxpool.(*Pool).Exec",
			"github.com/jackc/pgx/v4/pgxpool.(*Conn).Exec", "github.com/jackc/pgx/v4/pgxpool.(*Tx).Exec", "github.com/jackc/pgx/v4.(Tx).Exec"}},
		{"sqlx", `package p
import "github.com/jmoiron/sqlx"
func F(db *sqlx.DB) error {
	tx, _ := db.Beginx()
	tx.Select(nil, "q")
	return tx.Commit()
}`, []string{"github.com/jmoiron/sqlx.(*DB).Beginx", "github.com/jmoiron/sqlx.(*Tx).Select", "database/sql.(*Tx).Commit"}},
		{"gorm", `package p
import "gorm.io/gorm"
func F(db *gorm.DB) error { return db.Where("x").Create(1).Error }`,
			[]string{"gorm.io/gorm.(*DB).Where", "gorm.io/gorm.(*DB).Create"}},
		{"x-crypto", `package p
import "golang.org/x/crypto/bcrypt"
func F(p []byte) { bcrypt.GenerateFromPassword(p, 10) }`, []string{"golang.org/x/crypto/bcrypt.GenerateFromPassword"}},
	}
	tab := defaults(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, _, info := check(t, "example.com/p", c.src, imp)
			var fn *ast.FuncDecl
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "F" {
					fn = fd
				}
			}
			if got := tab.Calls(fn, info); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Calls = %#v, want %#v", got, c.want)
			}
		})
	}
}

func TestSites_LineSpans(t *testing.T) {
	fd, info := funcF(t, `package p

import (
	"os"
	"os/exec"
)

func F(p string) error {
	os.Remove(p)
	return exec.Command(p).Run()
}`)
	got := defaults(t).Sites(fd, info, fset)
	want := []Site{
		{API: "os.Remove", From: 9, To: 9},
		{API: "os/exec.Command", From: 10, To: 10},
		{API: "os/exec.(*Cmd).Run", From: 10, To: 10},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Sites = %v, want %v", got, want)
	}
}

func TestSites_MultiLineCallSpan(t *testing.T) {
	fd, info := funcF(t, `package p

import (
	"context"
	"database/sql"
)

func F(ctx context.Context, db *sql.DB) {
	db.ExecContext(
		ctx,
		"INSERT",
	)
}`)
	got := defaults(t).Sites(fd, info, fset)
	want := []Site{{API: "database/sql.(*DB).ExecContext", From: 9, To: 12}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Sites = %v, want %v", got, want)
	}
}

func TestCalls_UnchangedOnSites(t *testing.T) {
	fd, info := funcF(t, `package p

import "os"

func F(p string) {
	os.Remove(p)
	os.Remove(p)
}`)
	tab := defaults(t)
	if n := len(tab.Sites(fd, info, fset)); n != 2 {
		t.Errorf("Sites = %d, want 2", n)
	}
	if got := tab.Calls(fd, info); !reflect.DeepEqual(got, []string{"os.Remove"}) {
		t.Errorf("Calls = %v", got)
	}
}
