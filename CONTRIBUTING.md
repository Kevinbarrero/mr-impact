# Contributing to mrimpact

Thank you for helping. Bug reports, false positives and false negatives (a note that says something untrue, or misses something true) are as valuable as code.

## Build and test

You need Go 1.26 or newer and `git`.

```sh
go build ./cmd/mrimpact
go test ./...
go vet ./...
gofmt -l .        # must print nothing
```

CI runs the same commands on every push and pull request, then runs mrimpact on the pull request itself.

To see the note for your own branch:

```sh
go run ./cmd/mrimpact --base origin/main --no-post --out /tmp/note.md
```

## Goldens

The end-to-end tests in `internal/pipeline` render whole notes for the fixture repositories in `testdata/` and compare them with `expected.golden.md` files. When a change alters the note on purpose, regenerate them:

```sh
go test ./internal/pipeline/ -run TestE2E -update
git diff testdata/
```

Then read the diff, line by line. Every changed line of a golden must be one you meant to change; commit the goldens with the code that changed them.

## Rules for new checks

A check belongs in mrimpact when it keeps these properties:

- **General Go.** It works on any Go module, from the language, the standard library and the type-checked program. Nothing specific to one project, framework or company; a library-specific rule needs a widely used library and a default that is safe for everyone else.
- **Deterministic and offline.** The same repository and revisions give the same note. No network calls during analysis, no clocks or randomness in the output.
- **No AI.** No language models, no API keys, no tokens other than the forge token used to post.
- **Zero-config first.** It runs with useful defaults and no file. Configuration may add project knowledge, never be required. Config parsing stays strict.
- **Honest in the note.** It says what it does not see (under the note's limits) rather than guessing, and it comes with tests, including the case where it must stay silent.

For a larger change, open an issue first so the design can be discussed before the code.

## Pull requests

- Keep each pull request to one change, with tests.
- Use short, imperative commit subjects (`fix(report): …`, `feat(arch): …`).
- Update `docs/reference.md` when the note, a flag or the config changes, and add a line under `## [Unreleased]` in `CHANGELOG.md`.

## License

Contributions are accepted under the [Apache License 2.0](LICENSE), the project's license: what you submit is licensed to everyone under the same terms (inbound = outbound, as in section 5 of the license). There is no CLA to sign.
