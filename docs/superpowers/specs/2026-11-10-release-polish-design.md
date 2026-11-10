# mrimpact — Release polish (first public release)

- **Date:** 2026-11-10
- **Status:** Draft, pending author review
- **Base:** `feat/architecture` at 21388c3
- **Follows:** the mrimpact v1, signal-fixes, reach-focus and architecture specs

## 1. Problem

mrimpact works, but a stranger cannot find it, install it or try it:
- it lives on a local branch chain with no remote;
- it posts only to GitLab;
- the version is hard-coded;
- the README is a reference manual.

**Goal:** a first public release, `v0.3.0`, on GitHub (`github.com/Kevinbarrero/mrimpact`) with a GitLab mirror. A Go developer should understand the tool in 10 seconds and run it on their own PR in one minute, on GitHub or on GitLab.

## 2. Constraints

- **Unchanged rules:**
  - general Go;
  - deterministic;
  - no AI, API keys or tokens beyond the forge token used to post;
  - zero-config first.
- **GitLab behaviour is unchanged.** Every existing test and golden keeps passing, apart from the footer version.
- **Nothing is pushed, tagged or published by the implementation.** The owner adds the remote, pushes, and creates the first tag.
- **No new Go dependency.** The GitHub client uses `net/http` and `encoding/json`, like `internal/gitlab`.
- **License:** Apache-2.0, copyright line `Copyright 2026 Kevinbarrero`.

## 3. Repository and version

- **Branches:** `feat/v1` → `feat/signal-fixes` → `feat/reach-focus` → `feat/architecture` → `feat/release` are merged into `main`, fast-forward where possible. `main` is the release branch.
- **Module path:** `gitlab.com/wh1t3hat/mrimpact` becomes `github.com/Kevinbarrero/mrimpact`. That covers `go.mod`, every import, the docs, and the test expectations that print the path.
- **Version:**
  - `pipeline.Version` (a const `"0.2.0"`) is replaced by a value set by the binary;
  - `cmd/mrimpact` keeps `var version = "dev"`, set with `-ldflags "-X main.version=v0.3.0"`;
  - when it is `"dev"`, `runtime/debug.ReadBuildInfo()` supplies the module version (`go install …@v0.3.0`), otherwise it stays `dev`;
  - the footer reads `mrimpact <version>` (the `v` comes from the tag, never doubled);
  - `--version` prints `mrimpact <version>` and exits 0;
  - e2e tests pin the version so goldens stay stable.

## 4. GitHub support

### 4.1 Detection (`cmd/mrimpact`)

`detectCI(env, readFile)` returns one of:

- **GitLab:** when `CI_MERGE_REQUEST_IID`, `CI_PROJECT_ID` and `CI_API_V4_URL` are set, exactly as today.
- **GitHub PR:**
  - **When:** `GITHUB_ACTIONS == "true"`, and the JSON at `GITHUB_EVENT_PATH` has a `pull_request` object.
  - **Read from the event:**
    - `pull_request.number`;
    - `pull_request.base.sha`, the default for `--base` when neither the flag nor the GitLab variable is set;
    - fork: `pull_request.head.repo.full_name != pull_request.base.repo.full_name`.
  - **Read from the environment:** `GITHUB_REPOSITORY` (`owner/repo`) and `GITHUB_API_URL` (default `https://api.github.com`).
- **Local:** anything else. Nothing is posted, as today.

Errors:
- an unreadable or malformed event file is a usage error (exit 2) naming the file;
- a push or other non-PR event is local mode.

Posting goes through `type poster interface{ Upsert(ctx, body string) error }`, with a GitLab and a GitHub implementation. `main` only picks one.

### 4.2 Posting (`internal/github`, new)

- **Comments are issue comments:**
  - list: `GET {api}/repos/{owner}/{repo}/issues/{n}/comments?per_page=100`, following `Link: rel="next"`;
  - update: `PATCH …/issues/comments/{id}`;
  - create: `POST …/issues/{n}/comments`.
  - Headers: `Authorization: Bearer <token>`, `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28`.
- **The target comment** is the newest one (highest `id`) that contains `report.Marker` **and** whose `user.login` equals the expected author: `MRIMPACT_COMMENT_AUTHOR`, default `github-actions[bot]`. A marker inside anyone else's comment is ignored. If none matches, a new comment is created. If a `PATCH` is refused (403/404), a new comment is created once.
- **Retries and timeout:** the same policy and error style as `internal/gitlab`. 401/403 on list or create, and 5xx after retries, are errors.
- **Token:** `MRIMPACT_TOKEN`, as on GitLab. The action passes `github.token`. The workflow needs `permissions: pull-requests: write`.

