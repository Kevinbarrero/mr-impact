# mrimpact

`mrimpact` runs in a GitLab merge request pipeline on a Go repository. It finds the functions changed by the MR, their callers (call graph, configurable depth) and their test coverage, and posts one risk-ranked note on the MR. The note is updated in place on every pipeline run.

## Local usage

```sh
mrimpact --base origin/master
```

Prints the note to stdout and writes it to `mrimpact-report.md`. Nothing is posted unless the GitLab CI variables are present. Run it from the module root (or pass `--dir`), with the head revision checked out.

Flags: `--dir`, `--base` (env `CI_MERGE_REQUEST_DIFF_BASE_SHA`), `--head HEAD`, `--depth 2`, `--coverprofile` (repeatable glob), `--tags a,b`, `--timeout 5m`, `--fail-on red`, `--strict`, `--skip-empty`, `--no-post`, `--out mrimpact-report.md`.

## CI integration

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

## Token setup

1. Create a Project Access Token with the `api` scope and the Reporter role.
2. Add it as a masked CI/CD variable named `MRIMPACT_TOKEN`.

The note is posted only when `CI_MERGE_REQUEST_IID`, `CI_PROJECT_ID` and `CI_API_V4_URL` are set (GitLab sets them in MR pipelines).

## Shadow mode

`mrimpact --no-post` runs the full analysis, prints the note and writes `mrimpact-report.md` but never contacts GitLab. Use it to evaluate the output as a job artifact before enabling posting.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success, or degraded/internal failure without `--strict` (the failure is reported on stderr and in the note) |
| 1 | `--fail-on red` and at least one red function |
| 2 | Usage error, or `--strict` with an internal failure (analysis, posting or `--out` write error) |

`--fail-on` accepts only `""` or `red`. `--depth` must be at least 1 and `--timeout` greater than 0.

## What the note shows

In order, each section only when it has something:

- **Header**: how many functions changed, the 🔴/🟡/🟢 tally of the table's rows and, when new code was grouped, `+N new functions folded into M groups`.
- **Coverage hint**: `ℹ️ N functions are covered only by tests in other packages; …` when changed functions have added lines with 0 % in the coverprofile although tests in another package reach them. The coverprofile counts same-package tests only; run `go test -coverpkg=./... -coverprofile=…` for line coverage. Their cover cell reads `0 % in profile · reached by tests in <pkg>`, and they count as partly tested, not untested.
- **Removed `case`, value still exists**: a `case` removed from a switch on a named constant or type that is still declared; those values now go to `default`, or match no case.
- **Now unused / now only used by tests**: functions the removed code used that nothing reachable from an entry point (`main`, `init`, exported API) calls or takes as a value any more, plus what only they reached. When the base revision loads, what the removed code used is resolved with its types; otherwise it is matched by package and name.
- **Config no longer read**: struct fields with a config tag (`envconfig`, `env`, `yaml`, `mapstructure`, `toml`, `koanf`, …) the removed code used and nothing reads now: settings deployments still set for nothing.
- **Generated code no longer used**: e.g. sqlc queries; remove them from their source and regenerate. Methods an interface of the module still requires (mocks) and protobuf messages (`*.pb.go`) are never listed.
- **Registrations removed / rewired / added**: calls that take one of the module's functions as a value (an HTTP route, an event subscription, a hook, a CLI command), compared between base and head by call and handler. "Rewired" is the same call with another handler; "removed, no replacement" is a call that no longer gets one.
- **Table**: every changed function with its risk, kind (`new`, `body`, `signature`; `(branch removed)` when a `case`, `if`/`else` or `return` was removed), callers up to `--depth`, where it is registered as a callback, and the coverage of its added lines (`no added lines · reached by test` / `not reached` when it only lost lines). `· wiring` after the kind (`new · wiring`, `body · wiring`, `signature · wiring`) marks straight-line setup: a function that only constructs and connects values, with no branch or computation; a closure it passes on is held to the same rule, so a closure with a loop or arithmetic makes it not wiring; it is rated at most 🟡 (an exported signature change in an importable package stays 🔴).
- **Groups**: a new function called only by other new functions is folded into the row of the new function that enters it (kind `new · +N`); the row's coverage and risk are summed over the whole group, and ` · K untested` counts the folded functions with no covered added line. Under the table, `<details>` *X +N new* lists each folded function with its own risk, kind, coverage and file. The diagram shows only the entry.
- **Diagram**: the changed functions coloured by risk, with a legend; up to 3 callers each (whoever registered it as a callback first, as one dashed "registers via …" edge), the remaining direct callers as one "+N" node. The full list follows under *All callers*.
- **Deleted functions, changed declarations, touched tests**: inline when short, otherwise counted and collapsed, grouped by package or file.

Callers through a func value (an event bus, a handler table) are narrowed to the code that registered the function; callers of a closure are not counted as callers of the function that created it. An exported signature change is red only in a package other modules can import.

The base revision is exported with `git archive` into a temporary directory and type-checked next to the head; the repository and its worktree are not touched.

## Limits

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers; where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname`, files excluded by build tags or other repositories are not seen.
- Registrations: only calls that take a function directly (not inside a struct literal or a slice) are compared.

## License

TBD (MIT or Apache-2.0)
