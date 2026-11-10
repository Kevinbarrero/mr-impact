// Package sensitive finds the sensitive APIs a function touches: the default
// table (standard-library patterns and a few common data-access and crypto
// libraries), plus the config's additions and removals, matched against
// type-resolved callees.
package sensitive

import (
	"fmt"
	"go/token"
	"slices"
	"strings"

	"github.com/Kevinbarrero/mr-impact/internal/config"
)

// defaultPatterns is the default table (spec §3), in table order.
var defaultPatterns = []string{
	// Process / OS
	"os/exec.*", "syscall.*", "unsafe.*", "plugin.*", "os.(*Process).*", "os.StartProcess",
	// Files
	"os.Remove", "os.RemoveAll", "os.WriteFile", "os.Create", "os.OpenFile", "os.Chmod", "os.Chown",
	"os.Rename", "os.Symlink", "os.Link",
	// Reflection: mutating, unsafe and dynamic-dispatch APIs only; read-only
	// inspection (TypeOf, ValueOf, DeepEqual, Kind, Interface) is not
	// sensitive.
	"reflect.(Value).Set*", "reflect.(Value).Send", "reflect.(Value).TrySend", "reflect.(Value).Clear",
	"reflect.Copy", "reflect.Swapper",
	"reflect.NewAt", "reflect.(Value).UnsafePointer", "reflect.(Value).UnsafeAddr", "reflect.(Value).Pointer",
	"reflect.(Value).Call*", "reflect.(Value).Method", "reflect.(Value).MethodByName", "reflect.(Type).MethodByName",
	"reflect.MakeFunc",
	// Crypto / TLS
	"crypto/...", "crypto/tls.Config",
	// SQL
	"database/sql.(*DB).*", "database/sql.(*Tx).*", "database/sql.(*Conn).*", "database/sql.(*Stmt).*",
	// HTTP
	"net/http.ListenAndServe", "net/http.ListenAndServeTLS", "net/http.(*Server).*", "net/http.(*Client).*",
	"net/http.Get", "net/http.Post", "net/http.PostForm", "net/http.Head", "net/http.Handle",
	"net/http.HandleFunc",
	// Data access / crypto libraries (the common ones; matched by import path,
	// so a module that does not use them is unaffected)
	"github.com/jackc/pgx/v5.(*Conn).*", "github.com/jackc/pgx/v5.(Tx).*",
	"github.com/jackc/pgx/v5/pgxpool.(*Pool).*", "github.com/jackc/pgx/v5/pgxpool.(*Conn).*", "github.com/jackc/pgx/v5/pgxpool.(*Tx).*",
	"github.com/jackc/pgx/v4.(*Conn).*", "github.com/jackc/pgx/v4.(Tx).*",
	"github.com/jackc/pgx/v4/pgxpool.(*Pool).*", "github.com/jackc/pgx/v4/pgxpool.(*Conn).*", "github.com/jackc/pgx/v4/pgxpool.(*Tx).*",
	"github.com/jmoiron/sqlx.(*DB).*", "github.com/jmoiron/sqlx.(*Tx).*",
	"gorm.io/gorm.(*DB).*",
	"golang.org/x/crypto/...",
	// Deserialization / templates
	"encoding/gob.*", "encoding/xml.(*Decoder).*", "encoding/xml.Unmarshal",
	"text/template.(*Template).Execute*", "html/template.(*Template).Execute*",
}

// Table is an effective sensitive table. The zero value matches nothing.
type Table struct {
	patterns []pattern
}

// pattern is one parsed table entry.
type pattern struct {
	path   string // package path, without a trailing "/..."
	tree   bool   // path ended in "/...": the package and everything below
	method bool   // a (*T).M or (T).M pattern
	ptr    bool   // receiver written (*T)
	recv   string // T
	name   string // Name or M; "*" is any; a trailing "*" is a prefix
}

