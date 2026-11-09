# Reach & Focus (sub-project B) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Flag changed code that touches sensitive operations (red when untested), show which entry points reach each change, and accept an optional `.mrimpact.yml`. Everything works with zero config.

**Architecture:** Two new pure packages:
- `internal/config` loads and validates the optional YAML;
- `internal/sensitive` holds the default API table and matches the calls in a type-checked function body.

`load.Program` gains a lookup from (file, line) to the function's AST and types. `callgraph` gains `EntryPoints`. `risk` gains the untested-sensitive rule. The pipeline and report wire them in. `cmd/mrimpact` loads the config, and maps config errors to exit 2.

**Tech Stack:** Go 1.26, `go/ast`, `go/types`, `golang.org/x/tools` (existing), `gopkg.in/yaml.v3` (new, the only new dependency).

**Spec:** `docs/superpowers/specs/2026-11-09-reach-focus-design.md`

## Global Constraints

- **Zero-config by default:** no file and no flag means every feature runs on the built-in defaults. A missing default-location file is silent: no warning line, no note text.
- **General Go:** the default table holds stdlib patterns only, exactly the spec §3 table, and callees are matched by resolved `types.Object` (package path + name), never by import string.
- Deterministic and offline. The only new dependency is `gopkg.in/yaml.v3`, used only by `internal/config`.
- **One new flag**, `--config <path>`. Config errors (bad file, `--config` pointing at a missing file) → exit 2, one-line reason on stderr, no note written or posted. Other exit codes and `--fail-on red` are unchanged.
- Copy, verbatim:
  - **Kind suffix:** ` · sensitive` (after ` · wiring` when both apply).
  - **Section header:** `🔐 **Sensitive code changed**`.
  - **Section line:** `- 🔴 \`pkg.F\` — os/exec.Command, database/sql.(*Tx).Exec · 0 % (0/7)` (up to 3 APIs, then ` +N`); a critical-only function shows `critical: <glob>` instead of the API list.
  - **Legend suffix:** ` · sensitive and untested 🔴`.
  - **Column:** `Entry points`, cell = first 2 then ` +N`, `—` when none; entry text forms `main`, `init`, `api <Display>`, and the registration call (e.g. `GET("/x", …)`).
- **Untested-sensitive rule:** with a profile, `Total > 0 && Covered == 0`; without one, `!TestReached`. The result is 🔴 regardless of callers, is not capped by wiring, and is not softened by cross-package reach. Partly tested stays 🟡. With no added lines, today's rules apply.
- Existing goldens change only where these rules change them (the Legend footer, the new column where a fixture has entry points, the sensitive suffix), and each changed golden is reviewed line by line.
- Commit trailer: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **A method value rather than a call** (`run := db.ExecContext; run(...)`) must still count as touching the API. Test: Task 2 case `method-value`.
2. **A sensitive call nested two closures deep** (`go func(){ defer func(){ os.Remove(p) }() }()`) must be found. Test: Task 2 case `nested-closure`.
3. **An existing but empty or comments-only `.mrimpact.yml`** must behave exactly like no file. Test: Task 1 `TestLoad_CommentsOnlyFileIsDefaults`.
4. **Running with `--dir` pointing at another checkout** must read `.mrimpact.yml` from that module root, not the working directory. Test: Task 5 `TestRun_ConfigReadFromDir`.
5. **A degraded call graph (timeout)** must still rate untested sensitive code 🔴 and show the 🔐 section, without the Entry points column. Test: Task 6 `TestRun_DegradedStillSensitive`.

---

