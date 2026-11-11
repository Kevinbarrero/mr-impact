# mrimpact reference

The full reference for `mrimpact`: flags, configuration, the architecture rules, CI integration on GitHub and GitLab, exit codes and every section of the note. For a quick start, see the [README](../README.md).

## Local usage

```sh
mrimpact --base origin/main
```

Prints the note to stdout and writes it to `mrimpact-report.md`. Nothing is posted unless it runs in a GitLab merge request pipeline (the GitLab CI variables are present) or in a GitHub Actions `pull_request` workflow (see [GitHub mode](#github-mode)). Run it from the module root (or pass `--dir`), with the head revision checked out.

Flags: `--dir`, `--base` (env `CI_MERGE_REQUEST_DIFF_BASE_SHA`, or the pull request's base on GitHub), `--head HEAD`, `--depth 2`, `--coverprofile` (repeatable glob), `--tags a,b`, `--timeout 5m`, `--fail-on red,arch`, `--strict`, `--skip-empty`, `--no-post`, `--out mrimpact-report.md`, `--config <path>`, `--version`.

| Flag | Default | Meaning |
|---|---|---|
| `--dir` | `.` | repository root (module root), checked out at `--head` |
| `--base` | env `CI_MERGE_REQUEST_DIFF_BASE_SHA`, else the GitHub pull request's `base.sha` | base revision; the explicit flag wins over the GitLab variable, which wins over the GitHub event. Required in local mode and on a GitHub push event |
| `--head` | `HEAD` | head revision |
| `--depth` | `2` | caller depth (at least 1) |
| `--coverprofile` | none | coverage profile glob; repeatable |
| `--tags` | none | comma-separated build tags |
| `--timeout` | `5m` | call graph time budget (greater than 0) |
| `--fail-on` | none | comma list of gates that exit 1: `red` (a red function exists), `arch` (a new violation of the declared architecture) |
| `--strict` | off | exit 2 on an internal or post failure |
| `--skip-empty` | off | do not post when there are no Go changes |
| `--no-post` | off | shadow mode: never post (see [Shadow mode](#shadow-mode)) |
| `--out` | `mrimpact-report.md` | write the note to this file |
| `--config` | `.mrimpact.yml` in `--dir`, optional | config file |
| `--version` | | print `mrimpact <version>` and exit 0 |

The diff is taken against the merge base of `--base` and `--head`, so the base may be the target branch tip.

## Configuration

No file is needed. With no config file and no flag, every check runs on its built-in defaults, including the sensitive-API table below. A missing `.mrimpact.yml` is the normal case: nothing is printed and nothing is added to the note (unless the MR deleted one: see below).

To add project knowledge, put an optional `.mrimpact.yml` at the module root (`--dir`), or name another file with `--config <path>`:

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

- Every key is optional; an empty or comments-only file equals no file.
- Globs: `internal/x/...` matches the package and everything below it; `internal/x/*` matches one path segment (`path.Match`); a glob without the module path is joined to it. Globs are cleaned (`path.Clean`; a leading `./` and a trailing `/` are accepted). A glob that matches no loaded package is listed under the note's limits: `critical glob "<g>" matches no package`.
- Patterns: `pkgpath.Name`, `pkgpath.*`, `pkgpath.(*T).M` or `pkgpath.(T).M`; `*` in the method position matches any method, a trailing `*` a prefix (`Execute*`); `pkgpath/...` matches the package and every package below it. Callees are matched by their type-checked package path and name, so aliased imports and methods called through the type are found.
- The note states the config when it matters: whenever it removes defaults or names `critical` globs, one line under the header reads `⚙️ .mrimpact.yml: critical … · removes … · adds …`. When the MR changed the file (compared with its base version; a missing base file is the empty config), the line ends ` — changed in this MR (MR removes from defaults: os/exec.*)`, listing what the MR added or dropped, or `(base config unreadable)` when the base version does not parse; the base file never fails the run.
- Parsing is strict. An unknown key, a wrong type, a bad glob, a malformed pattern, a `remove` entry that is not in the default table, or `--config` naming a missing file is a usage error: one line `mrimpact: config: …` on stderr, exit 2, and no note is written or posted.

Default sensitive table (the standard library, plus a few common data-access and crypto libraries, matched by import path; routers such as gin, echo or chi are not in it):

| Area | Patterns |
|---|---|
| Process / OS | `os/exec.*`, `syscall.*`, `unsafe.*`, `plugin.*`, `os.(*Process).*`, `os.StartProcess` |
| Files | `os.Remove`, `os.RemoveAll`, `os.WriteFile`, `os.Create`, `os.OpenFile`, `os.Chmod`, `os.Chown`, `os.Rename`, `os.Symlink`, `os.Link` |
| Reflection | `reflect.(Value).Set*`, `reflect.(Value).Send`, `reflect.(Value).TrySend`, `reflect.(Value).Clear`, `reflect.Copy`, `reflect.Swapper`, `reflect.NewAt`, `reflect.(Value).UnsafePointer`, `reflect.(Value).UnsafeAddr`, `reflect.(Value).Pointer`, `reflect.(Value).Call*`, `reflect.(Value).Method`, `reflect.(Value).MethodByName`, `reflect.(Type).MethodByName`, `reflect.MakeFunc` (mutating, unsafe and dynamic-dispatch APIs; read-only `TypeOf`, `ValueOf`, `DeepEqual`, `Kind`, `Interface` are not sensitive) |
| Crypto / TLS | `crypto/...`, `crypto/tls.Config` (a composite literal counts) |
| SQL | `database/sql.(*DB).*`, `database/sql.(*Tx).*`, `database/sql.(*Conn).*`, `database/sql.(*Stmt).*` |
| HTTP | `net/http.ListenAndServe`, `net/http.ListenAndServeTLS`, `net/http.(*Server).*`, `net/http.(*Client).*`, `net/http.Get`, `net/http.Post`, `net/http.PostForm`, `net/http.Head`, `net/http.Handle`, `net/http.HandleFunc` |
| Data access / crypto libraries | `github.com/jackc/pgx/v5.(*Conn).*`, `github.com/jackc/pgx/v5.(Tx).*`, `github.com/jackc/pgx/v5/pgxpool.(*Pool).*`, `github.com/jackc/pgx/v5/pgxpool.(*Conn).*`, `github.com/jackc/pgx/v5/pgxpool.(*Tx).*`, the same for `github.com/jackc/pgx/v4`, `github.com/jmoiron/sqlx.(*DB).*`, `github.com/jmoiron/sqlx.(*Tx).*`, `gorm.io/gorm.(*DB).*`, `golang.org/x/crypto/...` |
| Deserialization / templates | `encoding/gob.*`, `encoding/xml.(*Decoder).*`, `encoding/xml.Unmarshal`, `text/template.(*Template).Execute*`, `html/template.(*Template).Execute*` |

## Architecture

mrimpact judges the module's package import graph against layer rules and reports what the MR changed in it, in a 🏛 section of the note.

**Zero config: hints.** With no `architecture` block, an inferred layer map runs. A package belongs to the layer owning the shallowest module-relative path segment that matches one of its directory names: `core` (domain, core, entity, entities, model, models), `ports` (port, ports), `application` (usecase, usecases, application, app, service, services), `adapters` (adapter, adapters, infrastructure, infra, repository, repositories, storage, store, persistence, transport, http, grpc, handler, handlers, controller, controllers, api, web, db). The rules: ports → core; application → ports, core; adapters → application, ports, core; every layer may import itself. A package named `main`, or one with a `cmd` segment, is `entry`: it may import anything and nothing may import it. A package that matches nothing is unclassified and never judged. The section is labelled *inferred — hints, not enforced* and lists the layer each touched package was mapped to; inferred results never change a rating or the exit code.

Only import paths and directory names are used. Edges are module-internal imports of non-test packages (test files and external test packages are ignored), one per package pair, positioned at the first file in name order that imports the target.

**Presets.** Declare one to enforce its rules (`→` = may import):

| Preset | Layers | Rules |
|---|---|---|
| `clean` | entities, usecases, adapters, frameworks | usecases → entities; adapters → usecases, entities; frameworks → adapters, usecases, entities |
| `onion` | domain, domainservices, application, infrastructure, ui | each outer layer → the inner ones (ui and infrastructure → application, domainservices, domain) |
| `hexagonal` | domain, ports, application, adapters (isolated) | ports → domain; application → ports, domain; adapters → application, ports, domain |
| `ddd` | domain, application, infrastructure, interfaces | application → domain; infrastructure, interfaces → application, domain |
| `layered` | presentation, service, repository, model | presentation → service, repository, model; service → repository, model; repository → model |
| `mvc` | controller, view, model | controller → view, model; view → model |
| `pipeline` | one layer per `stages` entry | a stage imports only earlier stages; each stage is isolated |
| `modular` | one per child directory of `modules_root` (default `internal`) | another module may be imported only through its root package (`internal/orders`, not `internal/orders/store`); inside a module anything goes |

An **isolated** layer's packages may import each other only inside one unit (the layer's directory plus one more segment: `internal/controller/http/x` → `internal/controller/http/common`) or along one directory path (`adapters/pg` → `adapters/pg/rows`), never between two units (`adapters/http` → `adapters/pg`). A layer matched by a glob has no unit: directory path only.

In a flat layout the unit is the package itself, so `internal/handlers/user` → `internal/handlers/common` is flagged `(isolated)`. Workaround: put the shared package in a layer of its own and `allow` the adapter layer to import it:

```yaml
architecture:
  preset: hexagonal
  layers: {shared: [internal/handlers/common]}
  allow: ["adapters -> shared"]
```

**Configuration** (all optional, strict like the rest of the file):

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

- `layers` without a `preset` is a custom architecture of exactly those layers: a layer may import only itself and what `allow` lists; the entry rule still applies. Globs win over directory names, the first declared layer first.
- `allow` entries are written `a -> b`, with a space on each side of the arrow (`a->b` is refused).
- An unknown preset, an `allow` or `isolated` entry naming an unknown layer, `stages` without `preset: pipeline` (or `pipeline` without `stages`), `modules_root` without `preset: modular` (or an absolute, `..` or `.` one), `entry` named in `allow` or `isolated` (it is implicit), or a duplicate layer is a config error: exit 2.

**The section** shows the model, conformance (conforming judged edges ÷ judged edges, base → head, `n/a` when nothing is judged) and how many packages were classified; then each **new** violating edge (up to 15, by file and line): `- ❌ new: \`domain → adapters\` — \`internal/domain/order/order.go:7\` imports \`internal/adapters/pg\``, with `(isolated)` when the isolation rule was broken, and the first line counting the violations already in the base. Under *Coupling*, Martin's metrics (Ca, Ce, instability I, abstractness A, distance D) base → head for touched packages (holding an added, modified, deleted or moved non-test file) whose Ca, Ce, A or D changed, at most 10; a package new in the MR shows `—` on the base side. The section appears only when there is a new violation, a conformance change or a metric change; in inferred mode, never when no package outside `entry` was classified. When the base revision cannot be loaded, it shows head conformance only and `(base not loaded: new violations not computed)`.

## CI integration

mrimpact runs where Go and the full Git history are available. The Go toolchain must be the project's own (type-checking needs it): Go 1.26 or newer to build mrimpact from source, and whatever the analysed module requires.

### GitHub Actions

```yaml
name: mrimpact
on: pull_request

jobs:
  mrimpact:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: Kevinbarrero/mr-impact@v0
        with:
          run-tests: true
```

The action is a composite action. It installs the release binary with `install.sh` into `$RUNNER_TEMP/mrimpact`, optionally runs the tests with coverage, then runs mrimpact. It does not install Go: put `actions/setup-go` before it.

Inputs (all optional):

| Input | Default | Meaning |
|---|---|---|
| `version` | the action's ref when it is a `vX.Y.Z` tag, else `latest` | the mrimpact version to install |
| `token` | `${{ github.token }}` | the token used to post, exported as `MRIMPACT_TOKEN` |
| `coverprofile` | `""` | coverage profile globs, one per line (each becomes `--coverprofile`) |
| `run-tests` | `false` | when `true` and no `coverprofile` is given, run `go test ./... -coverprofile=…` first; a test failure fails the step |
| `fail-on` | CLI default | passed as `--fail-on` |
| `config` | CLI default | passed as `--config` |
| `depth` | CLI default | passed as `--depth` |
| `tags` | CLI default | passed as `--tags` |
| `args` | `""` | extra flags; split on whitespace by the shell, with no quoting (use the dedicated inputs for values with spaces) |

Output: `report`, the path of the Markdown note (`$RUNNER_TEMP/mrimpact-report.md`).

### GitHub mode

mrimpact detects a GitHub pull request when `GITHUB_ACTIONS` is `true` and the event file at `GITHUB_EVENT_PATH` holds a `pull_request` object. From the event it reads the pull request number, `pull_request.base.sha` (the default `--base` when neither the flag nor `CI_MERGE_REQUEST_DIFF_BASE_SHA` is set) and whether it comes from a fork (`head.repo.full_name` differs from `base.repo.full_name`). From the environment it reads `GITHUB_REPOSITORY` (`owner/repo`) and `GITHUB_API_URL` (default `https://api.github.com`). GitLab detection wins when both are present. A push or any other non-PR event is local mode; an unreadable or malformed event file, or a `GITHUB_REPOSITORY` that is not `owner/repo`, is a usage error (exit 2).

- **The comment.** The note is one issue comment on the pull request, updated in place on every run. The comment updated is the newest one that carries the note's marker **and** was written by `MRIMPACT_COMMENT_AUTHOR` (default `github-actions[bot]`, the author of comments made with `github.token`); a marker inside anyone else's comment is ignored and never edited. If none matches, a new comment is created; if the update is refused (403/404), a new comment is created once. Set `MRIMPACT_COMMENT_AUTHOR` to the bot or user login when you post with another token (a GitHub App or a personal token).
- **The token.** `MRIMPACT_TOKEN`, as on GitLab; the action passes `github.token`. The workflow needs `permissions: pull-requests: write`. Without a token, mrimpact warns once and does not post; the job summary and `--out` are still written, and it is not an internal failure.
- **The job summary.** When `GITHUB_STEP_SUMMARY` is set, the note is appended to the job summary in every GitHub mode, posted or not (`--no-post` included).
- **Fork pull requests.** The `pull_request` token of a fork PR is read-only, so mrimpact does not try to post. It warns `fork pull request: the token is read-only, note written to the job summary`, writes the summary, and the exit code is unaffected, even with `--strict`.
- **A failed post** (401/403, or 5xx after retries) is a warning and an internal failure: with `--strict`, exit 2.

#### Do not use `pull_request_target`

Do not run mrimpact (or any job that builds or tests the pull request's code) on `pull_request_target`. That event runs with a write token and the base repository's secrets, and mrimpact type-checks the PR's code and the action can run its tests: a fork PR would execute untrusted code with that token. Use `pull_request`; for fork PRs the note lands in the job summary. Commenting on fork PRs through a separate `workflow_run` job is not provided.

### GitLab template

```yaml
include:
  - remote: https://raw.githubusercontent.com/Kevinbarrero/mr-impact/main/ci/mrimpact.gitlab-ci.yml
```

The template ([`ci/mrimpact.gitlab-ci.yml`](../ci/mrimpact.gitlab-ci.yml)) adds one job, `mrimpact`, that runs on merge request pipelines (`rules: if: $CI_PIPELINE_SOURCE == "merge_request_event"`) in `image: golang:${MRIMPACT_GO_VERSION}` with `GIT_DEPTH: "0"`. It installs mrimpact with `install.sh` and keeps `mrimpact-report.md` as an artifact for a week. The job runs in the `test` stage (GitLab's default); if your `stages:` has no `test`, override with `mrimpact: { stage: <yours> }`. `MRIMPACT_VERSION` takes `latest`, `v0.3.0` or `0.3.0`. Variables set in the template's job beat top-level `variables:`, so override them by redefining the job in your `.gitlab-ci.yml`:

```yaml
mrimpact:
  variables:
    MRIMPACT_RUN_TESTS: "true"
```

GitLab merges this with the included job. The variables:

| Variable | Default | Meaning |
|---|---|---|
| `MRIMPACT_GO_VERSION` | `"1.26"` | the `golang` image tag |
| `MRIMPACT_VERSION` | `latest` | the mrimpact version to install |
| `MRIMPACT_COVERPROFILE` | `""` | coverage profile globs, separated by spaces |
| `MRIMPACT_RUN_TESTS` | `"false"` | when `"true"` and no coverprofile is given, run `go test ./... -coverprofile=mrimpact.cover` first |
| `MRIMPACT_ARGS` | `""` | extra flags, split on whitespace |

To reuse a coverage profile from an earlier job, set `MRIMPACT_COVERPROFILE` the same way (`mrimpact: { variables: { MRIMPACT_COVERPROFILE: "coverage.out" }, needs: [test] }`); that job must publish the profile as an artifact.

Set `MRIMPACT_TOKEN` as described in [Token setup](#token-setup). Runners without github.com access can build mrimpact with `go install` (from a Go module proxy) instead of the template; see the job below.

### GitLab job without the template

```yaml
mrimpact:
  stage: report
  needs: [test]            # test job publishes coverage.out as artifact
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    GIT_DEPTH: 0
  script:
    - mrimpact --coverprofile coverage.out
  artifacts:
    paths: [mrimpact-report.md]
    when: always
```

The CI image must provide Go 1.26 (e.g. `golang:1.26`).

With no github.com access, install it in the job with `go install github.com/Kevinbarrero/mr-impact/cmd/mrimpact@latest` (or a pinned version) before the script line.

### Shallow clones

mrimpact needs the base commit in the clone. When it is missing, the run fails with `base commit not in clone: fetch full history (GitLab: GIT_DEPTH: 0; GitHub: actions/checkout with fetch-depth: 0)`. On GitLab, set `GIT_DEPTH: 0` (the template does); on GitHub, use `actions/checkout` with `fetch-depth: 0`.

## Install

- **From source:** `go install github.com/Kevinbarrero/mr-impact/cmd/mrimpact@latest` (Go 1.26 or newer).
- **A release binary:** [`install.sh`](../install.sh) downloads a release archive and its `checksums.txt`, verifies the SHA-256 and installs `mrimpact`:

  ```sh
  curl -sSfL https://raw.githubusercontent.com/Kevinbarrero/mr-impact/main/install.sh | sh -s -- -b ~/.local/bin
  ```

  Usage: `install.sh [-b bindir] [version]`. `bindir` defaults to `./bin`; `version` defaults to `latest` and takes a tag such as `v0.3.0` (or `0.3.0`). It needs `curl` or `wget`, `tar` (or `unzip` on Windows), and `sha256sum` or `shasum -a 256`. A checksum mismatch or a missing checksum entry exits 1 and installs nothing. `MRIMPACT_DOWNLOAD_URL` overrides the download base (default `https://github.com/Kevinbarrero/mr-impact/releases/download`), for a mirror.
- **Archives** are on the [releases page](https://github.com/Kevinbarrero/mr-impact/releases): `mrimpact_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) for linux, darwin and windows on amd64 and arm64.

## Token setup

On GitLab:

1. Create a Project Access Token with the `api` scope and the Reporter role.
2. Add it as a masked CI/CD variable named `MRIMPACT_TOKEN`.

The note is posted only when `CI_MERGE_REQUEST_IID`, `CI_PROJECT_ID` and `CI_API_V4_URL` are set (GitLab sets them in MR pipelines).

On GitHub, the action passes `github.token` as `MRIMPACT_TOKEN`; grant `permissions: pull-requests: write` to the job (see [GitHub mode](#github-mode)).

## Shadow mode

`mrimpact --no-post` runs the full analysis, prints the note and writes `mrimpact-report.md` but never contacts GitLab or GitHub (on GitHub the job summary is still written). Use it to evaluate the output as a job artifact before enabling posting.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success, or degraded/internal failure without `--strict` (the failure is reported on stderr and in the note) |
| 1 | `--fail-on red` and at least one red function, or `--fail-on arch` and a new violation of the declared architecture (never for an inferred model or a missing base) |
| 2 | Usage error (including a config error), or `--strict` with an internal failure (analysis, posting or `--out` write error) |

`--fail-on` takes a comma list of `red` and `arch` (empty items are ignored; anything else is a usage error). `--depth` must be at least 1 and `--timeout` greater than 0.

## What the note shows

In order, each section only when it has something:

- **Header**: how many functions changed, the 🔴/🟡/🟢 tally of the table's rows and, when new code was grouped, `+N new functions folded into M groups` (`1 group` for one).
- **Config line**: `⚙️ .mrimpact.yml: …` when the config removes defaults, names critical globs, declares an architecture, or was changed by the MR (see Configuration).
- **Coverage hint**: `ℹ️ N functions are covered only by tests in other packages; …` when changed functions have added lines with 0 % in the coverprofile although tests in another package reach them. A coverprofile made without `-coverpkg` counts same-package tests only; run `go test -coverpkg=./... -coverprofile=…` for line coverage. Their cover cell reads `0 % in profile · reached by tests in <pkg>`, and they count as partly tested, not untested.
- **Removed `case`, value still exists**: a `case` removed from a switch on a named constant or type that is still declared; those values now go to `default`, or match no case.
- **Now unused / now only used by tests**: functions the removed code used that nothing reachable from an entry point (`main`, `init`, exported API) calls or takes as a value any more, plus what only they reached. When the base revision loads, what the removed code used is resolved with its types; otherwise it is matched by package and name.
- **Config no longer read**: struct fields with a config tag (`envconfig`, `env`, `yaml`, `mapstructure`, `toml`, `koanf`, …) the removed code used and nothing reads now: settings deployments still set for nothing.
- **Generated code no longer used**: e.g. sqlc queries; remove them from their source and regenerate. Methods an interface of the module still requires (mocks) and protobuf messages (`*.pb.go`) are never listed.
- **Registrations removed / rewired / changed / added**: calls that take one of the module's functions as a value (an HTTP route, an event subscription, a hook, a CLI command), compared between base and head by call and handler. "Rewired" is the same call with another handler; "removed, no replacement" is a call that no longer gets one. "Changed" is a registration that kept its identity (callee, string literals, handler, and the route when one renders) but gained or lost other arguments, such as a middleware or an option: `` 🔌 **Registrations changed** (N, positions in head): `` lists each as `` - `GET /trade/:id` → `api.GetTrade`: + `mw`, − `old` — `api/api.go:21` ``, after Rewired and before Added. A base and a head registration are paired only when exactly one candidate on each side shares that identity; otherwise both stay removed and added. One edit gives one line: the other registrations a changed call yields at the same position (a middleware newly passed to a route is itself a function value) are not listed again, so a middleware added to a route appears only as `+ mw` on the handler's line. The same literal under another group prefix is another route (`admin.GET("/x", auth, h)` → `public.GET("/x", h)` is removed and added, not changed). Only functions passed to the call directly are compared here; arguments of a call that is not a route are shown as written, and an argument longer than 90 characters is cut with `…`. Every registration block shows the route when the call renders as one (see Routes), else the call text.
- **🔐 Sensitive code changed**: every changed function in which an added line falls on a use of an API in the sensitive table (a call, a function taken as a value or a composite literal, closures included), or that lies in a `critical` package. A use counts as touched when an added line is anywhere in its span, from the start of the whole expression to its end, so a multi-line call whose argument line changed counts; a function that only keeps an untouched `db.Ping()` while another line of its body changes is not sensitive. Only the touched APIs are listed; a function in a `critical` package lists all its sensitive APIs. Untested ones first, then by name: `- 🔴 \`pkg.F\` — os/exec.Command, database/sql.(*Tx).Exec · 0 % (0/7)`, up to 3 matched APIs then ` +N`; a function marked only by `critical` shows `critical: <glob>`. A sensitive function that is untested (it has added lines and, with a coverprofile, they are `0/N`; without one, no test reaches it) is 🔴 whatever its callers and not softened by tests in other packages; `main`, `init` and wiring functions are exempt from this rule (wiring stays at most 🟡) but still listed; partly tested stays 🟡, and one with no added lines is rated as usual, as is one in a degraded run without a coverprofile (test reach unknown; still listed). A folded group member is listed with its own row.
- **🏛 Architecture**: new import edges that break the layer rules, conformance and coupling changes (see Architecture).
- **Table**: every changed function with its risk, kind (`new`, `body`, `signature`; `(branch removed)` when a `case`, `if`/`else` or `return` was removed), callers up to `--depth`, where it is registered as a callback, and the coverage of its added lines. Without a coverprofile the cell reads `reached by test · added lines not measured` (added lines, reached by a test), `no added lines · reached by test` (it only lost lines) or `not reached`; `n/a` when there is no call graph. Without a coverprofile, a function with added lines that a test reaches is at most 🟡: whether the added lines ran is not measured. `· wiring` after the kind (`new · wiring`, `body · wiring`, `signature · wiring`) marks straight-line setup: a function that only constructs and connects values (type arguments of `make` and `new`, such as `make(map[string]int)`, and generic instantiations in type position count as construction), with no branch other than error exits (`if err != nil { return … }`, `Fatal*`, `Exit`, `panic`) and no computation (arithmetic, comparisons, indexing, loops, switches); a closure it passes on is held to the same rule, so a closure with a loop or arithmetic makes it not wiring, and a function with a `context.Context` parameter runs per request and is never wiring; it is rated at most 🟡 (an exported signature change in an importable package stays 🔴). `· sensitive` follows (after `· wiring` when both apply) for a function listed under 🔐.
- **Uncovered added lines** (coverprofile mode only): under the table, `<details><summary>Uncovered added lines (N functions)</summary>` lists, for each 🔴 and 🟡 row shown in the table, in table order, the added lines the coverprofile counts as not run: `` - `svc.(*Service).Lookup` — `svc/svc.go:14-16, 20` ``. Adjacent lines merge into ranges `a-b`; up to 5 ranges are shown, then ` +N more`; up to 15 functions, then `- … and N more`. Lines outside every coverprofile block (comments, blank lines) are neither covered nor uncovered and never listed. A group row lists its entry function, then each function folded into it that has uncovered lines, each on its own line (they count toward N and the 15-function cap). A row whose cover reads "reached by tests in …" lists none: those tests may run the lines a same-package profile counts as not run.
- **Groups**: a new function called only by other new functions, and not 🔴 on its own, is folded into the row of the new function that enters it (kind `new · +N`, `new · +N · sensitive` when that entry function is sensitive); the row's coverage and risk are summed over the whole group (but an entry 🔴 on its own keeps the row 🔴), and ` · K untested` counts the folded functions with no covered added line that no test in another package reaches either. Under the table, `<details>` *X +N new* lists each folded function with its own risk, kind, coverage and file. The diagram shows only the entry. A function 🔴 on its own always keeps its own row.
- **Diagram**: the changed functions coloured by risk, with a legend; up to 3 callers each (whoever registered it as a callback first, as one dashed "registers via …" edge), the remaining direct callers as one "+N" node. The full list follows under *All callers*.
- **Deleted functions, changed declarations, touched tests**: inline when short, otherwise counted and collapsed, grouped by package or file.

Callers through a func value (an event bus, a handler table) are narrowed to the code that registered the function; callers of a closure are not counted as callers of the function that created it. An exported signature change is red only in a package other modules can import.

The **Entry points** column shows which entry points reach a changed function through any number of calls, the most specific first: the registration call that passes it as a value, as its route when it renders as one (e.g. `GET /trade/:id`, see Routes) or else its call text (e.g. `Subscribe("x", …)`), `api <function>` (an exported function of an importable package, or an exported method of an exported type there), `main` and `init` (several `init` or `main` functions are one `init` or `main`). The first two are shown, then ` +N`; `—` means none. Registrations and `api` functions render as code spans. With two or more rows, an entry point every shown row has (often `main`) is left out of the cells and stated once above the table: `All changes shown are reached from: …`. The column appears only when some shown row has an entry point, and never in a degraded run.

**Routes.** A registration call renders as `VERB /joined/path` when its callee is a verb (case-insensitive `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`, `CONNECT`, `TRACE`; `Any` renders `ANY /p`) and its first argument is a path literal: one starting with `/`, a method pattern (`"GET /x"`), or `""` under a resolved prefix that starts with `/`. Any other literal is not a route, so `HandleFunc("email:deliver", h)`, `Handle("user.created", h)` or `cache.Get("key", f)` keep their call text, and so does `r.GET("", h)` with no prefix resolved. `Handle`, `HandleFunc` and `Match` render the path alone (`/x`); a literal that already starts with a method (`"GET /x"`) is kept as written, and gin's `Handle("GET", "/x", h)` renders `GET /x`. Prefixes come from `Group`, `Route`, `PathPrefix` and `Prefix` calls with a string-literal first argument, followed up to 4 levels through local variables assigned once in the same function (a plain copy `h := g` is followed too and counts as a level), and are joined like `path.Join` (a trailing `/` is kept, `""` is the prefix itself, the root is `/`): `api := e.Group("/trade"); api.GET("/:id", h)` is `GET /trade/:id`. A package identifier (`http.HandleFunc`, `http.DefaultServeMux`) or a call result (`gin.Default()`, `newGroup()`) is a complete root. A parameter, a method receiver or a struct field (`c.router`) is a root whose outer prefixes were set elsewhere: the path starts with `…` (`func Register(g *gin.RouterGroup) { g.POST("/trade", h) }` is `POST …/trade`), and the prefixes resolved in that function follow it. Whenever a prefix cannot be resolved without guessing (a prefix that is not a string literal, a local assigned more than once, another method call in the chain such as `.Use(mw)`, more than 4 levels), the call text is shown instead.

The base revision is exported with `git archive` into a temporary directory and type-checked next to the head; the repository and its worktree are not touched.

## Limits

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers; where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname`, files excluded by build tags or other repositories are not seen.
- Cross-package coverage is judged by static reachability: a test in another package whose call graph reaches the function counts, even if at run time it takes another path.
- Registrations blocks: only calls that take a function directly are compared; a function inside a slice literal (`[]HandlerFunc{…}...`) or a struct literal is not. Entry points and Registered do follow a slice literal that reaches exactly one call (the slice a variadic call builds).
- Registrations whose arguments were only reordered, or that share their identity (callee, string literals, handler, route) with another registration on either side, are listed as removed and added, not as changed.
- Arguments of a variadic call that is not a route are shown as written, all of them, with no `…` marker.
- Routes: prefixes set in another function (a router group passed in as a parameter or kept in a field) are not followed; the route shows `…` followed by the prefixes resolved in the registering function. A method pattern (`mux.HandleFunc("GET /x", h)`) is shown as written, without `…`.
- Signatures are compared as written: replacing a type by an alias of it (`type Completion = string`) reads as a signature change, red when exported.
- A function dispatched through a framework registry filled in `init` (e.g. Caddyfile directives) can read *not reached* although tests run it: the call graph narrows the registry's callers to the `init` that registered it. A coverprofile made with `-coverpkg=./...` shows its real coverage.
