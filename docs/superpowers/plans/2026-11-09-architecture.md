# Architecture (sub-project C) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** For every MR, report the imports it adds that break the project's architecture, conformance base → head, and the coupling metrics of the packages it touches. Layers are inferred (as hints) with no config and enforced when declared, with an opt-in `--fail-on arch` gate.

**Architecture:** A new package, `internal/arch`, holds:
- the layer model, the presets and the inferred map;
- classification, judging, conformance, Martin metrics and the base/head diff.

It builds its import graph from `go/packages` results. The pipeline feeds it the head program and the existing base load. `config` gains an order-preserving `architecture` block, `report` gains the 🏛 section, and `cmd/mrimpact` turns `--fail-on` into a list with an `arch` gate.

**Tech Stack:** Go 1.26, `golang.org/x/tools/go/packages` (existing), `gopkg.in/yaml.v3` (existing). No new dependency.

**Spec:** `docs/superpowers/specs/2026-11-09-architecture-design.md`

## Global Constraints

- Zero-config: with no `architecture` block, the inferred model runs. Inferred results never affect ratings or exit codes.
- General Go:
  - only import paths, directory names and type counts are used;
  - presets use exactly the directory names in spec §4;
  - every rule is the same for every project.
- Deterministic and offline; no new dependency.
- Packages: non-test package variants of the module only. Edges are module-internal imports.
- **Classification order:**
  1. config globs, first layer in declaration order;
  2. else the `entry` layer (package `main`, or a `cmd` path segment);
  3. else the layer owning the **first (shallowest)** module-relative path segment that equals one of its directory names;
  4. else unclassified, which is never judged.
- **Entry rule:** `entry` may import anything, and nothing may import `entry`.
- **Isolated rule:** two different packages of an isolated layer may import each other only when one path is an ancestor directory of the other.
- **Conformance** = conforming judged edges ÷ judged edges × 100, rendered with 1 decimal and ` %`, or `n/a` when nothing is judged.
- **Metrics:** I = Ce/(Ca+Ce) (0 when both are 0); A = interfaces ÷ named types (0 when there are none); D = |A+I−1|.
  - Rendered with 2 decimals.
  - Shown only for touched packages whose Ca, Ce, A or D changed.
  - At most 10, sorted by |ΔD| descending, then path.
- **Copy (verbatim from spec §6):**
  - **Headers:** `🏛 **Architecture** (<preset or custom>) · conformance <base> → <head> · <n>/<m> packages classified`. Inferred: `🏛 **Architecture (inferred — hints, not enforced)**`, with ` · layers: <pkg> → <layer>, …` for up to 10 touched packages.
  - **Violation line:** `- ❌ new: \`<from> → <to>\` — \`<file>:<line>\` imports \`<pkg>\``. Add ` (isolated)` after the layer pair when the isolation rule was broken. The first line carries ` (+N violations already in base)` when N > 0.
  - **Limits:** at most 15 violation lines, then `… and N more`.
  - **Coupling block:** `<details><summary>Coupling (<n> packages changed)</summary>` holding the table `| Package | Ca | Ce | I | A | D |`, with cells `<base> → <head>`.
  - **No base:** `(base not loaded: new violations not computed)`.
- `--fail-on` is a comma list of `red` and `arch`. Any other value → exit 2. `arch` exits 1 only for declared models with ≥ 1 new violation.
- Existing goldens change only where the 🏛 section appears, and each changed golden is reviewed line by line.
- Commit trailer, both lines:
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`
  `Claude-Session: https://claude.ai/code/session_01MVcztFMUkPoWcB4CENF3Gx`

## Review Focus

1. **A package matching layer names at two depths** (`internal/adapters/http/model`): it must be `adapters`, never `domain`. Test: Task 1 `TestClassify_ShallowestSegmentWins`.
2. **Layer directory names that also appear as ordinary words in a module path** (module `example.com/app`, package `example.com/app/internal/x`): segments are matched relative to the module root, so the module's own `app` segment never classifies anything. Test: Task 1 `TestClassify_ModuleSegmentsIgnored`.
3. **The same import appearing in several files of a package**: one edge, positioned at the first file in name order. Test: Task 3 `TestFromPackages_OneEdgePerPair`.
4. **A base revision that fails to load**: the run continues, the section shows head conformance and `(base not loaded: …)`, and `--fail-on arch` does not fail (nothing new can be computed). Test: Task 5 `TestRun_ArchBaseNotLoaded`.
5. **A config with `layers` listed in an order where two globs overlap**: the first declared wins, and the order must survive YAML decoding. Test: Task 4 `TestParse_LayerOrderPreserved`.

