# mrimpact — Architecture (sub-project C)

- **Date:** 2026-11-09
- **Status:** Draft, pending author review
- **Base:** `feat/reach-focus` at 5b7538a (sub-project B)
- **Follows:** `2026-11-06-mrimpact-design.md`, `2026-11-09-signal-fixes-design.md`, `2026-11-09-reach-focus-design.md`

## 1. Problem

AI-assisted changes often "work" by importing whatever is nearest: a domain package reaching into an HTTP adapter, one feature module importing another's internals. Line diffs don't show it, and reviewers notice it late, when it is already a pattern.

**Goal:** for every MR, show the imports it adds that break the project's architecture, how conformance moved (base → head), and how the touched packages' coupling metrics changed. It must work with no config, by inferring layers from conventional names and showing the result as hints, and it must be precise when a team declares its architecture.

Go forbids import cycles between packages, so package-level cycle detection is out of scope. Layer rules subsume the useful case: a cycle between groups of packages is a forbidden edge between their layers.

## 2. Constraints

- **Zero-config first.** With no `.mrimpact.yml` the inferred mode runs and is useful on its own. Declared rules are opt-in.
- **General Go.** Only import paths, directory names and the type system are used. Presets use names common across Go projects; nothing is project-specific.
- **Deterministic and offline.** No new dependency.
- **CLI:** `--fail-on` becomes a comma-separated list of `red` and `arch` (e.g. `red,arch`); `red` alone behaves exactly as today. Any other value is a usage error (exit 2). Other exit codes are unchanged.
- **Inferred results never affect ratings or exit codes.**

## 3. Model

### Packages and edges

- **Packages:** the module's analysed non-test package variants, at head and at base. Base uses the base load that already exists. Test files and external test packages are ignored.
- **Edges:** for each package P and each import Q that is a package of the same module, the edge P → Q, with the position of the import spec in head (file:line of the first file of P that imports Q, by file name order).
- **Edge identity** is the pair of package paths. A **new edge** exists at head and not at base. A package that only moved creates new edges, and that is accepted.

### Layers

A layer has a name, a set of directory names, optional globs, and an `isolated` flag. A package belongs to:

1. the layer whose **glob** matches its path (globs from the config, module-relative or full import path, with the `...` and `*` semantics of sub-project B), first matching layer in declaration order; else
2. the layer owning the **first** (shallowest) path segment, relative to the module root, that equals one of a layer's directory names. So `internal/adapters/http/model` is an adapter, and both `internal/domain/order` and the feature-first `internal/order/domain` are domain; else
3. no layer: **unclassified**. Unclassified packages are never guessed. Edges touching them are not judged.

**Entry layer**, in every mode:
- a package named `main`, or one whose path has a `cmd` segment, belongs to `entry`;
- `entry` may import any layer;
- nothing may import `entry`;
- `entry` takes precedence over directory names, but not over explicit globs.

### Rules

- A preset or config defines, for each layer, the layers it **may import**.
- A judged edge (both ends classified) **conforms** when its source and target are in the same layer, or the source may import the target's layer.
- **Isolated layer:** two different packages of it may import each other only when they share an isolation unit, or when one path is an ancestor directory of the other. The unit of a package classified by directory name is its module-relative path up to and including the matched segment plus one more segment if present (`internal/controller/http/x` → `internal/controller/http`; `internal/adapters/pg/sub` → `internal/adapters/pg`). Glob-classified packages have no unit (ancestor rule only). Two adapters (different units) may not import each other. *(Amended after validation on a real repository: the original sibling-package rule flagged controllers importing their shared `common` package.)*

**Conformance** = conforming judged edges ÷ all judged edges, as a percentage with one decimal, computed at base and at head. When no edge is judged, it is "n/a".

## 4. Presets

Each preset is a default layer map: directory names, plus the rules. `→` means "may import". Every layer may import itself, and `entry` may import everything.