### 4.3 Job summary and failure cases

When `GITHUB_STEP_SUMMARY` is set, the note is appended to that file in every GitHub mode, posted or not.

| Case | Behaviour |
|---|---|
| no `MRIMPACT_TOKEN` | one warning, no post; the summary and `--out` are still written; not an internal failure |
| fork PR | one warning: `fork pull request: the token is read-only, note written to the job summary`; no post; not a failure |
| a post fails (401/403/5xx) | a warning; internal failure (`--strict` → exit 2), as on GitLab |
| base not reachable (shallow clone) | the existing failure, plus the hint `use actions/checkout with fetch-depth: 0` |
| `--no-post` | nothing is posted; the summary is still written |

The README warns against `pull_request_target`, which runs untrusted PR code with a write token. Commenting on fork PRs with a separate `workflow_run` job is out of scope.

## 5. Distribution

- **`.goreleaser.yaml`:**
  - builds `./cmd/mrimpact` with `CGO_ENABLED=0` for linux, darwin and windows on amd64 and arm64, with `-s -w -X main.version=v{{.Version}}` (GoReleaser's `.Version` has no `v`);
  - packs `tar.gz` archives (`zip` on windows) named `mrimpact_<version>_<os>_<arch>`, each holding the binary, `LICENSE`, `README.md`, `CHANGELOG.md` and `third_party_licenses/` (the license of every module in `go.mod`'s require blocks);
  - publishes `checksums.txt` (SHA-256);
  - takes the release notes from the matching `CHANGELOG.md` section.
- **`install.sh` (POSIX sh, in the repo root):**
  - **Usage:** `install.sh [-b bindir] [version]`; the version defaults to `latest`.
  - **What it does:** resolves the OS/arch, downloads the archive and `checksums.txt` from `${MRIMPACT_DOWNLOAD_URL:-https://github.com/Kevinbarrero/mrimpact/releases/download}`, verifies the SHA-256 (exit 1 on mismatch or a missing entry), and installs `mrimpact` into the bindir.
  - **Resolving `latest`:** follows the `releases/latest` redirect.
  - **Dependencies:** `curl` or `wget`, and `sha256sum` or `shasum -a 256`.
- **`action.yml` (composite):**
  - **Steps:** run `install.sh` into `$RUNNER_TEMP/mrimpact`, optionally `go test ./... -coverprofile=$RUNNER_TEMP/mrimpact.cover`, then run mrimpact.
  - **Inputs:**
    | Input | Default | Meaning |
    |---|---|---|
    | `version` | the action's ref when it matches `vX.Y.Z`, else `latest` | the binary version |
    | `token` | `${{ github.token }}` | exported as `MRIMPACT_TOKEN` |
    | `coverprofile` | `""` | one glob per line |
    | `run-tests` | `false` | when `true` and no `coverprofile`: run `go test ./... -coverprofile` first; a test failure fails the step |
    | `fail-on`, `config`, `depth`, `tags` | CLI defaults | passed through |
    | `args` | `""` | extra flags |
  - **Output:** `report`, the note's path.
  - **Prerequisite:** the action does not install Go, because type-checking needs the project's own toolchain. The README shows `actions/setup-go` before it.
- **GitLab template, `ci/mrimpact.gitlab-ci.yml`:**
  - one job `mrimpact` with `rules: if $CI_PIPELINE_SOURCE == "merge_request_event"`, `GIT_DEPTH: "0"` and `image: golang:${MRIMPACT_GO_VERSION}`;
  - variables (defaults): `MRIMPACT_GO_VERSION` (`"1.26"`), `MRIMPACT_VERSION` (`latest`), `MRIMPACT_COVERPROFILE` (`""`), `MRIMPACT_RUN_TESTS` (`"false"`), `MRIMPACT_ARGS` (`""`);
  - installs with `install.sh`; the README documents `go install` for runners without github.com access.
- **The project's own automation:**
  - `.github/workflows/ci.yml`, on push and pull_request: `go test ./...`, `go vet ./...`, the gofmt check, then `uses: ./` on pull requests (dogfooding);
  - `.github/workflows/release.yml`, on `v*` tags: GoReleaser, then force-move the floating `v0` tag;
  - `.github/dependabot.yml`, for gomod and github-actions, weekly.

## 6. Documentation

- **`README.md`** (rewritten, short):
  1. Title, one-line pitch, and badges: CI, release, pkg.go.dev, Go Report Card, license.
  2. A real note: embedded Markdown from a real run until a dogfood PR screenshot exists. Never mocked.
  3. What a reviewer learns, in 5 bullets.
  4. Quickstart: a GitHub workflow (checkout with `fetch-depth: 0`, setup-go, the action), the GitLab `include:`, and local `go install` plus one command.
  5. The examples gallery: one line per example plus a link.
  6. How it works, in one paragraph.
  7. Compared to: codecov/diff-cover, golangci-lint + reviewdog, `deadcode`, arch-go/go-arch-lint, govulncheck, SonarQube and AI reviewers. Each row gives what that tool does, what mrimpact adds, and when you would still use the other one.
  8. Limitations: Go only, one module per run, reflection and plugins not followed, the Go toolchain is required on the runner.
  9. License.
- **`docs/reference.md`:** today's README reference sections, moved without loss (flags, configuration, the sensitive table, architecture, exit codes, the config line, the note's sections). It also fixes the wording "never when no package was classified" to "no package outside `entry`".
- **`docs/examples/`:**
  - **Repos:** 3–4 recently merged PRs from well-known Go repositories, chosen from gin, cobra, caddy and prometheus.
  - **How they are run:** locally on clones, with zero config, and coverage where the test suite is fast and self-contained. Nothing is posted to those projects.
  - **Each file:** the repo, the PR link, `base..head`, the exact command, and the note unedited.
  - **Misfires found on that code:** a small one with a clear fix is fixed with a test before the release; anything deeper goes under Limitations, or becomes an issue for the next sub-project.
- **`LICENSE`:** the Apache-2.0 text, with the appendix filled as `Copyright 2026 Kevinbarrero`.
- **`CHANGELOG.md`:** Keep-a-Changelog format, an `Unreleased` section, and a `v0.3.0` section summarising v1, the signal fixes, reach & focus, architecture and this release as the first public version.
- **`CONTRIBUTING.md`:**
  - build and test;
  - goldens: `-update`, then read the diff;
  - the rules for new checks: general Go, no AI, zero-config;
  - contributions are accepted under Apache-2.0, with no CLA.
- **`SECURITY.md`:** report through GitHub private vulnerability reporting.
- **`.github/ISSUE_TEMPLATE/`:** bug (asks for `mrimpact --version`, the command, and a note excerpt), false positive / false negative (what the note said against what is true), and feature.
- **Launch drafts:** a Show HN post, an r/golang post and a Golang Weekly email, under `docs/launch/`. They are listed in `.gitignore` and never committed.

## 7. Testing

- **`internal/github`, against an `httptest` server:**
  - the update path and the create path;
  - pagination through `Link`;
  - the marker in a comment by another author is ignored;
  - a refused `PATCH` falls back to create;
  - 401 and 5xx after retries;
  - the request headers.
- **Detection:** synthetic event files for a PR, a fork PR, a push event, a malformed file, and a missing file. Base defaulting covers flag over GitLab variable over event.
- **CLI:**
  - the summary file is written (posted, no token, fork, `--no-post`);
  - `--version`;
  - the version comes from ldflags, from build info, or is `dev`;
  - the GitLab tests are unchanged.
- **`install.sh`:** a Go test builds a fake release directory (an archive plus `checksums.txt`), serves it with `httptest`, and runs the script through `sh`. It checks a good install, a checksum mismatch giving exit 1, and an unknown version failing. The test is skipped where `sh` or `tar` is missing.
- **YAML:**
  - `action.yml`, the workflows and the GitLab template parse as YAML in a Go test;
  - `actionlint` and `goreleaser check`, plus `goreleaser release --snapshot --clean`, run when those tools are installed, and are noted as skipped otherwise.
- **Goldens:** only the footer version line changes, pinned in e2e.

## 8. Out of scope

- **Packaging:** a Homebrew tap, a Docker image, signing (cosign) and SBOMs.
- **Integrations:** an MCP server or JSON output (sub-project D), and a `workflow_run` pattern for commenting on fork PRs.
- **Owner actions:** publishing to the Marketplace (the owner does it on the first release), and pushing, tagging or posting anywhere.