---

### Task 1: `internal/arch` model, classification, judging, conformance

**Files:**
- Create: `internal/arch/model.go`, `internal/arch/judge.go`, `internal/arch/arch_test.go`

**Interfaces:**
- Produces:
  ```go
  type Edge struct{ From, To, File string; Line int }       // package paths; File repo-relative
  type Graph struct {
      Module     string
      Packages   []string          // sorted package paths
      Edges      []Edge            // sorted by From, To
      Named      map[string]int    // named types declared per package
      Interfaces map[string]int    // interface types declared per package
  }
  type Kind int // Layers, Pipeline, Modular
  type Layer struct {
      Name      string
      Dirs      []string // directory names
      Globs     []string // module-relative or full import path; "/..." and "*" like sensitive.Critical
      Isolated  bool
      MayImport []string // layer names
  }
  type Model struct {
      Name        string // preset name, "custom", or "inferred"
      Inferred    bool
      Kind        Kind
      Layers      []Layer // declaration order = glob precedence
      ModulesRoot string  // Modular
  }
  const EntryLayer = "entry"
  func (m Model) Classify(module, pkgPath string) (layer string, ok bool)
  type Violation struct{ Edge Edge; FromLayer, ToLayer string; Isolated bool }
  func (m Model) Judge(g Graph) (judged int, violations []Violation) // violations sorted by Edge.File, Edge.Line, Edge.To
  func Conformance(judged, violating int) (pct float64, ok bool)    // ok=false when judged == 0
  ```
  Glob matching reuses `sensitive.Critical(pkgPath, module, globs)`. For `Kind == Modular`, `Classify` returns the module name: the first segment under `ModulesRoot`. `Judge` allows a cross-module edge only when `To == module + "/" + ModulesRoot + "/" + <its module>`; any edge inside one module is allowed.

- [ ] **Step 1: Write failing tests** over a hand-built custom model and synthetic graphs:
  - `TestClassify_ShallowestSegmentWins` (Review Focus 1): with layers `domain{model}` and `adapters{adapters}`, `m/internal/adapters/http/model` → `adapters`.
  - `TestClassify_ModuleSegmentsIgnored` (Review Focus 2): module `example.com/app`; layer `application{app}`; `example.com/app/internal/x` → unclassified.
  - `TestClassify_GlobBeatsNames`, `TestClassify_FirstGlobInOrder`.
  - `TestClassify_Entry`:
    - `m/cmd/server` → `entry`;
    - a package named `main` → `entry` (pass the name through a `Graph`-independent helper `ClassifyNamed(module, path, pkgName)`, which `Classify` calls with `""`);
    - a glob overriding entry → that layer.
  - `TestJudge_AllowedForbiddenSameLayer`.
  - `TestJudge_EntryRules`: entry → anything allowed; anything → entry is a violation.
  - `TestJudge_Isolated`: sibling `m/adapters/pg` → `m/adapters/http` is a violation flagged `Isolated`; parent/child `m/adapters` → `m/adapters/pg` is allowed.
  - `TestJudge_UnclassifiedNotJudged`: `judged` excludes edges with an unclassified end.
  - `TestJudge_Modular`: root import allowed; subpackage import forbidden; same module free.
  - `TestConformance`: (10, 1) → 90.0, true; (0, 0) → false.
- [ ] **Step 2:** Run `go test ./internal/arch/`. Expected: FAIL.
- [ ] **Step 3:** Implement. Classification segments are `strings.Split(strings.TrimPrefix(pkgPath, module+"/"), "/")`. A `pkgPath == module` package has no segments.
- [ ] **Step 4:** Run `go test ./internal/arch/ && go vet ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(arch): layer model, classification, judging, conformance`.

### Task 2: Presets and the inferred map

**Files:**
- Create: `internal/arch/presets.go`, `internal/arch/presets_test.go`

