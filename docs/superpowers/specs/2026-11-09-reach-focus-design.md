# mrimpact — Reach & focus (sub-project B)

- **Date:** 2026-11-09
- **Status:** Draft, pending author review
- **Base:** `feat/signal-fixes` at b0be7ac (sub-project A, signal fixes)
- **Follows:** `2026-11-06-mrimpact-design.md`, `2026-11-09-signal-fixes-design.md`

## 1. Problem

The note says *who calls* a changed function and *whether it is tested*. It does not say:

1. **What it touches.** A changed function that runs a shell command, opens a SQL transaction, writes files or handles HTTP is a different review from one that formats a string. Today both get the same row.
2. **Where it can be reached from.** "2 callers at depth 2" does not tell a reviewer that the change sits behind a public route or the program's `main`.

**Goal:** flag changed code that touches sensitive operations, rate it red when it is untested, and show which entry points reach each change. All of this must work on any Go repository with no setup.

## 2. Constraints

- **Zero-config by default.** mrimpact must be fully useful with no config file and no new flag. The built-in defaults drive every feature here. The config file only *adds* project knowledge, such as critical packages and extra sensitive APIs, or *removes* a default. A missing file is the normal case and is never a warning.
- **General Go.** The default sensitive table lists only standard-library or universally known APIs (the data-access / crypto row: pgx, sqlx, gorm, golang.org/x/crypto; routers such as gin, echo or chi stay out). Detection uses type-checked objects (package path + name), never import strings or project names.
- **Deterministic and offline.** No network, AI service, API key or token.
- **Unchanged CLI contract**, with one addition: an optional `--config <path>`. Exit codes, `--fail-on red` and posting are unchanged.
- **One new dependency:** `gopkg.in/yaml.v3`, used only to read the optional file.

## 3. Sensitive code

### Detection

A changed (non-deleted, non-test) function is **sensitive** when either:

- its body, including the bodies of function literals inside it, contains a call or method value whose resolved callee matches a pattern in the effective sensitive table; or
- its package matches a `critical` glob from the config.

Detection works on the type-checked head program, the same one the call graph is built from. The callee is resolved through `types.Info.Uses` / `Selections`, so aliased imports and method calls on interfaces declared in a sensitive package (e.g. `*sql.DB`, `*sql.Tx`) are found. A call through an interface not declared in a sensitive package is not matched.

### Default table

Patterns use `pkgpath.Name`, `pkgpath.*`, `pkgpath.(*T).M` or `pkgpath.(T).M` (`*` in the method position matches any method).

| Area | Patterns |
|---|---|
| Process / OS | `os/exec.*`, `syscall.*`, `unsafe.*`, `plugin.*`, `os.(*Process).*`, `os.StartProcess` |
| Files | `os.Remove`, `os.RemoveAll`, `os.WriteFile`, `os.Create`, `os.OpenFile`, `os.Chmod`, `os.Chown`, `os.Rename`, `os.Symlink`, `os.Link` |
| Reflection | `reflect.(Value).Set*`, `reflect.(Value).Send`, `reflect.(Value).TrySend`, `reflect.(Value).Clear`, `reflect.Copy`, `reflect.Swapper`, `reflect.NewAt`, `reflect.(Value).UnsafePointer`, `reflect.(Value).UnsafeAddr`, `reflect.(Value).Pointer`, `reflect.(Value).Call*`, `reflect.(Value).Method`, `reflect.(Value).MethodByName`, `reflect.(Type).MethodByName`, `reflect.MakeFunc` (mutating, unsafe and dynamic-dispatch APIs; read-only `TypeOf`, `ValueOf`, `DeepEqual`, `Kind`, `Interface` are not sensitive) |
| Crypto / TLS | `crypto/...` (every package under `crypto`), `crypto/tls.Config` (composite literal counts) |
| SQL | `database/sql.(*DB).*`, `database/sql.(*Tx).*`, `database/sql.(*Conn).*`, `database/sql.(*Stmt).*` |
| HTTP | `net/http.ListenAndServe`, `net/http.ListenAndServeTLS`, `net/http.(*Server).*`, `net/http.(*Client).*`, `net/http.Get`, `net/http.Post`, `net/http.PostForm`, `net/http.Head`, `net/http.Handle`, `net/http.HandleFunc` |
| Data access / crypto libraries | `github.com/jackc/pgx/v5.(*Conn).*`, `github.com/jackc/pgx/v5.(Tx).*`, `github.com/jackc/pgx/v5/pgxpool.(*Pool).*`, `github.com/jackc/pgx/v5/pgxpool.(*Conn).*`, `github.com/jackc/pgx/v5/pgxpool.(*Tx).*`, the same for `github.com/jackc/pgx/v4`, `github.com/jmoiron/sqlx.(*DB).*`, `github.com/jmoiron/sqlx.(*Tx).*`, `gorm.io/gorm.(*DB).*`, `golang.org/x/crypto/...` |
| Deserialization / templates | `encoding/gob.*`, `encoding/xml.(*Decoder).*`, `encoding/xml.Unmarshal`, `text/template.(*Template).Execute*`, `html/template.(*Template).Execute*` |

