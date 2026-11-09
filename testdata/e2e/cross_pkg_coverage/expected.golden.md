<!-- mrimpact:v1 -->
### 🔍 MR impact — 1 functions changed
**🔴 0 · 🟡 1 · 🟢 0** · coverage: `coverprofile` · base `0000000`

ℹ️ 1 functions are covered only by tests in other packages; if the coverprofile was made without -coverpkg it counts same-package tests only. Run `go test -coverpkg=./... -coverprofile=…` for line coverage.

| Risk | Function | Change | Callers | Entry points | Added-line coverage |
|---|---|---|---|---|---|
| 🟡 | `a.F` | body | 1 | api `a.F` | 0 % in profile · reached by tests in b |

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested or a branch removed · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🟡 a.F"]:::yellow
  n1["main.main"]
  n1 --> n0
  subgraph legend ["Legend"]
    direction LR
    lr["🔴 changed, not tested"]:::red
    ly["🟡 partly tested / branch removed"]:::yellow
    lg["🟢 changed, tested"]:::green
    lc["caller, not changed"]
  end
  classDef red fill:#fdd,stroke:#c00
  classDef yellow fill:#ffd,stroke:#c90
  classDef green fill:#dfd,stroke:#2a2
```

<details><summary>All callers (1)</summary>

**`a.F`** (1)

- depth 1: `main.main`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers; where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, or a branch removed · 🟢 covered (no added lines: reached by a test) · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴 · mrimpact v0.2.0 · 0.0 s</sub>