**Interfaces:**
- Consumes: Task 1 types.
- Produces:
  ```go
  func Preset(name string) (Model, bool) // clean, onion, hexagonal, ddd, layered, mvc; pipeline/modular are built by NewPipeline/NewModular
  func NewPipeline(stages []string) Model // stage i MayImport stages < i; every stage Isolated; Dirs = {stage name}
  func NewModular(root string) Model      // root "" → "internal"
  func Inferred() Model                   // Name "inferred", Inferred true
  func PresetNames() []string             // the 8 names in spec order, for validation messages
  ```
  The directory names and rules are copied verbatim from spec §4's table and "Inferred mode" paragraph.

- [ ] **Step 1: Write failing tests**, one table per preset with:
  - one allowed edge;
  - one forbidden edge, using the preset's own directory names (e.g. hexagonal `m/internal/domain/order` → `m/internal/adapters/pg` is forbidden; `m/internal/adapters/pg` → `m/internal/ports` is allowed);
  - `TestHexagonal_AdaptersIsolated`;
  - `TestPipeline_Order`: stages `[ingest, transform, load]`; `load` → `transform` allowed; `transform` → `load` forbidden; sibling packages within `transform` forbidden;
  - `TestInferred_Map`: `m/internal/service/x` → `application`; `m/internal/repository/pg` → `adapters`; `m/internal/ports` → `ports`; `m/internal/util` → unclassified; a domain → repository edge is a violation;
  - `TestPresetNames`: equals `[clean onion hexagonal ddd layered mvc pipeline modular]`.
