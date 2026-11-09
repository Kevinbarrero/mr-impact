# Signal Fixes (sub-project A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the note misdirecting reviewers. Functions tested from other packages are no longer red, wiring functions are capped at yellow, and new-code subtrees fold into one row per entry.

**Architecture:** Three independent signals feed the existing `risk.Assess`:
- `callgraph.Impact.TestPkgs`, for cross-package coverage;
- `symbols.ChangedFunc.Wiring`, from a pure AST classifier;
- grouping, a pure function over rated rows and their depth-1 callers, run by the pipeline after rating.

`report` renders group rows, member `<details>` and two new note lines. The CLI and flags are unchanged.

**Tech Stack:** Go 1.26, `go/ast`, `golang.org/x/tools` (SSA call graph, existing), stdlib `testing` with `-update` golden files.

**Spec:** `docs/superpowers/specs/2026-11-09-signal-fixes-design.md`

## Global Constraints

- Every rule uses only the Go AST, the type system, the SSA call graph and the coverprofile format. No project, library or function name appears in a rule. (`Fatal*`/`Exit`/`panic` are the only names, as the spec fixes.)
- Deterministic and offline: no network beyond the existing GitLab posting, no AI service, API key or token.
- No new flags. Exit codes, `--fail-on red`, posting and the marker are unchanged. `--fail-on red` loops over `Model.Funcs`, which after this work holds **rows** (a group is one row).
- Copy, verbatim from the spec:
  - Cover cell: `0 % in profile · reached by tests in <pkg>[, <pkg>][ +N]` (packages relative to the module path, at most 2).
  - Hint line: `ℹ️ N functions are covered only by tests in other packages; the coverprofile counts same-package tests only. Run `go test -coverpkg=./... -coverprofile=…` for line coverage.`
  - Kind: `new · wiring` / `body · wiring` / `signature · wiring`; group kind `new · +N`.
  - Group cover suffix ` · K untested`; header fold line `+N new functions folded into M groups`; members summary `X +N new`.
  - Legend suffix `· wiring (straight-line setup) at most 🟡`.
- Existing golden files are regenerated only where these rules change them, and every changed golden is reviewed line by line before committing.
- Commit trailer: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **An external test package (`x_test`) on its own package `x`** must not count as cross-package. The comparison strips `_test`. Test: Task 4 `TestCoverText_ExternalTestPackageIsSamePackage`.
2. **An `if err != nil` body that logs and falls through** (no return, panic, Fatal or Exit) is real branching, not wiring. Test: Task 2 case `log-and-continue`.
3. **`--fail-on red` with a red member inside a non-red group** now passes, because the group row carries the summed rating. That is intended: the tally and fail-on both count rows. Pin it so a later change is deliberate. Test: Task 5 `TestGroup_RatingFromSumsNotWorstMember`.
4. **Methods on pointer and generic receivers** must still match between `Impact.Callers[].Display`+`Pkg` and the changed function, or grouping silently folds nothing. Test: Task 5 `TestGroup_PointerReceiverMethodIsMember`, using displays in the `pkg.(*T).M` form.
5. **The degraded call graph (timeout)** must leave the table exactly as today, with no groups and no cross-package text. Test: Task 6 `TestRun_DegradedGraphNoGroups`.

---

### Task 1: `callgraph.Impact.TestPkgs`

**Files:**
- Modify: `internal/callgraph/callgraph.go` (`Impact` struct ~line 42; the loop over `depthOf` in `Impact`, ~line 515)
- Test: `internal/callgraph/callgraph_test.go`, fixture under `internal/callgraph/testdata/testpkgs/` (new)

**Interfaces:**
- Produces: `Impact.TestPkgs []string`: the sorted, distinct `node.pkg` values of test-entry nodes reaching the target at any depth. It is empty when there are none.

- [ ] **Step 1: Write the failing test.** Add `TestImpact_TestPkgs`, with a fixture module `example.com/tp` that has:
  - `a/a.go` with `func A()`;
  - `a/a_test.go` (package `a`) with `TestA` calling `A`;
  - `a/x_test.go` (package `a_test`) with `TestAX` calling `a.A`;
  - `b/b_test.go` with `TestB` calling `a.A` through `b.Helper()`.

  It asserts that `Impact(file of A, line, 2).TestPkgs` equals `[]string{"example.com/tp/a", "example.com/tp/a_test", "example.com/tp/b"}`, and that a function no test reaches has `TestPkgs == nil`.
