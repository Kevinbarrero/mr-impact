# mrimpact shadow-mode validation protocol (pre-registered)

Pre-registered 2026-11-06, committed before any real MR was run. Copied verbatim from spec section 8.4.

### 8.4 Real-world validation (pre-registered)

- **Shadow mode:** ~20 real MRs at the author's workplace; artifact only, no posting. Per MR record: useful flag the human review missed or under-checked (reviewer agrees), false positives, runtime.
- **Success criteria (fixed before running):** ≥ 5/20 MRs with a useful flag; p90 runtime < 2 min; zero pipeline breakages caused by the tool.
- **Kill/pivot:** < 3/20 useful → rethink before adding features (graph view, LLM labels, GitHub).
- **Performance baseline:** time and peak memory on one medium-sized open-source Go service.

## Procedure

Add the job with `--no-post` to one service's CI and fill the table below over 20 MRs.

## Per-MR record

| MR | useful flag? | reviewer agrees? | false positives | runtime s | notes |
|----|--------------|------------------|-----------------|-----------|-------|

## Baseline

Repo: https://github.com/prometheus/alertmanager at `c409098c9adbbef9c9889bdaca1029d8c553e0ef` (full clone). Tool: mrimpact built from `07bbbf9`. Local mode, `--no-post`.
Machine: 8 cores (`nproc`), 20 GB RAM (MemTotal 20389740 kB), go1.26.0 linux/amd64.

| Run | Base | Changed functions | Wall time | Max RSS |
|---|---|---|---|---|
| 1 (first with this binary; module cache already warm) | `HEAD~20` (`cf75c5c`) | 34 | 1.52 s | 302664 kB (~296 MB) |
| 2 (baseline) | `HEAD~20` | 34 | 1.79 s | 314872 kB (~308 MB) |
| 3 | `HEAD~1` | 0 (no Go changes) | 0.01 s | 9728 kB |

Baseline = run 2: 34 changed functions, 1.79 s wall, ~308 MB peak RSS. This is far under the spec p90 < 2 min target (about 1.5% of it). Caveat: this is one repo and one range, not a p90.

Summary line: `MR impact — 34 functions changed` / `6 red, 0 yellow, 28 green`, coverage `static`.

Table excerpt:

```
| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🔴 | `dispatch.NewDispatcher` | signature (exported) | 7 (4 pkgs) | reached by test |
| 🔴 | `dispatch.(*Dispatcher).Groups` | signature (exported) | 6 (4 pkgs) | reached by test |
| 🔴 | `notify.(*PipelineBuilder).New` | signature (exported) | 5 (5 pkgs) | reached by test |
```

Note: the first attempt on this repo (earlier binary) analyzed 0 functions because `go:embed app/dist` made go/packages report an error and the whole analysis aborted; that surfaced the bug fixed in 07bbbf9. The module-download run was that earlier attempt (41.65 s wall, 503580 kB RSS), so the numbers above are warm-cache.