| Preset | Layers (directory names) | Rules |
|---|---|---|
| `clean` | `entities` (entity, entities, model, models, domain); `usecases` (usecase, usecases, interactor, interactors, application, app); `adapters` (adapter, adapters, controller, controllers, presenter, presenters, gateway, gateways, repository, repositories, handler, handlers); `frameworks` (framework, frameworks, infrastructure, infra, driver, drivers, db, web, http, grpc) | usecases → entities; adapters → usecases, entities; frameworks → adapters, usecases, entities |
| `onion` | `domain` (domain, model, models, entity, entities); `domainservices` (domainservice, domainservices); `application` (application, app, usecase, usecases); `infrastructure` (infrastructure, infra, persistence, repository, repositories); `ui` (ui, api, web, http, grpc, handler, handlers, controller, controllers) | domainservices → domain; application → domainservices, domain; infrastructure → application, domainservices, domain; ui → application, domainservices, domain |
| `hexagonal` | `domain` (domain, core, model, models, entity, entities); `ports` (port, ports); `application` (application, app, usecase, usecases, service, services); `adapters` (adapter, adapters, infrastructure, infra, repository, repositories, storage, store, transport, http, grpc, handler, handlers, controller, controllers), isolated | ports → domain; application → ports, domain; adapters → application, ports, domain |
| `ddd` | `domain` (domain); `application` (application, app, usecase, usecases); `infrastructure` (infrastructure, infra, persistence); `interfaces` (interfaces, interface, api, http, grpc, web, handler, handlers, controller, controllers) | application → domain; infrastructure → application, domain; interfaces → application, domain |
| `layered` | `presentation` (presentation, ui, web, api, http, handler, handlers, controller, controllers); `service` (service, services, business, logic); `repository` (repository, repositories, dao, store, storage, persistence); `model` (model, models, entity, entities, domain) | presentation → service, repository, model; service → repository, model; repository → model |
| `mvc` | `controller` (controller, controllers, handler, handlers); `view` (view, views, template, templates); `model` (model, models) | controller → view, model; view → model |
| `pipeline` | one layer per entry of `stages` (in order, directory name = stage name, overridable by globs) | stage *i* → stages *j* with *j* < *i*; no stage imports a later one; each stage is isolated |
| `modular` | one layer per direct child directory of `modules_root` (default `internal`) | a package of module M may import, from another module N, only N's root package (`<root>/N`); inside M anything is allowed |

`pipeline` requires a `stages` list. `modular` takes `modules_root` (default `internal`). These two exist only when declared, never inferred.

### Inferred mode (no `architecture` block)

The map:
- `core` (domain, core, entity, entities, model, models);
- `ports` (port, ports);
- `application` (usecase, usecases, application, app, service, services);
- `adapters` (adapter, adapters, infrastructure, infra, repository, repositories, storage, store, persistence, transport, http, grpc, handler, handlers, controller, controllers, api, web, db);
- `entry`.

The rules: ports → core; application → ports, core; adapters → application, ports, core.

The mode is labelled *inferred*, and when it runs the note lists the layer each touched package was mapped to.

## 5. Martin metrics

These are computed per package from the module-internal edges, at base and at head:

- **Ca** (afferent coupling): the number of module packages that import it.
- **Ce** (efferent coupling): the number of module packages it imports.
- **I** (instability) = Ce / (Ca + Ce), and 0 when both are 0.
- **A** (abstractness) = interface types ÷ named types declared in the package (exported or not), and 0 when there are no named types.
- **D** (distance from the main sequence) = |A + I − 1|.

They are reported only for **touched packages** (those containing a changed non-test file), and only when Ca, Ce, A or D changed. At most 10 are shown, sorted by |ΔD| descending, then path. Values use two decimals.

## 6. The note

A section after the 🔐 section and before the table:

```
🏛 **Architecture** (hexagonal) · conformance 96.0 % → 95.2 % · 214/225 packages classified
- ❌ new: `domain → adapters` — `internal/domain/order/order.go:7` imports `internal/adapters/pg` (+3 violations already in base)
- ❌ new: `adapters → adapters (isolated)` — `internal/adapters/http/h.go:5` imports `internal/adapters/pg`
<details><summary>Coupling (3 packages changed)</summary>

| Package | Ca | Ce | I | A | D |
|---|---|---|---|---|---|
| `internal/domain/order` | 4 → 4 | 1 → 2 | 0.20 → 0.33 | 0.25 → 0.25 | 0.55 → 0.42 |
</details>
```

- **Header in inferred mode:** `🏛 **Architecture (inferred — hints, not enforced)**`, followed by `· layers: <pkg> → <layer>, …` for touched packages, at most 10.
- **New violations:**
  - up to 15 lines, then `… and N more`;
  - sorted by file then line;
  - each line shows the source layer, the target layer, `(isolated)` when the isolation rule was broken, and the import's position.