- [ ] **Step 2:** Run `go test ./internal/callgraph/ -run TestImpact_TestPkgs`. Expected: FAIL (field undefined).
- [ ] **Step 3:** Implement. Collect `n.pkg` into a set where `im.TestReached = true` is set, then sort.
- [ ] **Step 4:** Run `go test ./internal/callgraph/...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(callgraph): Impact.TestPkgs, the packages of reaching tests`.

### Task 2: Wiring classifier

**Files:**
- Create: `internal/symbols/wiring.go`, `internal/symbols/wiring_test.go`
- Modify: `internal/symbols/symbols.go` (`fn` struct ~line 89: add `wiring bool`; set it where `x.branches = branches(d)` at ~line 311; copy it into `ChangedFunc` at ~line 197; add the `Wiring bool` field to `ChangedFunc` ~line 29)

**Interfaces:**
- Produces: `func isWiring(fd *ast.FuncDecl) bool` (unexported); `ChangedFunc.Wiring bool` (false for deleted and test functions).

- [ ] **Step 1: Write the failing test.** `TestIsWiring` is table-driven. Each case is a Go source snippet parsed with `parser.ParseFile`, and `isWiring` is called on its first `FuncDecl`.
  - **Want `true`:**
    - `chain` (`a := NewA(); b := NewB(a); b.Set(a); return b`)
    - `err-return` (`x, err := f(); if err != nil { return nil, err }; return x, nil`)
    - `if-init` (`if err := f(); err != nil { return err }; return nil`)
    - `log-then-return` (`if err != nil { log.Printf("x"); return err }`)
    - `fatal` (`if err != nil { log.Fatal(err) }`)
    - `exit` (`if err != nil { os.Exit(1) }`)
    - `panic` (`if err != nil { panic(err) }`)
    - `closure-arg-with-loop` (`r.Handle(func() { for {} })`)
    - `composite` (`return &S{A: a, B: []int{1}}`)
    - `var-decl` (`var s S; s.X = 1; return s`)
    - `nil-on-left` (`if nil != err { return err }`)
  - **Want `false`:**
    - `cond` (`if x > 0 { return }`)
    - `else` (`if err != nil { return err } else { g() }`)
    - `for`, `range`, `switch`, `typeswitch`, `select`, `go`, `defer`
    - `if-init-call` (`if f(); err != nil { return err }`)
    - `log-and-continue` (`if err != nil { log.Printf("x") }`)
    - `empty` (`func f() {}`)
    - `label` (`L: g()`)
- [ ] **Step 2:** Run `go test ./internal/symbols/ -run TestIsWiring`. Expected: FAIL (undefined).
- [ ] **Step 3:** Implement `isWiring` per spec §4. The `if` body must end in a terminating statement: `return`, or an expression statement calling `panic`, a selector or ident whose name has prefix `Fatal`, or one named `Exit`. Statements before it are only calls or assignments. Function literals are not descended into. Set `fn.wiring` for non-test files, and copy it to `ChangedFunc.Wiring` when the kind is not Deleted.
- [ ] **Step 4:** Add `TestFind_SetsWiring` to `symbols_test.go`: a changed file with one wiring and one non-wiring function, asserting `Wiring` on each `ChangedFunc`. Run `go test ./internal/symbols/...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(symbols): classify straight-line setup functions as wiring`.

### Task 3: Risk rules

**Files:**
- Modify: `internal/risk/risk.go`
- Test: `internal/risk/risk_test.go`

**Interfaces:**
- Produces:
  - `risk.Input.CrossPkgTestReached bool`;
  - `risk.Input.Wiring bool`;
  - `risk.Legend` ending in `· wiring (straight-line setup) at most 🟡`.

