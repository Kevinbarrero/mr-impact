# mrimpact — Signal fixes (sub-project A)

- **Date:** 2026-11-09
- **Status:** Draft, pending author review
- **Base:** `feat/v1` at 917f58d (v0.2.0)
- **Follows:** `2026-11-06-mrimpact-design.md`. Later sub-projects: B reach & focus, C pure-Go detectors, D wider-input detectors, each with its own spec.

## 1. Problem

A run on two production Go services (41 and 39 changed functions) showed where the note misdirects a reviewer:

1. **Cross-package tests read as "untested".** `go test ./... -coverprofile` without `-coverpkg` credits a package's blocks only to that package's own tests. A function exercised by an integration test in another package shows `0 %`. With callers it is rated 🔴. In that run, most red rows were functions that a test in another package does exercise.
2. **Composition roots are always red.** Wiring functions (constructing services and connecting them) are rarely unit-tested and have callers (`main`), so every MR that adds a dependency gets a red row nobody acts on.
3. **New code floods the table.** A new service with 20 helpers that only call each other yields 20+ 🟡 rows. That is one decision for a reviewer, not 20.

**Goal:** the rows a reviewer reads first are the ones that need attention, with no loss of information. Everything folded or softened stays visible and says why.

## 2. Constraints

- **General Go.** Every rule uses only the Go AST, the type system, the SSA call graph and the standard coverprofile format. Nothing depends on a project, a library or a function name chosen by a project.
- **Deterministic and offline.** The same input always gives the same note. There is no network access beyond the existing GitLab posting, and no AI service, API key or token.
- **No new flags.** The behaviour is the new default; nothing is configurable in this sub-project.
- **Unchanged CLI contract.** Exit codes, `--fail-on red`, posting and the note marker stay as they are. `--fail-on red` sees the new ratings, so fewer false reds can fail a job.

## 3. Coverage across packages

### Rule

`callgraph.Impact` gains `TestPkgs []string`: the sorted, distinct package paths of the test entry functions (`Test*`, `Benchmark*`, `Fuzz*`, `Example*` in `_test.go`) that reach the target at any depth. These are the same functions that set `TestReached` today. An external test package (`pkg_test`) counts as its own package path.

A changed function is **covered only cross-package** when all of these hold:

- a coverprofile is in use, and its added lines are known (`Total > 0`) and covered `0/Total`;
- `TestPkgs` contains at least one path other than the function's own package (the `_test` suffix stripped for the comparison).

### Effect

- **Rating:** `risk.Input` gains `CrossPkgTestReached bool`. When it is set, the function is rated as *partial* (🟡) instead of *untested*. Red remains for a function no test reaches at all, or for an exported signature change in an importable package (unchanged rule, which takes precedence).
- **Cover cell:** `0 % in profile · reached by tests in <pkg>[, <pkg>][ +N]`. Packages are shown relative to the module path (`internal/x`), with `_test` stripped and duplicates dropped (`x` and `x_test` show `x` once), at most 2 in first-seen order, then `+N` distinct names. Reach is static (call graph), not observed at run time.
- **Note:** when at least one function qualifies, one line goes above the table:
  > ℹ️ N functions are covered only by tests in other packages; if the coverprofile was made without -coverpkg it counts same-package tests only. Run `go test -coverpkg=./... -coverprofile=…` for line coverage.
- **Static mode** (no profile) is unchanged. `reached by test` already counts any package.

## 4. Wiring

### Rule