- **Existing violations:** shown only as the `+N violations already in base` count, on the first line.
- **When the section appears:** only when there is a new violation, a changed conformance, or a changed metric. Otherwise it is omitted.
- **Degraded mode:** the base revision failing to load does not stop the run. The section then shows head conformance only, plus `(base not loaded: new violations not computed)`, and lists no violations.

## 7. Configuration

Added to `.mrimpact.yml` (all optional, strict like the rest):

```yaml
architecture:
  preset: hexagonal               # clean | onion | hexagonal | ddd | layered | mvc | pipeline | modular
  layers:                         # globs per layer name; adds globs to a preset layer, or defines a new layer
    domain: [internal/core/...]
  allow: ["adapters -> ports"]    # extra edges "<layer> -> <layer>"
  isolated: [adapters]            # extra isolated layers
  stages: [ingest, transform, load]   # pipeline only (required there)
  modules_root: internal          # modular only
```

- **No preset, but `layers` given:** a custom architecture made of exactly those layers. Rules come from `allow` (a layer may import only itself and what `allow` lists), and the entry rule still applies.
- **Strict validation (exit 2, one stderr line):**
  - an unknown preset;
  - `allow` naming an unknown layer, or malformed (`a->b`, extra arrows);
  - `stages` without `preset: pipeline`, or `pipeline` without `stages`;
  - `modules_root` without `preset: modular`;
  - a duplicate layer name.
- **Visibility:** the effective architecture appears in the config line from sub-project B (`⚙️ .mrimpact.yml: … · architecture hexagonal`). The "changed in this MR" rule applies to the `architecture` block too.

## 8. Gate

- `--fail-on arch` exits 1 when the architecture is **declared** (a preset or custom layers) and the MR adds at least one new violation.
- Inferred mode never fails.
- With `--fail-on red,arch`, either condition fails the job. Exit 1, as today.

## 9. Components

| Package | Change |
|---|---|
| `internal/arch` (new) | `Edge`, `Layer`, `Model`; presets; inferred map; `Classify`; `Judge`; `Conformance`; `Metrics`; `Diff(base, head)` (new violations, counts) |
| `internal/load` | export the module-internal import edges and per-package named/interface type counts for a package set (head and base) |
| `internal/pipeline` | build edges for head and base (base from the existing base load); run `arch`; fill `Model.Arch` |
| `internal/config` | the `architecture` block and its validation |
| `internal/report` | 🏛 section |
| `cmd/mrimpact` | `--fail-on` list parsing; the `arch` gate |

## 10. Testing

- **`arch`, per preset** (table-driven, over synthetic package paths and edges):
  - an allowed edge;
  - a forbidden edge;
  - a same-layer edge;
  - an isolated sibling edge (forbidden) and an isolated parent/child edge (allowed);
  - entry importing everything, and nothing importing entry;
  - an unclassified package not judged;
  - shallowest-segment classification (`internal/adapters/http/model` → `adapters`, not `domain`; `internal/order/domain` → `domain`);
  - globs overriding names.
- **`pipeline`:** an earlier stage is allowed; a later stage and siblings are forbidden.
- **`modular`:** importing another module's root is allowed; importing its subpackage is forbidden; the same module is free.
- **Conformance and Metrics:** arithmetic, including the zero cases (no edges, no named types).
- **`Diff`:** a new violation listed; an existing one counted, not listed; a moved package.
- **`config`:**
  - every validation error from §7;
  - a custom layers + allow model;
  - a preset + overrides.
- **CLI:**
  - `--fail-on red,arch` parsing;
  - `--fail-on bogus` → exit 2;
  - a declared violation → exit 1;
  - an inferred-only violation → exit 0.
- **E2E fixtures:**
  - `arch_inferred`: no config, a domain package importing a repository → the inferred section;
  - `arch_hexagonal`: declared preset, an adapter importing another adapter;
  - `arch_modular`: a cross-module subpackage import.
- **Existing goldens:** regenerated only where the new section appears, and each is reviewed.

## 11. Out of scope

Function-level dependency rules; cyclomatic metrics (sub-project D); auto-detecting the preset; any visualisation beyond the note; cross-module (go.work) graphs.