### Task 1: `internal/config`

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`, testdata files under `internal/config/testdata/`
- Modify: `go.mod`, `go.sum` (add `gopkg.in/yaml.v3`)

**Interfaces:**
- Produces:
  ```go
  type Config struct {
      Critical        []string // globs as written
      SensitiveAdd    []string
      SensitiveRemove []string
  }
  // Load reads explicitPath when non-empty (must exist), else <moduleRoot>/.mrimpact.yml
  // when it exists, else returns the zero Config. Strict: unknown keys and wrong types are errors.
  func Load(moduleRoot, explicitPath string) (Config, error)
  // ErrConfig wraps every error Load returns (errors.Is), so the CLI maps it to exit 2.
  var ErrConfig = errors.New("config")
  ```
  `Load` validates YAML shape and globs only (`path.Match` on every segment after stripping a trailing `/...`). Pattern syntax and `remove` membership are validated by `sensitive.NewTable` (Task 2).

- [ ] **Step 1: Write failing tests:**
  - `TestLoad_NoFileIsDefaults`: an empty temp dir gives `Config{}` and a nil error.
  - `TestLoad_EmptyFileIsDefaults`: a zero-byte file gives `Config{}`.
  - `TestLoad_CommentsOnlyFileIsDefaults` (Review Focus 3): `# nothing\n` gives `Config{}`.
  - `TestLoad_Full`: the spec §5 example gives the 1 critical glob, 2 adds and 1 remove, verbatim.
  - `TestLoad_UnknownKey`: `critcal: [x]` → `errors.Is(err, ErrConfig)`, and the message contains `critcal`.
  - `TestLoad_WrongType`: `critical: internal/x` (a scalar) → `ErrConfig`.
  - `TestLoad_BadGlob`: `critical: ["internal/[x"]` → `ErrConfig`, and the message contains `internal/[x`.
  - `TestLoad_ExplicitMissing`: `Load(dir, dir+"/nope.yml")` → `ErrConfig`.
  - `TestLoad_ExplicitPathWins`: a root file and an explicit file both exist → the explicit one's content is used.
- [ ] **Step 2:** Run `go test ./internal/config/`. Expected: FAIL (package missing).
- [ ] **Step 3:** Implement with `yaml.v3` `Decoder.KnownFields(true)` into a struct `{Critical []string \`yaml:"critical"\`; Sensitive struct{Add, Remove []string} \`yaml:"sensitive"\`}`. Treat `io.EOF` (an empty document) as the zero Config.
- [ ] **Step 4:** Run `go test ./internal/config/ && go vet ./... && go mod tidy && git diff --stat go.mod`. Expected: PASS, and go.mod gains exactly `gopkg.in/yaml.v3`.
- [ ] **Step 5:** Commit `feat(config): optional .mrimpact.yml, strict, zero-config by default`.

### Task 2: `internal/sensitive`

**Files:**
- Create: `internal/sensitive/table.go` (default table + patterns), `internal/sensitive/match.go` (`Calls`, `Critical`), `internal/sensitive/sensitive_test.go`

**Interfaces:**
- Produces:
  ```go
  type Table struct{ /* unexported */ }
  // NewTable is the default table (spec §3, verbatim) plus add, minus remove. A malformed
  // pattern, or a remove entry not in the defaults, is an error wrapping config.ErrConfig.
  func NewTable(add, remove []string) (Table, error)
  // Calls returns the APIs fn touches: resolved callees of calls, method values and composite
  // literal types in fn's body and every nested function literal, rendered "pkgpath.Name" /
  // "pkgpath.(*T).M" / "pkgpath.(T).M", deduplicated, in first-occurrence source order.
  func (t Table) Calls(fn *ast.FuncDecl, info *types.Info) []string
  // Critical reports the first glob (in order) matching pkgPath; a glob without the module
  // prefix is joined to module; "/..." matches the package and everything below.
  func Critical(pkgPath, module string, globs []string) (glob string, ok bool)
  ```
  Pattern grammar: `path.Name`, `path.*`, `path.(*T).M`, `path.(T).M`, `path.(*T).*`, `path.(T).*`. `path` may end in `/...` (the package and its subpackages; `crypto/...` also matches `crypto` itself). The method `M` may end in `*` (prefix, e.g. `Execute*`). For a method, the receiver's named type must be declared in the pattern's package. A method reached through an interface matches only when the interface is declared in the pattern's package.

