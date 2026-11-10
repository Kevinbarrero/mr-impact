# Release Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make mrimpact releasable as `v0.3.0` on `github.com/Kevinbarrero/mr-impact`:
- the module renamed;
- a real version string;
- GitHub PR posting and a job summary;
- an install script, GoReleaser, an Action and a GitLab template;
- a short README with real examples;
- legal and contribution files.

**Architecture:** A new `internal/github` package mirrors `internal/gitlab`. `cmd/mrimpact` gains a `detectCI` step and a small `poster` interface, so the run picks GitLab, GitHub or local mode from the environment. Distribution is plain files (`install.sh`, `.goreleaser.yaml`, `action.yml`, `ci/mrimpact.gitlab-ci.yml`, workflows), tested with Go tests where they can run offline.

**Tech Stack:** Go 1.26 standard library (`net/http`, `encoding/json`, `runtime/debug`), POSIX sh, YAML. No new Go dependency (`gopkg.in/yaml.v3` already exists, for the YAML parse test).

**Spec:** `docs/superpowers/specs/2026-11-10-release-polish-design.md`

## Global Constraints

- **Rules for every check:** general Go, deterministic, no AI/API keys/tokens beyond the forge token, zero-config first.
- **GitLab behaviour is unchanged.** Every existing test and golden passes; only the footer version line may change.
- **Nothing is pushed, tagged or published.** No `git remote add`, `git push` or `git tag`, and nothing is posted to any forge.
- **No new Go dependency:** the GitHub client uses `net/http` and `encoding/json`.
- **Module path:** `github.com/Kevinbarrero/mr-impact` (the repo is `mr-impact`); the command, the binary, `cmd/mrimpact` and the archive names stay `mrimpact` (GoReleaser `project_name: mrimpact`). The Action is `Kevinbarrero/mr-impact@v0`.
- **Remote:** `origin` = `git@github.com:Kevinbarrero/mr-impact.git` is already configured. Its `main` holds one commit (`c5c4a3a`, "Initial commit": GitHub's Apache-2.0 `LICENSE`). Never push.
- **License:** Apache-2.0, with the appendix line `Copyright 2026 Kevinbarrero`.
- **Version:**
  - `cmd/mrimpact` keeps `var version = "dev"`, set by `-X main.version=v{{.Version}}`;
  - when it is `dev`, it comes from `runtime/debug.ReadBuildInfo().Main.Version` if that is not `(devel)` or empty;
  - the footer reads `mrimpact <version>`, with no extra `v`;
  - `--version` prints `mrimpact <version>` and exits 0.
- **GitHub API:**
  - list `GET {api}/repos/{owner}/{repo}/issues/{n}/comments?per_page=100` (follow `Link rel="next"`);
  - update `PATCH {api}/repos/{owner}/{repo}/issues/comments/{id}`;
  - create `POST {api}/repos/{owner}/{repo}/issues/{n}/comments`;
  - headers `Authorization: Bearer <token>`, `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28`.
- **The target comment** is the newest (highest `id`) whose body contains `report.Marker` and whose `user.login` equals `MRIMPACT_COMMENT_AUTHOR` (default `github-actions[bot]`). A refused `PATCH` (403/404) falls back to one `POST`.
- **Fork warning, verbatim:** `fork pull request: the token is read-only, note written to the job summary`.
- **Shallow-clone hint, verbatim:** `base commit not in clone: fetch full history (GitLab: GIT_DEPTH: 0; GitHub: actions/checkout with fetch-depth: 0)`.
- **Download URL default:** `https://github.com/Kevinbarrero/mr-impact/releases/download`, overridable by `MRIMPACT_DOWNLOAD_URL`.
- **Archive name:** `mrimpact_<version without v>_<os>_<arch>.tar.gz` (`.zip` on windows), plus `checksums.txt`. This is GoReleaser's default `name_template` with the project name.
- **Commit trailer, one line only:** `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **A PR comment carrying the marker but written by someone else.** It must never be edited, and a new bot comment is created instead. Test: Task 2 `TestUpsert_IgnoresMarkerFromOtherAuthor`.
2. **A fork PR.** No post is attempted, the warning appears, the summary is written, and the exit code is unaffected even with `--strict`. Test: Task 3 `TestRun_GitHubForkWritesSummaryOnly`.
3. **`--base` precedence.** The explicit flag wins over `CI_MERGE_REQUEST_DIFF_BASE_SHA`, which wins over the event's `base.sha`. A push event with no `--base` keeps today's "--base is required" usage error. Test: Task 3 `TestRun_BasePrecedence`.
4. **A checksum entry missing for the archive** (an archive name typo, or an unsupported arch): `install.sh` exits non-zero and installs nothing. It must never install unverified. Test: Task 4 `TestInstallScript_MissingChecksumEntry`.
5. **The module rename leaving the old path anywhere** (an import, a doc, a test expectation, a golden): `grep -r gitlab.com/wh1t3hat` returns nothing outside `docs/superpowers/` history files. Test: Task 1 Step 4.

---

### Task 1: Module rename and version

**Files:**
- Modify:
  - `go.mod` and every file that imports or prints `gitlab.com/wh1t3hat/mrimpact` (29 files today);
  - `internal/pipeline/pipeline.go:36-37,82` (drop `const Version`; take the version from `Config.Version`);
  - `internal/diff/git.go:17` (`ErrShallow` text = the shallow-clone hint);
  - `cmd/mrimpact/main.go` (`var version = "dev"`, `--version`, `resolveVersion`);
  - the e2e harness, which pins the version.
- Test: `cmd/mrimpact/main_test.go`, `internal/pipeline/e2e_test.go`.

**Interfaces:**
- Produces:
  - `pipeline.Config.Version string`, rendered as `mrimpact <Version>` in the footer;
  - in `cmd/mrimpact`: `var version = "dev"` and `func resolveVersion(v string, info func() (*debug.BuildInfo, bool)) string`.

- [ ] **Step 1: Write failing tests:**
  - `TestResolveVersion`: `("v0.3.0", any)` → `v0.3.0`; `("dev", build info v0.3.1)` → `v0.3.1`; `("dev", "(devel)")` → `dev`; `("dev", !ok)` → `dev`.
  - `TestMain_Version`: `--version` → stdout `mrimpact dev\n` (or whatever `resolveVersion` gives under `go test`), exit 0, no other output.
  - The footer test expects `mrimpact v9.9.9` when `Config.Version = "v9.9.9"`; e2e pins `Config.Version = "test"`.
- [ ] **Step 2:** Run `go test ./cmd/... ./internal/pipeline/ ./internal/report/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Rename with `go mod edit -module github.com/Kevinbarrero/mr-impact`, then rewrite the import paths.
  - Set the footer format to `mrimpact %s`.
  - Regenerate goldens with `-update` and read the diff: only the footer version line and the shallow hint may change.
- [ ] **Step 4:**
  - Run `go build ./... && go vet ./... && gofmt -l . && go test ./...`. Expected: PASS.
  - Run `grep -rn "gitlab.com/wh1t3hat" --exclude-dir=.git . | grep -v '^./docs/superpowers/'`. Expected: no output (Review Focus 5).
- [ ] **Step 5:** Commit `chore: module github.com/Kevinbarrero/mr-impact; version from the build`.

### Task 2: `internal/github` client

**Files:**
- Create: `internal/github/github.go`, `internal/github/github_test.go`

**Interfaces:**
- Produces:
  ```go
  type Client struct {
      BaseURL string              // GITHUB_API_URL, default https://api.github.com
      Token   string
      Author  string              // expected comment author; "" → "github-actions[bot]"
      HTTP    *http.Client
      Sleep   func(time.Duration) // nil → time.Sleep
  }
  func (c *Client) Upsert(ctx context.Context, repo string, number int, marker, body string) error // repo = "owner/name"
  ```
  The retry delays, the page cap (100 pages), the 30 s default timeout, the retry policy (5xx and transport errors retried; 4xx not) and the error style (`github: <METHOD> <path>: <status>: <snippet>`) copy `internal/gitlab`.

- [ ] **Step 1: Write failing tests** against an `httptest.Server`:
  - `TestUpsert_CreatesWhenNone`;
  - `TestUpsert_UpdatesNewestOwnMarker`: two own marker comments → `PATCH` on the higher id;
  - `TestUpsert_IgnoresMarkerFromOtherAuthor` (Review Focus 1): a marker comment by `mallory` only → `POST`, and no `PATCH` request is ever made;
  - `TestUpsert_FollowsLinkPagination`: the own marker comment is on page 2;
  - `TestUpsert_PatchRefusedFallsBackToCreate`: `PATCH` → 403, then exactly one `POST`;
  - `TestUpsert_Unauthorized`: the list returns 401 → an error containing `401`;
  - `TestUpsert_RetriesServerErrors`: 502 twice, then 200;
  - `TestUpsert_Headers`: the three headers on every request;
  - `TestUpsert_CustomAuthor`.
- [ ] **Step 2:** Run `go test ./internal/github/`. Expected: FAIL.
- [ ] **Step 3:** Implement. Parse `Link` for `rel="next"` and use that URL verbatim.
- [ ] **Step 4:** Run `go test ./internal/github/ && go vet ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(github): upsert a marker comment on a pull request`.

### Task 3: CI detection, posting and the job summary

**Files:**
- Create: `cmd/mrimpact/ci.go`, `cmd/mrimpact/ci_test.go`
- Modify: `cmd/mrimpact/main.go` (lines 85-94 and 141-151: posting through `poster`; the `--base` default)
- Test: `cmd/mrimpact/main_test.go`

**Interfaces:**
- Consumes: `github.Client.Upsert` (Task 2), `gitlab.Client.Upsert` (existing).
- Produces:
  ```go
  type poster interface{ Upsert(ctx context.Context, body string) error }
  type ciEnv struct {
      Kind     string // "gitlab", "github", "local"
      BaseSHA  string // GitHub: pull_request.base.sha
      Fork     bool
      Summary  string // GITHUB_STEP_SUMMARY path, "" when unset
      Poster   poster // nil in local mode; built with the token
  }
  func detectCI(env func(string) string, readFile func(string) ([]byte, error)) (ciEnv, error)
  ```
  - **Tests stub the clients:** `newGitHubClient` sits beside `newClient` as a replaceable `var`.
  - **Base default:**
    1. the `--base` flag;
    2. `CI_MERGE_REQUEST_DIFF_BASE_SHA`;
    3. `ciEnv.BaseSHA`;
    4. else the existing usage error.
  - **No token** keeps the GitLab warning text on GitLab. On GitHub it says `set MRIMPACT_TOKEN (the action passes github.token; needs permissions: pull-requests: write); comment not posted`.
  - **The GitHub client** gets `BaseURL` from `GITHUB_API_URL` (default `https://api.github.com`), `Author` from `MRIMPACT_COMMENT_AUTHOR`, and repo from `GITHUB_REPOSITORY`.
  - **The summary** is appended (create if missing, `0o644`) whenever `Summary != ""`, including with `--no-post`. A write failure warns but is not an internal failure.

- [ ] **Step 1: Write failing tests:**
  - `TestDetectCI`, table-driven:
    - the GitLab variables → `gitlab`;
    - a GitHub PR event → `github`, with `BaseSHA` and `Fork=false`;
    - a fork event → `Fork=true`;
    - a push event → `local`;
    - a missing or malformed event file → an error naming the path;
    - nothing → `local`;
    - GitLab and GitHub together → `gitlab`.
  - `TestRun_GitHubPostsComment`: the stubbed client receives `("owner/repo", 7, report.Marker, note)`; the summary file contains the note.
  - `TestRun_GitHubForkWritesSummaryOnly` (Review Focus 2): no `Upsert` call; stderr has the fork warning; summary written; exit 0 with `--strict`.
  - `TestRun_GitHubNoToken`: the warning text, the summary written, exit 0 with `--strict`.
  - `TestRun_GitHubPostFailureIsInternal`: `Upsert` returns an error → exit 2 with `--strict`, 0 without.
  - `TestRun_BasePrecedence` (Review Focus 3).
  - `TestRun_MalformedEventIsUsageError`: exit 2.
  - Existing GitLab tests unchanged.
- [ ] **Step 2:** Run `go test ./cmd/...`. Expected: FAIL.
- [ ] **Step 3:** Implement. The event JSON needs only `pull_request.number`, `.base.sha`, `.base.repo.full_name` and `.head.repo.full_name`.
- [ ] **Step 4:** Run `go test ./... && go vet ./... && gofmt -l .`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(cli): GitHub pull requests — detection, comment, job summary, fork-safe`.

### Task 4: `install.sh`

**Files:**
- Create: `install.sh` (executable), `install_test.go` (package `mrimpact_test` at the repo root, or `internal/installtest/` if a root test package clashes; say which in the report)

**Interfaces:**
- **Produces:** `install.sh [-b bindir] [version]`.
  - **Defaults:** bindir `./bin`, version `latest`.
  - **Platforms:** os from `uname -s` (`Linux`→`linux`, `Darwin`→`darwin`, `MINGW*|MSYS*|CYGWIN*`→`windows`); arch from `uname -m` (`x86_64|amd64`→`amd64`, `aarch64|arm64`→`arm64`). Anything else → exit 1 naming it.
  - **`latest`** resolves via the `Location` of `https://github.com/Kevinbarrero/mr-impact/releases/latest`, or `${MRIMPACT_LATEST_URL}` in tests.
  - **Download and verify:** `${MRIMPACT_DOWNLOAD_URL}/<tag>/<archive>` and `/<tag>/checksums.txt`, then verify with `sha256sum` or `shasum -a 256`.
  - **Messages and exits:** prints `mrimpact <tag> installed to <bindir>/mrimpact`; any failure → a message on stderr and exit 1, with nothing left in bindir.
  - **Tools:** `curl` or `wget`, `tar` or `unzip`. `MRIMPACT_OS`/`MRIMPACT_ARCH` override detection, for tests.

- [ ] **Step 1: Write failing tests** (skip when `sh`, `tar` or `sha256sum`/`shasum` are missing). The test builds `mrimpact_0.0.1_<goos>_<goarch>.tar.gz` holding a tiny script named `mrimpact`, plus `checksums.txt`, served by `httptest`:
  - `TestInstallScript_InstallsVerified`;
  - `TestInstallScript_ChecksumMismatch`: exit 1, the bindir has no `mrimpact`;
  - `TestInstallScript_MissingChecksumEntry` (Review Focus 4);
  - `TestInstallScript_Latest`: the latest URL redirects to `/tag/v0.0.1`;
  - `TestInstallScript_UnknownArch`: `MRIMPACT_ARCH=mips` → exit 1 naming it.
- [ ] **Step 2:** Run `go test -run InstallScript ./...`. Expected: FAIL.
- [ ] **Step 3:** Implement with `set -eu`, download into `mktemp -d`, and move into bindir only after verification.
- [ ] **Step 4:** Run the tests again, plus `sh -n install.sh` (and `shellcheck install.sh` if installed; otherwise note that it was skipped). Expected: PASS.
- [ ] **Step 5:** Commit `feat: install.sh — verified download of a release binary`.

### Task 5: GoReleaser, Action, GitLab template, project workflows

**Files:**
- Create:
  - `.goreleaser.yaml`;
  - `scripts/third-party-licenses.sh` (copies each `go.mod` require module's LICENSE* file from `go env GOMODCACHE` into `third_party_licenses/<module path>/`, for the GoReleaser `before.hooks`; `third_party_licenses/` is gitignored);
  - `action.yml`;
  - `ci/mrimpact.gitlab-ci.yml`;
  - `.github/workflows/ci.yml` and `.github/workflows/release.yml`;
  - `.github/dependabot.yml`;
  - `release_files_test.go` (or the same location as Task 4's test).
- Modify: `.gitignore` (create it: `third_party_licenses/`, `dist/`, `docs/launch/`, `mrimpact-report.md`).

**Interfaces:**
- **Consumes:** `install.sh` (Task 4); the CLI flags.
- **`action.yml`:**
  - inputs exactly as in spec §5 (`version`, `token`, `coverprofile`, `run-tests`, `fail-on`, `config`, `depth`, `tags`, `args`) and the output `report`;
  - `version` resolution: `github.action_ref` if it matches `^v[0-9]+\.[0-9]+\.[0-9]+$`, else `latest`;
  - it runs `install.sh -b "$RUNNER_TEMP/mrimpact"`, then mrimpact with `MRIMPACT_TOKEN: ${{ inputs.token }}` and `--out "$RUNNER_TEMP/mrimpact-report.md"`;
  - every input reaches the shell through `env:`, never `${{ }}` inside `run:` (script injection).
- **`release.yml`:** `goreleaser/goreleaser-action` pinned to a full release tag, `fetch-depth: 0`, `permissions: contents: write`, then `git tag -f v0 && git push -f origin v0`.
- **`ci.yml`:** test, vet and the gofmt check on push and pull_request. Then a PR-only job with `permissions: pull-requests: write` running `uses: ./` with `run-tests: true`.

- [ ] **Step 1: Write failing tests:**
  - `TestReleaseFiles_ParseAsYAML`: each file above unmarshals with yaml.v3;
  - `TestAction_InputsMatchSpec`: the input names are the set from spec §5, the `report` output exists, and no `run:` step contains `${{ inputs.`;
  - `TestGoreleaser_Targets`: goos `{linux, darwin, windows}`, goarch `{amd64, arm64}`, `CGO_ENABLED=0`, ldflags contain `-X main.version=v{{.Version}}`, and archives include `LICENSE`, `README.md`, `CHANGELOG.md`, `third_party_licenses/**`;
  - `TestGitLabTemplate_Variables`: the five variables with the spec defaults, and the rule on `merge_request_event`.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:**
  - Run `go test ./...` and `bash scripts/third-party-licenses.sh` (it must produce the licenses of x/tools, x/mod, x/sync and yaml.v3).
  - If `goreleaser` or `actionlint` can be installed offline, run `goreleaser check`, `goreleaser release --snapshot --clean` and `actionlint`; otherwise write in the report that they were skipped and why.
  - Expected: PASS.
- [ ] **Step 5:** Commit `feat: release automation — goreleaser, GitHub Action, GitLab template, CI workflows`.

### Task 6: GitHub-visible docs and legal files

**Files:**
- Create:
  - `LICENSE` = `git show origin/main:LICENSE` (GitHub's canonical Apache-2.0 text), with only the appendix line `Copyright [yyyy] [name of copyright owner]` replaced by `Copyright 2026 Kevinbarrero`;
  - `CHANGELOG.md`, `CONTRIBUTING.md`, `SECURITY.md`;
  - `.github/ISSUE_TEMPLATE/bug.yml`, `false-positive.yml`, `feature.yml`;
  - `docs/reference.md`.
- Modify: `README.md` (the rewrite; the gallery is filled in Task 7).

**Interfaces:**
- **`docs/reference.md`** holds, without loss, every section of today's README after its intro: local usage and flags, configuration, the sensitive table, architecture, the config line, exit codes, and the note's sections. It also gains the GitHub mode, the action inputs, the GitLab template variables, and the warning against `pull_request_target` (it runs untrusted PR code with a write token). The inferred-section wording becomes "no package outside `entry`".
- **`README.md`** follows spec §6 items 1–9 in order. The real note is a `<details>` block holding the Markdown of the dogfood-style run of mrimpact on its own last commit range (`--base` = the commit before Task 1). Badges point at `github.com/Kevinbarrero/mr-impact`. The gallery is a heading with a placeholder line that Task 7 replaces.
- **`CHANGELOG.md`:** an `## [Unreleased]` section, then `## [v0.3.0] - unreleased`, with one section each for Added, Changed and Fixed, summarising v1, the signal fixes, reach & focus, architecture and this release.

- [ ] **Step 1: Write a failing test,** `TestDocs_ReferenceKeepsReadmeSections`. It holds a list of the current README's `##`/`###` headings (copy them before editing) and checks each appears in `docs/reference.md` or the new README. It also checks that every flag in `mrimpact --help` appears in `docs/reference.md`.
- [ ] **Step 2:** Run it. Expected: FAIL.
- [ ] **Step 3:** Write the files. `diff <(git show origin/main:LICENSE) LICENSE` shows only the copyright line.
- [ ] **Step 4:** Run `go test ./...` and check that every link and path in README and reference resolves to a file in the repo or a `github.com/Kevinbarrero/mr-impact` URL. Expected: PASS.
- [ ] **Step 5:** Commit `docs: README for the first release, reference, license, changelog, contributing`.

### Task 7: Real examples on well-known repositories

**Files:**
- Create: `docs/examples/<repo>-<pr>.md` (3–4 files), `docs/examples/README.md`
- Modify: `README.md` (the gallery)

- [ ] **Step 1:**
  - Clone into a scratch directory outside the repo (never inside it): `gin-gonic/gin`, `spf13/cobra`, `caddyserver/caddy`, `prometheus/prometheus`.
  - For each, list recently merged PRs that change non-test `.go` files (`git log --merges --first-parent`, or a merge commit whose second parent is the PR head).
  - Pick 3–4 whose note is non-obvious: a route or handler reached, a red row, a sensitive API, or an architecture hint.
- [ ] **Step 2:**
  - For each, check out the PR head and run `go test ./... -coverprofile=c.out` when it finishes in under 5 minutes without external services; otherwise use no profile.
  - Then run `mrimpact --base <merge-base> --no-post --out note.md`.
  - Record the exact commands.
- [ ] **Step 3:**
  - Write each example file: repo, PR URL, `base..head`, the commands, the toolchain version, and the note unedited inside the file.
  - Write `docs/examples/README.md` as an index.
  - Fill the README gallery with one line per example saying what the note showed.
- [ ] **Step 4: Misfires.** For each row that is wrong on that code (a false red, a wrong entry point, a false violation):
  - a small fix with a clear rule gets a failing test in the owning package, then the fix, then one commit `fix(<pkg>): <rule> on <construct>`, and the example is re-run;
  - anything deeper is listed in README Limitations and in the task report as an issue for the next sub-project.
  - Nothing is posted to any of those projects.
- [ ] **Step 5:** Commit `docs: real examples on gin, cobra, caddy, prometheus` (name the repos actually used).

### Task 8: Launch drafts and the release dry run

**Files:**
- Create (gitignored, never committed): `docs/launch/show-hn.md`, `docs/launch/reddit-golang.md`, `docs/launch/golang-weekly-email.md`
- Modify: none committed unless a dry-run problem is found.

- [ ] **Step 1:**
  - Write the three drafts from the README pitch and the Task 7 examples.
  - Show HN: the title `Show HN: mrimpact – what a Go pull request really touches (offline, no AI)` and a ≤ 150-word first comment.
  - r/golang: the "why I built it" story, generic, with no employer details.
  - The email: 5 lines with a link.
- [ ] **Step 2:**
  - If GoReleaser is available, run `goreleaser release --snapshot --clean`. Extract the linux/amd64 archive and run `./mrimpact --version`; expect `mrimpact v0.3.0-SNAPSHOT-<sha>` or similar. Run it on this repo with `--no-post`.
  - Run `install.sh` against `dist/` served by a local `python3 -m http.server` or the Go test server.
  - If GoReleaser is unavailable, build with `go build -ldflags "-X main.version=v0.3.0"` and check `--version`.
- [ ] **Step 3:**
  - Fast-forward `main` to `feat/release`: `git checkout main && git merge --ff-only feat/release`. `main` is an ancestor today; if the fast-forward is refused, stop and report.
  - Join GitHub's initial commit so the first push is a fast-forward: `git fetch origin && git merge -s ours --allow-unrelated-histories origin/main -m "Merge GitHub's initial commit (LICENSE kept from this history)"`. Check `git diff HEAD~1 HEAD` is empty (the tree is unchanged).
  - Do not tag. Do not push.
- [ ] **Step 4:** Report the exact commands the owner runs to publish:
  2. `git push -u origin main`;
  3. set up the GitLab pull mirror;
  4. `git tag v0.3.0 && git push origin v0.3.0`;
  5. after the release workflow, publish the action to the Marketplace from the release page.
