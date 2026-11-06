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

(To be filled after the performance run.)