- [ ] **Step 1: Write failing tests**, table-driven over type-checked snippets (build them with `go/types` + `importer.ForCompiler(fset, "source", nil)`, or `packages.Load` on a testdata module; follow `internal/callgraph` test fixtures if they type-check from testdata):
  - **alias:** `x "os/exec"; x.Command("ls")` → `["os/exec.Command"]`.
  - **sql:** `db.ExecContext(ctx, q)` on `*sql.DB` → `["database/sql.(*DB).ExecContext"]`; `tx.Commit()` on `*sql.Tx` → `["database/sql.(*Tx).Commit"]`.
  - **method-value** (Review Focus 1): `run := db.ExecContext; run(ctx, q)` → `["database/sql.(*DB).ExecContext"]`.
  - **closure:** `func() { os.Remove(p) }()` → `["os.Remove"]`.
  - **nested-closure** (Review Focus 2): `go func(){ defer func(){ os.Remove(p) }() }()` → `["os.Remove"]`.
  - **tls-literal:** `_ = &tls.Config{}` → `["crypto/tls.Config"]`.
  - **crypto-tree:** `sha256.Sum256(b)` → `["crypto/sha256.Sum256"]`.
  - **template:** `t.ExecuteTemplate(w, "x", d)` on `*html/template.Template` → matched.
  - **plain:** `fmt.Println`, `strings.Join` → `nil`.
  - **foreign-interface:** a call through a locally declared `interface{ Exec(string) }` → `nil`.
  - **add:** `NewTable([]string{"example.com/l.(*Ledger).Post"}, nil)` matches a call to that method.
  - **remove:** `NewTable(nil, []string{"reflect.*"})` does not match `reflect.TypeOf`.
  - **order:** a body calling `os.Remove`, then `exec.Command`, then `os.Remove` → `["os.Remove", "os/exec.Command"]`.
  - **errors:**
    - `NewTable([]string{"os.(*File"}, nil)` → error.
    - `NewTable(nil, []string{"fmt.Println"})` (not a default) → error.
    - Both satisfy `errors.Is(err, config.ErrConfig)`.
  - **critical:**
    - `Critical("m/internal/pay/x", "m", []string{"internal/pay/..."})` → `"internal/pay/...", true`;
    - `Critical("m/internal/payx", "m", []string{"internal/pay/..."})` → false;
    - `"m/a/b"` with `"a/*"` → true;
    - a full import path glob → true.
- [ ] **Step 2:** Run `go test ./internal/sensitive/`. Expected: FAIL.
- [ ] **Step 3:** Implement. Walk the body with `ast.Inspect`, including `*ast.FuncLit` bodies. Resolve `*ast.CallExpr` Fun, `*ast.SelectorExpr` method values and `*ast.CompositeLit` types through `info.Uses` / `info.Selections` / `info.Types`.
- [ ] **Step 4:** Run `go test ./internal/sensitive/ && go vet ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(sensitive): default sensitive-API table and type-resolved matching`.

### Task 3: Function lookup + `callgraph.EntryPoints`

**Files:**
- Modify: `internal/load/load.go` (add a method), `internal/callgraph/callgraph.go`
- Test: `internal/load/load_test.go`, `internal/callgraph/callgraph_test.go` + fixture `internal/callgraph/testdata/entries/`

**Interfaces:**
- Produces:
  ```go
  // load: the declaration starting at file:line in a non-test package variant, with its
  // package's type info and path; ok=false when not found (e.g. excluded by build tags).
  func (p *Program) FuncDecl(file string, line int) (fd *ast.FuncDecl, info *types.Info, pkgPath string, ok bool)

  // callgraph
  type Entry struct{ Kind, Display string } // Kind: "main" | "init" | "api" | "registered"
  // EntryPoints walks all callers of the function at file:startLine (no depth limit) and
  // returns main/init functions, functions api(info) accepts, and the registration sites
  // (Site.Call) of every reached function; target included; tests never; deduplicated by
  // (Kind, Display); sorted by Kind in the order main, init, api, registered, then Display;
  // at most 20.
  func (g *Graph) EntryPoints(file string, startLine int, api func(FuncInfo) bool) []Entry
  ```
- [ ] **Step 1: Write failing tests.**
  - `TestProgram_FuncDecl`: it finds a function and a method by (file, line), and returns `ok=false` for a line with no declaration.
  - `TestEntryPoints` on fixture `example.com/ep`:
    - `cmd/main.go` main calls `svc.Run`, which calls `svc.helper`;
    - `api/api.go` (importable) has exported `api.Do`, which calls `svc.helper`;
    - `svc/routes.go` registers `svc.handle` via `mux.HandleFunc("/x", handle)`, and `handle` calls `svc.helper`;
    - a test calls `svc.helper`.

    Expected for `svc.helper`: `[{main main.main} {api api.Do} {registered HandleFunc("/x", …)}]`, with an api predicate equivalent to pipeline's `root`. For a function reached only by a test: `nil`.
  - `TestEntryPoints_Cap`: 25 exported callers give 20 entries.