- [ ] **Step 2:** Run `go test ./internal/arch/`. Expected: FAIL.
- [ ] **Step 3:** Implement as data tables.
- [ ] **Step 4:** Run `go test ./internal/arch/`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(arch): presets and the inferred layer map`.

### Task 3: Graph from packages, metrics, base/head analysis

**Files:**
- Create: `internal/arch/graph.go`, `internal/arch/metrics.go`, `internal/arch/analyze.go`, tests `internal/arch/graph_test.go`, `internal/arch/analyze_test.go`, fixture module `internal/arch/testdata/graph/`

**Interfaces:**
- Consumes: Tasks 1–2.
- Produces:
  ```go
  // FromPackages builds the module-internal import graph of the non-test packages; rel maps an
  // absolute file to its repo-relative path (load's rel); one edge per (From, To) at the first
  // file in name order importing To, at that import spec's line.
  func FromPackages(pkgs []*packages.Package, module string, rel func(string) (string, bool)) Graph
  type Metric struct{ Ca, Ce int; I, A, D float64 }
  func Metrics(g Graph) map[string]Metric
  type MetricDelta struct{ Pkg string; Base, Head Metric }
  type Result struct {
      Model                string
      Inferred             bool
      Classified, Total    int            // head
      BaseConf, HeadConf   *float64       // nil = n/a (or base missing)
      New                  []Violation    // head violations whose (From, To) edge is not in base
      Existing             int            // head violations whose edge is in base
      BaseMissing          bool
      Coupling             []MetricDelta  // touched packages with a change, sorted per Global Constraints, ≤ 10
      TouchedLayers        []PkgLayer     // inferred header: touched pkg → layer, sorted by pkg, ≤ 10
  }
  type PkgLayer struct{ Pkg, Layer string }
  func Analyze(m Model, base *Graph, head Graph, touched []string) Result // base nil → BaseMissing
  ```
  Test variants are skipped (`strings.HasSuffix(p.ID, ".test]")` or a name ending `_test`). `Named` counts `*types.TypeName` in package scope. `Interfaces` counts those whose underlying type is `*types.Interface`.

- [ ] **Step 1: Write failing tests:**
  - `TestFromPackages_OneEdgePerPair` (Review Focus 3): fixture `example.com/g`, where `a/x.go` and `a/y.go` both import `example.com/g/b`, plus `a/a_test.go` imports `example.com/g/c` → exactly one edge `a → b`, at `a/x.go` and its import line, and no edge to `c`.
  - `TestFromPackages_TypeCounts`: a package with 2 interfaces and 2 structs → Named 4, Interfaces 2.
  - `TestMetrics`: a hand-built graph where `x` is imported by 3 and imports 1 → Ca 3, Ce 1, I 0.25. With A = 0.5, D = 0.25. An isolated package → I 0, D 1.
  - `TestAnalyze_NewVsExisting`: a violation present in both base and head is counted in `Existing`; one only in head is listed in `New`.
  - `TestAnalyze_BaseMissing`: base nil → `BaseMissing`, `BaseConf == nil`, `New == nil`.
  - `TestAnalyze_CouplingOnlyChangedTouched`, plus sort and cap.
- [ ] **Step 2:** Run `go test ./internal/arch/`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `go test ./internal/arch/ && go vet ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(arch): import graph, Martin metrics, base/head analysis`.

### Task 4: Config `architecture` block

**Files:**
- Modify: `internal/config/config.go` (the `architecture` key in `fileShape`, decoded order-preserving; validation)
- Create: `internal/arch/fromconfig.go` (builds the model, validates names)
- Test: `internal/config/config_test.go`, `internal/arch/fromconfig_test.go`

**Interfaces:**
- Produces:
  ```go
  // config
  type LayerGlobs struct{ Name string; Globs []string }
  type Architecture struct {
      Set         bool   // the block exists
      Preset      string
      Layers      []LayerGlobs // file order
      Allow       []string     // "<a> -> <b>"
      Isolated    []string
      Stages      []string
      ModulesRoot string
  }
  // Config gains: Architecture Architecture

  // arch
  // FromConfig returns Inferred() when !a.Set. Errors wrap config.ErrConfig.
  func FromConfig(a config.Architecture) (Model, error)
  ```
  `config.Parse` validates shape:
  - the preset is one of `arch.PresetNames()` — to avoid an import cycle, config keeps its own list, and a test asserts it equals `arch.PresetNames()`;
  - `allow` entries match `^\s*[A-Za-z0-9_-]+\s*->\s*[A-Za-z0-9_-]+\s*$`;
  - `stages` appears only with `pipeline`, and `pipeline` requires `stages`;
  - `modules_root` appears only with `modular`;
  - no duplicate layer names;
  - each glob passes the existing glob check.

  `FromConfig` validates names: `allow` and `isolated` must name a known layer (a preset layer, a configured layer, or `entry`), and globs are added to the matching layer. Without a preset, a custom model is built from `layers`, with rules from `allow` only.

- [ ] **Step 1: Write failing tests:**
  - `TestParse_LayerOrderPreserved` (Review Focus 5): `layers: {zeta: [a/...], alpha: [a/b/...]}` keeps `[zeta, alpha]`.
  - Each validation error from spec §7 → `errors.Is(err, ErrConfig)`, with the offending value in the message.
  - `TestFromConfig_PresetOverride`: hexagonal plus `layers: {domain: [internal/core/...]}` → `m/internal/core/x` is domain.
  - `TestFromConfig_Custom`: layers `a`, `b`; `allow: ["b -> a"]` → b → a allowed, a → b forbidden.
  - `TestFromConfig_UnknownLayerInAllow` → error.
  - `TestFromConfig_NotSetIsInferred`.
- [ ] **Step 2:** Run `go test ./internal/config/ ./internal/arch/`. Expected: FAIL.
- [ ] **Step 3:** Implement. Decode `layers` via a `yaml.Node` (mapping node, in order), keeping `KnownFields(true)` for the rest.
- [ ] **Step 4:** Run `go test ./internal/config/ ./internal/arch/ && go vet ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(config): architecture block, order-preserving and strict`.

### Task 5: Pipeline and the 🏛 section

**Files:**
- Modify:
  - `internal/pipeline/pipeline.go`: `Config.Arch *arch.Model` (nil → `arch.Inferred()`); the head graph from `prog.Packages`; the base graph inside `loadBase`, as a new `baseResult.graph *arch.Graph`; touched packages from the changed non-test files; `m.Arch = arch.Analyze(...)`;
  - `internal/report/report.go`: `Model.Arch *arch.Result`; `writeArch`, placed after the 🔐 section and before the table;
  - `README.md`: the architecture section (zero-config hints first, then presets and config).
- Create: e2e fixtures `testdata/e2e/arch_inferred/`, `testdata/e2e/arch_hexagonal/` (`head/.mrimpact.yml` with `architecture: {preset: hexagonal}`), `testdata/e2e/arch_modular/`
- Test: `internal/report/report_test.go`, `internal/pipeline/e2e_test.go`

**Interfaces:**
- Consumes: Tasks 1–4.
- Produces: `report.Model.Arch`. The section is rendered only when `len(New) > 0`, or `BaseConf != HeadConf` (by value), or `len(Coupling) > 0`.

- [ ] **Step 1: Write failing tests.**
  - `TestRender_ArchDeclared` pins the exact copy: header, 2 violation lines (one isolated) with `(+3 violations already in base)`, and the coupling details block.
  - `TestRender_ArchInferred`: header plus the layers list.
  - `TestRender_ArchCap`: 17 violations → 15 lines and `… and 2 more`.
  - `TestRender_ArchOmittedWhenNothing`.
  - `TestRun_ArchBaseNotLoaded` (Review Focus 4): force the base load to fail (follow how existing e2e tests make the base unloadable, or inject via a test hook already used for base failures). Expect `Arch.BaseMissing`, and the rendered `(base not loaded: new violations not computed)`.
  - **E2E:**
    - `arch_inferred`: no config; `internal/domain/order` adds an import of `internal/repository/pg` → inferred header, one new violation;
    - `arch_hexagonal`: `internal/adapters/http` adds an import of `internal/adapters/pg` → `(isolated)`;
    - `arch_modular`: config `{preset: modular}`; `internal/billing/x` adds an import of `internal/orders/store` → violation, while `internal/orders` (root) stays allowed.
- [ ] **Step 2:** Run the new tests. Expected: FAIL.
- [ ] **Step 3:** Implement. Build the model in the pipeline caller: `cmd/mrimpact` passes `arch.FromConfig(cfg.Architecture)`, and its errors go through the existing exit-2 path, because they wrap `ErrConfig`. The e2e harness loads config the way main does.
- [ ] **Step 4:** Generate the three new goldens with `-update` for those cases only, then read them. Then run `go test ./...` without `-update`.
  - Any older golden that changes must be explained by the 🏛 section: a fixture whose MR adds a classified edge, or whose touched package's metrics changed. Read each one, and list the changes in the commit message.
  - If an older golden shows the section only because the inferred map classifies its paths, check that the change is genuine and keep it.
  - Run `go vet ./...` and `gofmt -l .`.
- [ ] **Step 5:** Commit `feat: architecture section — inferred hints, declared rules, coupling`.

### Task 6: `--fail-on` list, `arch` gate, config line

**Files:**
- Modify:
  - `cmd/mrimpact/main.go`: parse `--fail-on` as a comma list over `{red, arch}`; empty items are ignored; anything else is a usage error, exit 2. Keep `red` exactly as today. `arch` → exit 1 when `!m.Arch.Inferred && len(m.Arch.New) > 0`;
  - `internal/report/report.go`: the sub-project B config line gains ` · architecture <name>` when the block is set; the "changed in this MR" diff includes the `architecture` block (extend `config.Changes`);
  - `README.md`: `--fail-on` docs.
- Test: `cmd/mrimpact/main_test.go`, `internal/config/config_test.go`, `internal/report/report_test.go`

**Interfaces:**
- Consumes: `report.Model.Arch`, `config.Changes`.

- [ ] **Step 1: Write failing tests:**
  - `TestMain_FailOnList`: `red,arch` accepted; `arch` alone accepted; ` red , ` accepted; `bogus` → exit 2 with a stderr message naming `bogus`.
  - `TestMain_FailOnArchDeclared`: the `arch_hexagonal` fixture repo with `--fail-on arch` → exit 1.
  - `TestMain_FailOnArchInferredPasses`: the `arch_inferred` fixture with `--fail-on arch` → exit 0.
  - `TestMain_FailOnRedUnchanged`: the existing red-fixture behaviour.
  - `TestRender_ConfigLineArchitecture` and `TestChanges_ArchitectureBlock`.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `go test ./...`, `go vet ./...` and `gofmt -l .`. Expected: PASS. Only goldens with a config line change.
- [ ] **Step 5:** Commit `feat(cli): --fail-on list with an arch gate; config line shows the architecture`.

### Task 7: Validate on real repositories

**Files:** none committed unless a misfire is found.

- [ ] **Step 1:** Build, then run with no config on the two real branches used before (same coverprofiles). Expected:
  - the inferred section appears only if those MRs add an inner → outer import, or change a touched package's metrics;
  - every listed violation is a real import at the stated `file:line`;
  - the layer map for touched packages matches the directory names.

  Then run once with a temporary `--config` declaring `hexagonal` (outside both repos, never committed into them) to see the declared rendering. Note the counts in the hand-off.
- [ ] **Step 2:** If a rule misfires on real code (a wrong layer, a false violation, a wrong metric), add a failing test reproducing it to the owning task's test file, fix it, and commit `fix(arch): <rule> on <construct>`.