- [ ] **Step 1: Write the failing tests.**
  - `TestAssess_CrossPkgIsPartial`: HasProfile, Covered 0, Total 5, NonTestCallers 2, CrossPkgTestReached → Yellow. The same without the flag → Red.
  - `TestAssess_WiringCapsRed`: an untested wiring function with callers → Yellow. A wiring function, covered → Green. Partial → Yellow.
  - `TestAssess_WiringDoesNotCapExportedSignature`: Exported, Signature, Importable, Wiring → Red.
  - `TestLegend_MentionsWiring`: `strings.HasSuffix(Legend, "· wiring (straight-line setup) at most 🟡")`.
- [ ] **Step 2:** Run `go test ./internal/risk/`. Expected: FAIL.
- [ ] **Step 3:** Implement:
  - `CrossPkgTestReached` turns `untested` into `partial` when `HasProfile && Total > 0 && Covered == 0`.
  - After the switch, a `Red` result from any rule other than the exported-signature one becomes `Yellow` when `Wiring` is set.
- [ ] **Step 4:** Run `go test ./internal/risk/`. Expected: PASS, with all existing cases unchanged.
- [ ] **Step 5:** Commit `feat(risk): cross-package coverage is partial; wiring at most yellow`.

### Task 4: Pipeline + report: cross-package cover text, hint, wiring kind

**Files:**
- Modify: `internal/pipeline/pipeline.go` (per-function loop ~lines 215-265, `kindText` ~line 516); `internal/report/report.go` (`Model`: add `CrossPkgOnly int`; render the hint between the header and `writeRemovedCases`; the footer uses `risk.Legend`, unchanged)
- Create: `internal/pipeline/cover.go` with `coverText` extracted, plus `internal/pipeline/cover_test.go`
- Create: e2e fixtures `testdata/e2e/cross_pkg_coverage/{base,head,expected.golden.md,expected_callers.txt}` and `testdata/e2e/wiring/{…}`; register both in the `cases` table of `internal/pipeline/e2e_test.go` (`cross_pkg_coverage` with `profiles: []string{"coverage.out"}`)

**Interfaces:**
- Consumes: `Impact.TestPkgs` (Task 1), `ChangedFunc.Wiring` (Task 2), `risk.Input.CrossPkgTestReached` / `.Wiring` (Task 3).
- Produces:
  - `func crossPkgTests(fnPkg string, testPkgs []string) []string`: test packages other than `fnPkg`, with `_test` stripped for the comparison and full paths kept;
  - `func coverText(prof bool, covered, total int, graph bool, testReached bool, crossPkgs []string, mod string) string`;
  - `report.Model.CrossPkgOnly int`.

- [ ] **Step 1: Write the failing tests** in `cover_test.go`:
  - `TestCrossPkgTests`: fn `m/a`, tests `[m/a, m/a_test, m/b]` → `[m/b]`.
  - `TestCoverText_ExternalTestPackageIsSamePackage`: fn `m/a`, tests `[m/a_test]` → none, so the plain `0 % (0/3)`.
  - `TestCoverText_CrossPkg`: covered 0, total 3, crossPkgs `[m/b, m/c, m/d]`, mod `m` → `0 % in profile · reached by tests in b, c +1`.
  - Existing texts unchanged: `60 % (3/5)`, `no added lines · reached by test`, `not reached`, `n/a`.
- [ ] **Step 2:** Run `go test ./internal/pipeline/ -run 'TestCrossPkg|TestCoverText'`. Expected: FAIL.
- [ ] **Step 3:** Implement and wire into the loop:
  - set `in.CrossPkgTestReached` and `in.Wiring`;
  - count `m.CrossPkgOnly`;
  - `kindText` appends ` · wiring` (for `new`: `new · wiring`).
  - In `report.Render`, when `m.CrossPkgOnly > 0`, write the hint line from Global Constraints with N, followed by a blank line.
- [ ] **Step 4: Build the two fixtures.**
  - `cross_pkg_coverage`: head adds a line to `a.F` called by `cmd/main.go`, with the test only in `b/b_test.go`. `head/coverage.out` holds a same-package-only profile, where `a/a.go`'s blocks have count 0. Expected: 🟡, the cross-package cover cell and the hint line.
  - `wiring`: head changes `main.run` (wiring, untested, called by `main`), `lib.NewService` (wiring, in a non-main package) and `lib.Build` (has a `for`). Expected: the first two 🟡 `body · wiring`, and `lib.Build` 🔴 with no wiring tag.
  - Run `go test ./internal/pipeline/ -run 'TestE2E/(cross_pkg_coverage|wiring)$' -update` (these two cases only), then read both goldens and check them against the expectations above.