- [ ] **Step 2:** Run `go test ./internal/load/ ./internal/callgraph/ -run 'FuncDecl|EntryPoints'`. Expected: FAIL.
- [ ] **Step 3:** Implement.
  - `FuncDecl`: iterate `p.Packages` and skip test variants (`strings.HasSuffix(pkg.ID, ".test]")` or `pkg.Name` ending `_test`). Match the file via `p.rel` and the decl start line.
  - `EntryPoints`: reuse the reverse traversal `Impact` uses, unbounded, with the existing `g.entry(n)` and `g.registered(id)`.
- [ ] **Step 4:** Run `go test ./internal/load/ ./internal/callgraph/`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(callgraph): EntryPoints; load: FuncDecl lookup`.

### Task 4: Risk rule

**Files:**
- Modify: `internal/risk/risk.go`
- Test: `internal/risk/risk_test.go`

**Interfaces:**
- Produces: `risk.Input.Sensitive bool`; `risk.Legend` ending with `· sensitive and untested 🔴`.

- [ ] **Step 1: Write failing tests:**
  - `TestAssess_SensitiveUntestedNoCallers`: HasProfile, 0/5, NonTestCallers 0, Sensitive → Red.
  - `TestAssess_SensitiveBeatsWiring`: the same plus Wiring → Red.
  - `TestAssess_SensitiveBeatsCrossPkg`: the same plus CrossPkgTestReached → Red.
  - `TestAssess_SensitiveStatic`: no profile, `TestReached` false → Red; true → Green.
  - `TestAssess_SensitivePartial`: 2/5 → Yellow.
  - `TestAssess_SensitiveCovered`: 5/5 → Green.
  - `TestAssess_SensitiveNoAddedLines`: HasProfile, Total 0, TestReached true → today's result (Green).
  - Update the exact-Legend test.
- [ ] **Step 2:** Run `go test ./internal/risk/`. Expected: FAIL.
- [ ] **Step 3:** Implement. Check the sensitive rule first after the exported-signature rule. "Untested" here means `HasProfile && Total > 0 && Covered == 0`, or `!HasProfile && !TestReached`.
- [ ] **Step 4:** Regenerate goldens for the Legend footer only (`go test ./internal/report/ -update`, `go test ./internal/pipeline/ -run TestE2E -update`). Confirm with `git diff` that the footer line is the only change in each, then run `go test ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(risk): untested sensitive code is red`.

### Task 5: Sensitive in the pipeline, the note and the CLI

**Files:**
- Modify:
  - `cmd/mrimpact/main.go`: `--config`; `config.Load(*dir, *configPath)`; `sensitive.NewTable`; an error wrapping `config.ErrConfig` → stderr + exit 2 before `pipeline.Run`;
  - `internal/pipeline/pipeline.go`: `Config.Sensitive sensitive.Table`, `Config.Critical []string`; per function `prog.FuncDecl` → `Calls` + `Critical` → `in.Sensitive`; kind suffix; `m.Sensitive`;
  - `internal/report/report.go`: `Model.Sensitive []SensitiveRow`; `writeSensitive` placed after the registration sections and before the table;
  - `README.md`: zero-config wording, the 🔐 section, `.mrimpact.yml` schema, `--config`.
- Create: e2e fixtures `testdata/e2e/sensitive_zero_config/` and `testdata/e2e/config_file/` (the latter with `head/.mrimpact.yml`), registered in `e2e_test.go`
- Test: `cmd/mrimpact/main_test.go`, `internal/report/report_test.go`, `internal/pipeline/e2e_test.go`

**Interfaces:**
- Consumes: Tasks 1–4.
- Produces: `report.SensitiveRow{Level risk.Level; Display string; APIs []string; Critical string; CoverText string}`. Ordering: untested first (`Level == Red` and the untested condition), then by Display.

- [ ] **Step 1: Write failing tests.**
  - **`TestRender_SensitiveSection`** (report): pins the header, a 2-API line, a 4-API line (`+1`) and a critical-only line, in the exact Global Constraints copy.
  - **`TestMain_InvalidConfigExits2`** (cmd): a repo with `.mrimpact.yml` containing `critcal: [x]` → exit code 2, stderr contains `critcal`, and no `--out` file is written.
  - **`TestMain_ExplicitMissingConfigExits2`.**
  - **`TestRun_ConfigReadFromDir`** (Review Focus 4): with the working directory elsewhere and `Dir` set to the `config_file` fixture repo, the critical function is sensitive. This test lives in `cmd/mrimpact`, because main does the loading.
  - **E2E `sensitive_zero_config`** (no config file):
    - `run.Exec`, new, calls `exec.Command`, has no callers and no test → 🔴 `new · sensitive`, listed first in the section;
    - `store.Save`, changed, calls `(*sql.DB).ExecContext`, covered → 🟢 and listed;
    - a plain changed helper is not listed.
  - **E2E `config_file`:**
    - `critical: [internal/pay/...]` makes untested `pay.Settle`, which has no sensitive calls, 🔴 with `critical: internal/pay/...`;
    - `sensitive.remove: [reflect.*]` keeps a `reflect.TypeOf` caller out of the section.
- [ ] **Step 2:** Run the new tests. Expected: FAIL.
- [ ] **Step 3:** Implement. A function `prog.FuncDecl` cannot find is treated as not sensitive (the function is already in NotAnalyzed when its file is excluded). The pipeline's zero `Config.Sensitive` must behave as the default table: give `sensitive.Table` a usable zero value, or have `Run` call `NewTable(nil, nil)` when it is unset, so library callers get defaults too.
- [ ] **Step 4:** Generate the two new goldens (`go test ./internal/pipeline/ -run 'TestE2E/(sensitive_zero_config|config_file)$' -update`) and read them. Run `go test ./...` without `-update`. Every other golden change must be explained by the ` · sensitive` suffix on a fixture that calls a table API; list such goldens in the commit message. Run `go vet ./...` and `gofmt -l .`.
- [ ] **Step 5:** Commit `feat: sensitive code in the note, --config, zero-config defaults`.

### Task 6: Entry points column

**Files:**
- Modify: `internal/pipeline/pipeline.go` (extract the existing `root` closure into `apiRoot(prog *load.Program) func(callgraph.FuncInfo) bool`, used by both `Dead` and `EntryPoints`; fill `FuncRow.Entries`), `internal/report/report.go` (`FuncRow.Entries []callgraph.Entry`; the column; `entryCell`), `README.md`
- Test: `internal/report/report_test.go`, `internal/pipeline/e2e_test.go`

**Interfaces:**
- Consumes: `callgraph.EntryPoints`, `callgraph.Entry` (Task 3).
- Produces: a column placed after `Callers` (and after `Registered` when present), named `Entry points`. Cell: the first 2 entries rendered as `main` / `init` / `api <Display>` / `<Site.Call>`, joined by `, `, then ` +N`, or `—` when none. The column is omitted when no shown row has an entry, or when the graph is degraded.

- [ ] **Step 1: Write failing tests.**
  - `TestRender_EntryPointsColumn`: 3 entries render as `main, api x.Do +1`; zero entries render as `—`; the column is absent when every row has none.
  - `TestRun_DegradedStillSensitive` (Review Focus 5): the `sensitive_zero_config` fixture run with `Timeout: time.Nanosecond` gives `m.Degraded`, `run.Exec` still 🔴, `len(m.Sensitive) > 0`, and no row with `Entries`.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3:** Implement. Group rows use the entry's own `Entries`. Members carry none.
- [ ] **Step 4:** Regenerate the goldens the column changes. Read each changed golden: the only differences must be the new column header and cells, and each cell must agree with that fixture's call graph. Run `go test ./...`, `go vet ./...` and `gofmt -l .`. Expected: PASS.
- [ ] **Step 5:** Commit `feat: entry points column`.

### Task 7: Validate on real repositories

**Files:** none committed unless a misfire is found.

- [ ] **Step 1:** Build, then run with no config file on the two real branches used for sub-project A, with the same coverprofiles. Expected:
  - the 🔐 section lists functions that touch the default APIs (e.g. HTTP handlers, SQL);
  - every 🔴 is either an untested sensitive function or one of the earlier genuine reds;
  - the Entry points cells match what you know of each function's routes and `main`.

  Note the red counts and the section size in the hand-off.
- [ ] **Step 2:** If a rule misfires on real code (a false sensitive match, a missed one, a wrong entry), add a failing test reproducing it to the owning task's test file, fix it, and commit `fix: <rule> on <construct>`.
