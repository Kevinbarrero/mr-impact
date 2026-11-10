# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Routes:** registration calls render as the route they register (`GET /trade/:id`), with `Group`/`Route`/`PathPrefix`/`Prefix` prefixes followed through locals of the same function, in the Entry points column and every registration block; only path literals (`/x`, `"GET /x"`, `""` under a `/` prefix) render, and anything ambiguous keeps the call text.
- **Registrations changed:** a registration that keeps its call and handler but gains or loses other arguments (a middleware, an option) is listed once under `🔌 **Registrations changed**` as `+ a, − b`, instead of as removed plus added. A route moved to another group prefix stays removed plus added.
- **Uncovered added lines:** in coverprofile mode, a collapsed list under the table gives the uncovered added line ranges of each 🔴 and 🟡 row and of the functions folded into it; rows reached only by tests in other packages are not listed.

### Fixed

- A function is sensitive only when an added line touches the sensitive call (multi-line calls included) or it lies in a `critical` package; 🔐 lists only the touched APIs. An unrelated edit to a function that calls `db.Ping()` no longer marks it sensitive.
- Handlers passed to a variadic registration (`GET("/x", mw, h)`) resolve to that call, so middleware-wrapped routes are registrations and entry points.
- `make` and `new` type arguments (`make(map[string]int)`) and generic instantiations no longer stop a constructor from being wiring.

## [v0.3.1] - 2026-11-10

### Changed

- The GitHub Action's Marketplace name is now `mrimpact for Go` (the name `mrimpact` is taken in the Marketplace). Usage is unchanged: `uses: Kevinbarrero/mr-impact@v0`.

## [v0.3.0] - 2026-11-10

The first public release. Earlier versions were internal and never published.

### Added

- **The note.** One risk-ranked note per merge request: the changed functions (new, body, signature), their callers up to `--depth` from an SSA/VTA call graph, the coverage of the added lines from coverprofiles (or static test reach without one), a Mermaid diagram with a legend, deleted functions, changed declarations and touched tests. Updated in place on every run.
- **Detectors for what a change leaves behind:** removed `case` arms whose value still exists, code now unused or only used by tests, config fields no longer read, generated code no longer used, and registrations (routes, subscriptions, hooks, commands) removed, rewired or added.
- **Signal:** cross-package test reach (a coverage hint when a coverprofile was made without `-coverpkg`), the `wiring` kind for straight-line setup (at most 🟡), and new-code groups folded into the row of the function that enters them.
- **Reach and focus:** the 🔐 sensitive-code section with a default table of standard-library and common data-access and crypto APIs (untested sensitive code is 🔴), the Entry points column (registrations, exported API, `main`, `init`), and an optional, strict `.mrimpact.yml` (`critical` globs, `sensitive` add/remove) with a config line that states what the MR changed in it.
- **Architecture:** the 🏛 section judges the package import graph against layer rules: zero-config inferred hints, presets (`clean`, `onion`, `hexagonal`, `ddd`, `layered`, `mvc`, `pipeline`, `modular`) or custom layers, isolated layers, new violations with file and line, conformance and Martin coupling metrics base → head, and the `--fail-on arch` gate.
- **GitHub pull requests:** detection from the Actions event, one issue comment upserted by marker and author (`MRIMPACT_COMMENT_AUTHOR`), the job summary, and fork pull requests handled without posting.
- **Distribution:** the GitHub Action `Kevinbarrero/mr-impact@v0`, the GitLab template `ci/mrimpact.gitlab-ci.yml`, release archives for linux, darwin and windows (amd64, arm64) with `checksums.txt`, and `install.sh`, which verifies the checksum before installing.
- `--version`, `--fail-on red,arch`, `--skip-empty`, `--strict`, `--config`.

### Changed

- The module path is `github.com/Kevinbarrero/mr-impact`; the command stays `mrimpact`.
- The version comes from the release build, or from the module's build info for `go install`; the note's footer reads `mrimpact <version>`.
- The shallow-clone error names both fixes: `GIT_DEPTH: 0` on GitLab and `fetch-depth: 0` on GitHub.
- The README is short; the full reference moved to `docs/reference.md`.
- Licensed under Apache-2.0.

### Fixed

- The diff is taken against the merge base of base and head, not the base tip.
- Parameter and result renames are not reported as signature changes.
- Packages with only non-type errors (such as `go:embed`) are still analysed.
- Callers are deduplicated by function and package; a closure's callers are not counted as the enclosing function's.
- A group is never rated below its red entry, and a function red on its own is never folded.
- A function taking `context.Context` is never wiring, and wiring inspects closure bodies.
- The inferred architecture section no longer appears when no package outside `entry` is classified; isolation compares adapter units, not sibling packages.
- `git` runs with `LC_ALL=C`; error-response bodies from the forge are capped at 1 MiB; no post is attempted without a token.