`crypto/...` and the `.../` form match the package and every package below it. A composite literal of a matched type (e.g. `tls.Config{…}`) counts as touching it.

### Effect

- **Kind column:** ` · sensitive` is appended, after ` · wiring` if both apply.
- **Rating:** `risk.Input` gains `Sensitive bool` and `AddedLines int`. A sensitive function that is *untested* is 🔴, whatever its caller count. Untested means: it has added lines and, with a profile, they are `0/N` (N > 0); without a profile, no test reaches it. Without a profile and without a call graph (degraded), test reach is unknown (`Input.ReachUnknown`): the rule does not fire and today's rating applies; the function is still listed in the section. This rule:
  - does not apply to `main` (of package main), `init` or wiring functions (`Input.Root`, `Input.Wiring`): they keep today's rules, so wiring stays capped at 🟡; they are still listed in the section;
  - is not softened by cross-package reach, because a cross-package test that reaches the function does not prove its added lines ran;
  - leaves partly tested sensitive functions as they are (🟡), listed in the section below;
  - leaves a sensitive function with no added lines (lines only removed) rated by today's rules.
- **Groups:** a sensitive member's own rating decides whether it is folded, per R7 of sub-project A. An untested sensitive member is 🔴 and stays its own row. A group row whose entry function is sensitive keeps the suffix: `new · +N · sensitive`.
- **Section** "🔐 **Sensitive code changed**", placed after the registration sections and before the table, listing every sensitive changed function, untested first, then by Display: `- 🔴 \`pkg.F\` — os/exec.Command, database/sql.(*Tx).Exec · 0 % (0/7)`. Up to 3 matched APIs are shown, then ` +N`. A function marked only by `critical` shows `critical: <glob>`.
- **Legend:** gains `· sensitive and untested 🔴`.

## 4. Affected entry points

### Rule

`callgraph` gains `EntryPoints(file string, startLine int, api func(FuncInfo) bool) []Entry`, with `Entry{Kind, Display string}`. It walks the callers of the target with no depth limit and collects these, in this order (the most specific first):

1. registration sites of any reached function, from the existing `Registered` data (Kind `registered`, Display the shortened call, e.g. `GET("/rate-correlation", …)`);
2. exported functions of importable packages, and exported methods whose receiver's named type is exported, in importable packages, reached (Kind `api`, Display the function). This predicate is for entries only and narrower than the `root` rule `Dead` uses, which is unchanged (there an exported method of any type stays a root, since it may implement a dependency's interface). Generated code is never an `api` entry, as in `Dead`'s rule;
3. `main` and `init` functions reached (Kind `main` / `init`).

The target itself counts if it qualifies. Test functions are never entry points. The list is deduplicated by (Kind, Display), sorted by Kind order (registered, api, main, init) then Display, and capped at 20 per function.

### Effect

- **Table:** a new column **Entry points** shows the first 2 as `GET("/x", …)`, `api orders.Create`, `main`, then ` +N`, or `—` when none. A registration and an `api` function render as code spans (a double-backtick span when the text holds a backtick), `|` escaped, so `*`, `_` and `<` stay literal. When no row has any entry point, the column is omitted, as `Registered` already is. When the table shows at least 2 rows, an entry point every shown row has (e.g. `main`, `RunContext(…)`) tells them apart in no way: it is left out of every cell (and of ` +N`) and stated once in a line directly above the table, `All changes shown are reached from: <entry>, <entry>`, rendered as in the cells and in their order; a cell left empty renders `—`.
- **Rating:** unchanged. Entry points are context only.
- **Degraded call graph:** the column is omitted.
- A registration that is not a call argument (no call text) shows its `file:line`, as the Registered column does; entries are deduplicated on that text.

## 5. Configuration file

### Location

Without `--config`, mrimpact reads `.mrimpact.yml` from the module root. If it doesn't exist, it uses the defaults, prints nothing and posts nothing about it. With `--config <path>`, the file must exist: a missing file there is a usage error (exit 2).

### Schema

```yaml
critical:                 # package globs; functions in matching packages are sensitive
  - internal/payment/...  # module-relative, or a full import path
sensitive:
  add:                    # patterns added to the default table
    - github.com/shopspring/decimal.*
    - example.com/ledger.(*Ledger).Post
  remove:                 # patterns removed from the default table, matched textually
    - os/exec.*
```