// NewTable is the default table (spec §3, verbatim) plus add, minus remove.
// A malformed pattern, or a remove entry not in the defaults, is an error
// wrapping config.ErrConfig.
func NewTable(add, remove []string) (Table, error) {
	for _, r := range remove {
		if !slices.Contains(defaultPatterns, r) {
			return Table{}, fmt.Errorf("%w: sensitive.remove %q: not in the default table", config.ErrConfig, r)
		}
	}
	var t Table
	for _, s := range defaultPatterns {
		if slices.Contains(remove, s) {
			continue
		}
		p, err := parsePattern(s)
		if err != nil { // a broken default is a programming error
			panic(fmt.Sprintf("sensitive: default %q: %v", s, err))
		}
		t.patterns = append(t.patterns, p)
	}
	for _, s := range add {
		p, err := parsePattern(s)
		if err != nil {
			return Table{}, fmt.Errorf("%w: sensitive.add %q: %v", config.ErrConfig, s, err)
		}
		t.patterns = append(t.patterns, p)
	}
	return t, nil
}

// parsePattern parses path.Name, path.*, path.(*T).M, path.(T).M, where path
// may end in "/..." (alone, it means every name in the tree) and M may end
// in "*".
func parsePattern(s string) (pattern, error) {
	if strings.ContainsAny(s, " \t\n") {
		return pattern{}, fmt.Errorf("contains whitespace")
	}
	if p, ok := strings.CutSuffix(s, "/..."); ok {
		if err := checkPath(p); err != nil {
			return pattern{}, err
		}
		return pattern{path: p, tree: true, name: "*"}, nil
	}
	var pat pattern
	var rest string
	if i := strings.Index(s, ".("); i >= 0 {
		pat.path, rest = s[:i], s[i+2:]
		recv, m, ok := strings.Cut(rest, ").")
		if !ok {
			return pattern{}, fmt.Errorf("want path.(*T).M or path.(T).M")
		}
		pat.method = true
		pat.recv, pat.ptr = strings.CutPrefix(recv, "*")
		if !token.IsIdentifier(pat.recv) {
			return pattern{}, fmt.Errorf("receiver %q is not an identifier", recv)
		}
		if m != "*" && !token.IsIdentifier(strings.TrimSuffix(m, "*")) {
			return pattern{}, fmt.Errorf("method %q is not an identifier, *, or a prefix ending in *", m)
		}
		pat.name = m
	} else {
		i := strings.LastIndex(s, ".")
		if i < 0 {
			return pattern{}, fmt.Errorf("want path.Name")
		}
		pat.path, pat.name = s[:i], s[i+1:]
		if pat.name != "*" && !token.IsIdentifier(pat.name) {
			return pattern{}, fmt.Errorf("name %q is not an identifier or *", pat.name)
		}
	}
	if p, ok := strings.CutSuffix(pat.path, "/..."); ok {
		pat.path, pat.tree = p, true
	}
	if err := checkPath(pat.path); err != nil {
		return pattern{}, err
	}
	return pat, nil
}

// checkPath rejects an empty path or an empty or wildcard segment.
func checkPath(p string) error {
	if p == "" {
		return fmt.Errorf("empty package path")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || strings.ContainsAny(seg, "*()") || seg == "..." {
			return fmt.Errorf("bad package path %q", p)
		}
	}
	return nil
}

// matches reports whether p matches the API a.
func (p pattern) matches(a api) bool {
	if a.pkg != p.path && !(p.tree && strings.HasPrefix(a.pkg, p.path+"/")) {
		return false
	}
	if !p.method {
		// path.* covers every function, type and method of the package;
		// path.Name only a package-level name.
		return p.name == "*" || (a.recv == "" && a.name == p.name)
	}
	if a.recv != p.recv || a.ptr != p.ptr {
		return false
	}
	if prefix, ok := strings.CutSuffix(p.name, "*"); ok {
		return strings.HasPrefix(a.name, prefix)
	}
	return a.name == p.name
}