- [ ] **Step 5:** Run `go test ./internal/pipeline/ -run TestE2E` without `-update`. If any older golden now differs (expected: none unless a fixture has a wiring function), regenerate it, review the diff line by line, and say which rule changed it in the commit message. Then run `go test ./...`. Expected: PASS.
- [ ] **Step 6:** Commit `feat: cross-package coverage hint and wiring kind in the note`.

### Task 5: Grouping (pure)

**Files:**
- Create: `internal/pipeline/group.go`, `internal/pipeline/group_test.go`

**Interfaces:**
- Consumes: `report.FuncRow` (existing), `risk.Assess` / `risk.Input`.
- Produces:
  ```go
  // ratedFunc is one rated, non-deleted changed function as the pipeline built it.
  type ratedFunc struct {
      Row      report.FuncRow
      Key      string      // Display + "\x00" + package path: matches Caller.Display/Pkg
      New      bool        // symbols.New
      In       risk.Input  // the input its Row.Level came from
      Callers1 []string    // Keys of depth-1 non-test callers, registrants (Kind Registers) included
  }
  // group folds members into entries; rows keep input order for entries/others.
  func group(fs []ratedFunc, mod string) (rows []report.FuncRow, folded, groups int)
  ```
  `report.FuncRow` gains `Members []report.FuncRow` (non-nil only on a group row; Task 6 renders it).

- [ ] **Step 1: Write the failing tests** (build `ratedFunc` values by hand):
  - `TestGroup_MemberSharedGoesToSmallerEntry`: entries `p.A`, `p.B` both call member `p.h` → `h` under `p.A` only.
  - `TestGroup_MemberCalledByExistingCodeIsEntry`: `p.h`'s callers are `p.A` (new) and `p.Old` (not in fs) → `h` is its own row.
  - `TestGroup_CycleWithoutOutsideCaller`: `p.x` ↔ `p.y`, no other caller → one group under `p.x`, with `p.y` as member.
  - `TestGroup_EntryWithoutMembersUnchanged`: the row is identical to the input row, with `Members == nil`.
  - `TestGroup_RegistrationCountsAsCaller`: a member's only caller is a Registers edge from a new entry → folded.
  - `TestGroup_SumsAndUntested`: entry 10/10, members 5/10 and 0/4. The row's cover text reads `62 % (15/24) · 1 untested`, its kind `new · +2`, and its Level is `risk.Assess` over the sums with the entry's `NonTestCallers`.
  - `TestGroup_RatingFromSumsNotWorstMember` (Review Focus 3): a member alone would be Red, but the sums are partial → the group row is Yellow.
  - `TestGroup_PointerReceiverMethodIsMember` (Review Focus 4): member key `p.(*T).m\x00m/p`.
  - `TestGroup_NonNewNeverFolded`: a Body function whose only caller is new stays a row.
  - Return values: `folded` = member count, `groups` = entries with members.
- [ ] **Step 2:** Run `go test ./internal/pipeline/ -run TestGroup`. Expected: FAIL.
- [ ] **Step 3: Implement per spec §5.**
  - **Membership:** `New`, `len(Callers1) > 0`, and every key in `Callers1` is a New function in `fs`.
  - **Assignment:** a BFS from entries in `Display` byte order through member edges, first claim wins. Any unclaimed member, which can only be in a cycle, is assigned to the smallest `Display` of its connected unclaimed set, and that function is promoted to the entry.
  - **The group row's figures:**
    - Covered and Total are summed over the entry and its members.
    - When there is no profile, the cover text is `reached by test` if any member is reached, else `not reached`.
    - Level = `risk.Assess(sum input)`, where the sum input's `CrossPkgTestReached` is set when the summed Covered is 0, Total > 0, and any function in the group had `In.CrossPkgTestReached`.
    - Wiring is set only if all of them are wiring.
    - Members are sorted by `Display`.
