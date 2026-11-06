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

## Limits

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers.

## License

TBD (MIT or Apache-2.0)
