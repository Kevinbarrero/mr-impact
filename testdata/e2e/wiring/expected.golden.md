<!-- mrimpact:v1 -->
### 🔍 MR impact — 3 functions changed
**🔴 1 · 🟡 2 · 🟢 0** · coverage: `static` · base `0000000`

👉 **Look at first:** `lib.Build` (untested, 2 callers) · `lib.NewService` (wiring, untested) · `main.run` (wiring, untested)

All changes shown are reached from: main

| Risk | Function | Change | Callers | Entry points | Added-line coverage |
|---|---|---|---|---|---|
| 🔴 | `lib.Build` | body | 2 | api `lib.Build` | not reached |
| 🟡 | `lib.NewService` | body · wiring | 2 | api `lib.NewService` | not reached |
| 🟡 | `main.run` | body · wiring | 1 | — | not reached |

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested or a branch removed · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🔴 lib.Build"]:::red
  n1["🟡 lib.NewService"]:::yellow
  n2["🟡 main.run"]:::yellow
  n3["main.main"]
  n3 --> n2
  n2 --> n0
  n2 --> n1
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

<details><summary>All callers (5)</summary>

**`lib.Build`** (2)

- depth 1: `main.run`
- depth 2: `main.main`

**`lib.NewService`** (2)

- depth 1: `main.run`
- depth 2: `main.main`

**`main.run`** (1)

- depth 1: `main.main`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers (methods the standard library calls on interface values, such as `Error`, `Unwrap` or `ServeHTTP`, count as reached from the code that converts their type to an interface; see the list in the reference); where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, a branch removed, or added lines not measured · 🟢 covered, or no added lines and reached by a test · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴 · mrimpact test · 0.0 s</sub>