- [ ] **Step 4:** Run `go test ./internal/pipeline/ -run TestGroup`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(pipeline): fold new-code subtrees into one row per entry`.

### Task 6: Groups in the pipeline and the note

**Files:**
- Modify: `internal/pipeline/pipeline.go` (build `ratedFunc` in the per-function loop; after it, when `graph != nil`, `m.Funcs, m.Folded, m.Groups = group(...)`), `internal/report/report.go` (`Model.Folded`, `Model.Groups int`; header; table; members `<details>` after the table; diagram skips members since they are not in `m.Funcs`), `README.md` ("What the note shows" — the three new behaviours; Legend)
- Create: e2e fixture `testdata/e2e/new_subtree/`, registered in `e2e_test.go` with `profiles: []string{"coverage.out"}`
- Test: `internal/report/report_test.go` (golden `internal/report/testdata/groups.golden.md`), `internal/pipeline/e2e_test.go`

**Interfaces:**
- Consumes: `group` (Task 5), `Model.CrossPkgOnly` (Task 4).
- Produces: the rendered note. The header reads `### 🔍 MR impact — <rows + folded> functions changed`, and the tally counts rows. When `Folded > 0`, the next line is `+<Folded> new functions folded into <Groups> groups`. Each group gets `<details><summary>X +N new</summary>` listing its members: rating, display, kind, cover, file.

- [ ] **Step 1: Write the failing tests.**
  - `TestRender_Groups` (report golden): a model with one group row (2 members, one untested) and one plain row. It pins the header count `3`, the tally `🔴 0 · 🟡 1 · 🟢 1` (example values), the fold line and the `<details>` block.
  - `TestRun_DegradedGraphNoGroups` (Review Focus 5), in `e2e_test.go` next to `TestTimeoutDegrades` and built the same way: run the `new_subtree` fixture with `Timeout: time.Nanosecond`. Assert `m.Degraded`, no row has `Members`, and `m.Folded == 0`.
- [ ] **Step 2:** Run `go test ./internal/report/ -run TestRender_Groups` and `go test ./internal/pipeline/ -run TestRun_DegradedGraphNoGroups`. Expected: FAIL.
- [ ] **Step 3:** Implement the wiring and rendering. Members carry no callers column in `<details>`; their file is shown instead. The diagram is unchanged in code, because members are not in `m.Funcs`.
- [ ] **Step 4: Build the `new_subtree` fixture.** Head adds `svc.New` (called from existing `main`) plus `svc.load`, `svc.price`, `svc.(*S).apply` and `svc.fmt`, which only call each other. Its `coverage.out` covers everything except `svc.fmt`. Generate the golden with `go test ./internal/pipeline/ -run 'TestE2E/new_subtree$' -update`, then read it. Expected: one `new · +4` row, a ` · 1 untested` suffix, the fold line and a `<details>` block listing 4 members.
- [ ] **Step 5:** Run the full suite `go test ./...` without `-update`. For every older golden that changed (fixtures with ≥ 2 new functions calling each other), regenerate it, review the diff line by line, and list the affected fixtures in the commit message. Update `README.md` ("What the note shows": a group row, the hint line, the wiring kind; plus the new Legend clause). Expected: PASS, and `go vet ./...` clean.
- [ ] **Step 6:** Commit `feat: render new-code groups; README for the signal fixes`.

### Task 7: Validate on a real repository

**Files:** none committed. This is a check of the finished branch.

- [ ] **Step 1:** Build `go build -o /tmp/mrimpact ./cmd/mrimpact`, then run it against any Go repository with a multi-package MR and a profile made **without** `-coverpkg`:

  ```
  go test ./... -coverprofile=/tmp/c.out
  /tmp/mrimpact --base <base> --no-post --coverprofile /tmp/c.out --out /tmp/a.md
  ```

  Expected:
  - every 🔴 row is a function no test reaches, or an exported signature change;
  - wiring rows show `· wiring`;
  - a new subtree is one row.

  Note the before and after red counts in the hand-off.
- [ ] **Step 2:** If a rule misfires on real code (a non-wiring function tagged wiring, a wrong fold), add a failing test reproducing it to the owning task's test file, fix it, and commit `fix: <rule> on <construct>`.