New file `internal/symbols/wiring.go`, `isWiring(fd *ast.FuncDecl, imports map[string]string) bool` (imports: the file's import names to paths), sets `ChangedFunc.Wiring` for non-deleted, non-test functions. Wiring constructs and connects but computes nothing. A function with any parameter (the receiver excluded) of type exactly `context.Context` is never wiring: it runs per request, not once at startup. `context` is resolved through the file's imports, so an aliased import (`c "context"` → `c.Context`) counts; a dot-import is not resolved, and a pointer or any other type does not count. Otherwise a body is wiring when it meets both conditions below.

**1. Statements.** Every statement, recursively in the allowed blocks, is one of:

- an expression statement (a call), or an assignment or `:=` / `var` declaration;
- `return`, with any results;
- an `if` whose condition is `<ident> != nil` (or `nil != <ident>`), with no `else`, and an optional init statement that is itself an assignment or `:=` (`if err := f(); err != nil`), whose body only returns, calls `panic`, or calls a function or method whose name starts with `Fatal` or is `Exit`. An optional preceding call or assignment in the body is allowed, so `log.Printf(…); return err` counts.

**2. Plumbing only.** Every expression in the body is plumbing: an identifier, a selector (`x.y`), a basic literal, a function literal whose body is itself wiring by these statement and plumbing rules (recursively), a call whose function and arguments are plumbing (type conversions are calls), a composite literal whose elements and key-value pairs are plumbing, `&` or `*` of plumbing, or parentheses around plumbing. Binary operators (arithmetic, comparison, logical), index, slice, type assertion and unary operators other than `&` (`!`, `-`, `<-`) make the function not wiring. The only exception is the `<ident> != nil` condition above. This covers assignment sides, return results, call statements, `var` values and the `if` init. The body must also contain at least one call or composite literal, so a body of only `return x` or `return nil` is not wiring.

Function literals that appear as expressions (arguments, assigned values) are inspected: a closure body that is not wiring makes the outer function not wiring, since a closure handed to `InTx`, `Go` or a handler table is where the real work hides. A closure's own calls and composite literals count toward "at least one call or composite literal"; an empty closure body is accepted. Any other statement makes the function not wiring: `if` (other conditions, or with `else`), `for`, `range`, `switch`, `type switch`, `select`, `go`, `defer`, `goto`, labelled statements, and an `if` whose init is anything but an assignment. An empty body is not wiring.

The rule works in any package, `main` included or not.

### Effect

- **Kind column:** `wiring` replaces the kind for a `new` function (`new · wiring`), or is appended for others (`body · wiring`, `signature · wiring`).
- **Rating:** `risk.Input` gains `Wiring bool`. `risk.Assess` caps a wiring function at 🟡: a computed 🔴 becomes 🟡, and 🟡 or 🟢 stay as they are. The exported-signature red rule is **not** capped, since a broken exported API is not a wiring concern.
- **Legend:** gains `· wiring (straight-line setup) at most 🟡`.

## 5. New-code groups

### Rule

Grouping runs in the pipeline after every function is rated, when the call graph is available. It is skipped when the graph is degraded, and the table is then unchanged.

Among the rated functions of kind `new`, from depth-1 non-test callers and the registering functions in `Registered`:

- **Member:** is not rated 🔴 on its own, has at least one non-test caller or registrant, and every one of them is another `new` function of this MR.
- **Entry:** every other `new` function. Existing code calls or registers it, nothing in non-test code does, or it is rated 🔴 on its own: a red function always keeps its own row, so a sum over a covered entry never hides it, and it claims the members it reaches like any entry.

Each member is assigned to exactly one entry: the entry with the smallest `Display` (byte order) from which the member is reachable through members only. A member reachable from no entry, for example a cycle of new functions with no outside caller, is assigned to the smallest `Display` member of its cycle, which becomes the entry. Assignment is deterministic and never double-counts.

The partition and assignment live in a pure function, `pipeline/group.go`: `group(rows []ratedFunc, edges map[string][]string) []groupRow`, with no I/O.

### Effect

- An entry with members is shown as **one row**:
  - **Function:** the entry's display.
  - **Kind:** `new · +N` (N = members).
  - **Callers / Registered:** the entry's own.
  - **Cover:** the sums over entry and members of `Covered` and `Total`: `74 % (52/70)`. When any member alone is `0/k` with k > 0 and no test in another package reaches it (its cross-package list is empty), ` · K untested` is appended, K counting those members. With no profile, it reads `reached by test` if any of them is reached, else `not reached`.
  - **Rating:** `risk.Assess` over the sums, with `Kind = new`, `NonTestCallers` = the entry's, `TestReached` = any, and `CrossPkgTestReached` per §3 applied to the sums. `Wiring` is set only if every function in the group is wiring. The group row is never rated below its entry's own rating when that is 🔴: an entry red on its own keeps the group 🔴, whatever its members cover.
- Under the table, one `<details>` block per group row the table shows (the first 50 rows; a group past the cut gets none): `<summary>X +N new</summary>` listing each member with its own rating, cover cell and file, sorted by `Display`.
- The diagram shows the entry only. Members are not nodes.
- Entries without members, and every non-`new` function, stay ordinary rows.
- The header count ("41 functions changed") keeps counting every changed function, members included. The 🔴/🟡/🟢 tally counts **rows**, so a group counts once with its own rating. A new line under the header, `+N new functions folded into M groups` (`1 group` when M = 1), appears when grouping folded anything.

## 6. Components

| Package | Change |
|---|---|
| `callgraph` | `Impact.TestPkgs` (collected in the existing caller walk; `n.pkg` of test-entry nodes) |
| `symbols` | `ChangedFunc.Wiring`; `wiring.go` (`isWiring`) called from `Find` |
| `risk` | `Input.CrossPkgTestReached`, `Input.Wiring`; cap and partial rules; `Legend` text |
| `pipeline` | cover text and cross-package count; `group.go` (pure) and its call after rating |
| `report` | `Model.CrossPkgOnly int`, `Model.Groups`, group row rendering, members `<details>`, header fold line, hint line |

Data flow is unchanged up to rating. After rating, `pipeline` builds the groups and the report renders rows, then groups and members, then the existing sections.

## 7. Error handling

Nothing here can fail on its own. Missing data degrades to today's behaviour:

- No profile: the §3 rule is off.
- No call graph (timeout): no `TestPkgs`, no grouping.
- A file outside the load (build tags): not rated, so not grouped (unchanged).

## 8. Testing

- **`symbols/wiring_test.go`:** table of bodies.
  - **Wiring:** constructor chain; `if err != nil { return nil, err }`; `if err := f(); err != nil { return err }`; `log.Fatal(err)`; `os.Exit(1)`; `panic(err)`; a handler closure that only calls; `g.Go(func() error { return work(c) }); return g.Wait()`.
  - **Not wiring:** `if x > 0`, `if err != nil {…} else {…}`, `for`, `range`, `switch`, `select`, `go`, `defer`, an `if` whose init is a call statement (`if f(); err != nil`), an empty body, a closure argument containing a loop (`InTx(ctx, func(q Store) error { for … })`). A function with a `context.Context` parameter (plain or aliased import) whose body would otherwise be wiring; a constructor without one stays wiring.
- **`risk`:**
  - **Cap:** red → yellow for wiring; green and yellow unchanged; exported-signature red not capped.
  - **Cross-package:** the rule gives yellow, while 0 % with no reach stays red.
- **`callgraph`:** `TestPkgs` for a same-package test, an external `_test` package, and a test two packages away.
- **`pipeline/group_test.go`:**
  - a member shared by two entries goes to the smaller one;
  - a member also called from existing code becomes an entry;
  - a cycle with no outside caller;
  - an entry without members;
  - registration counts as a caller;
  - sums and ` · K untested`.
- **E2E fixtures** in `testdata/e2e/`, each with a golden note:
  - `cross_pkg_coverage`: a profile made without `-coverpkg`;
  - `wiring`: one wiring function in `main`, one in a library package, one near-miss with a loop;
  - `new_subtree`: an entry + 3 members, and an untested (🔴) function the entry calls that stays its own row.
- **Existing goldens:** regenerated only where these rules change them, and every changed golden reviewed line by line.

## 9. Out of scope

Entry-point reachability and the criticality config (B); new detectors (C, D); making any of these rules configurable.