- Every key is optional. An empty file equals no file.
- Parsing is strict: an unknown key, a wrong type, a glob that `path.Match` rejects, a malformed pattern, or a `remove` entry not in the default table is a usage error (exit 2). The message names the key and the value. The note is never posted with a half-applied config.
- Globs:
  - `internal/x/...` matches the package and everything below it.
  - `internal/x/*` uses `path.Match` semantics on one segment.
  - A glob without the module prefix is joined to the module path.
  - Globs are cleaned with `path.Clean`; a leading `./` is stripped and a trailing `/` accepted (`./internal/x/` reads as `internal/x`).
  - A glob that matches no loaded package adds a warning to the note: `critical glob "<g>" matches no package`.

- **Visibility.** The config is read from the head checkout. Whenever the effective config has `sensitive.remove` entries or `critical` globs, the note shows one line after the header block: `⚙️ .mrimpact.yml: critical <globs> · removes <patterns> · adds <patterns>` (empty parts omitted; `defaults` when none is left; with `--config` the line names the path as given). When the file differs between base and head (the base version is read with `git show`; a base without it is the empty config; an explicit `--config` outside the repository is not compared), the line also appears and ends ` — changed in this MR (MR removes from defaults: os/exec.*; …)`, listing per key what the MR added or dropped (`adds critical` / `drops critical`, `removes from defaults` / `restores defaults`, `adds` / `drops added`). A base file that does not parse reads ` — changed in this MR (base config unreadable)`; the base file never fails the run. Order and comments do not count as a change.

## 6. Components

| Package | Change |
|---|---|
| `internal/config` (new) | `type Config{Critical []string; SensitiveAdd, SensitiveRemove []string}`; `Load(moduleRoot, explicitPath string) (Config, error)`; `Parse(name, io.Reader)`; `Changes(base, head) []Change`; validation |
| `internal/sensitive` (new) | default table; `type Table`; `NewTable(add, remove []string) (Table, error)`; `Calls(fn *ast.FuncDecl, info *types.Info) []string` (matched API names, deterministic order); `Critical(pkgPath string, globs []string, module string) (glob string, ok bool)` |
| `callgraph` | `EntryPoints`, `Entry` |
| `risk` | `Input.Sensitive`; untested-sensitive rule; Legend |
| `symbols` / `pipeline` | `pipeline` needs each changed function's `*ast.FuncDecl` and `types.Info` from the loaded head program. It resolves them by file and start line from `load.Program`, adding a lookup there if none exists. |
| `report` | Entry points column; 🔐 section; kind suffix |
| `cmd/mrimpact` | `--config` flag; config errors map to exit 2 |

## 7. Error handling

- **Config errors:** usage error, exit 2, with a one-line reason on stderr. The note is not written or posted. This applies to the head file only; the base file is only compared, and one that cannot be read or parsed is stated in the config line.
- **A function whose declaration can't be found in the loaded program** (build tags): not rated today, so not checked here either (unchanged).
- **Degraded call graph:** no entry points. Sensitive detection still runs, since it needs only types; without a profile the untested-sensitive rule does not fire (test reach is unknown), with one the profile decides.

## 8. Testing

- **`config`:**
  - no file → defaults;
  - empty file;
  - full valid file;
  - unknown key;
  - wrong type;
  - bad glob;
  - bad pattern;
  - `remove` of a pattern not in the defaults;
  - `--config` pointing at a missing file.
- **`sensitive`:** table-driven over type-checked snippets:
  - `exec.Command` through an alias;
  - `(*sql.DB).ExecContext`;
  - `(*sql.Tx).Commit`;
  - a call inside a closure;
  - `tls.Config{}` composite literal;
  - `crypto/sha256.Sum256` via `crypto/...`;
  - a call through a non-sensitive interface (not matched);
  - an `add` pattern on a third-party method;
  - `remove: os/exec.*`;
  - deterministic ordering of results.
- **`critical`:** `...` and `*` globs, plus module-relative vs full path.
- **`callgraph`:** `EntryPoints` through a route registration, through an exported API of an importable package, and through `main`; none for test-only reach; the cap.
- **`risk`:**
  - untested sensitive with 0 callers → 🔴;
  - untested sensitive + wiring → 🔴;
  - untested sensitive + cross-package reach → 🔴;
  - partly tested sensitive → 🟡;
  - covered sensitive → 🟢.
- **E2E fixtures:**
  - `sensitive_zero_config`: no config file; an untested `exec.Command` with no callers → 🔴 with the section; a tested SQL write → 🟢 and listed; entry points shown.
  - `config_file`: a `critical` glob marks a plain function sensitive; `remove: os/exec.*` silences an os/exec call.
  - A CLI test: an invalid `.mrimpact.yml` → exit 2 and no note.
- **Existing goldens:** regenerated only where these rules change them (the new column and the Legend), each reviewed line by line.

## 9. Out of scope

Architecture rules (next sub-project); taint or data-flow tracking (which data reaches the sensitive call); vulnerability data; per-API severity levels; any setting beyond `critical` and `sensitive.add/remove`.
