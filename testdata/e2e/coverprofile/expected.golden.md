<!-- mrimpact:v1 -->
### 🔍 MR impact — 5 functions changed
**🔴 1 · 🟡 3 · 🟢 1** · coverage: `coverprofile` · base `0000000`

| Risk | Function | Change | Callers | Entry points | Added-line coverage |
|---|---|---|---|---|---|
| 🔴 | `grade.Report` | body | 1 | api grade.Card, api grade.Report | 0 % (0/2) |
| 🟡 | `grade.Bonus` | new | 0 | api grade.Bonus | 80 % (4/5) |
| 🟡 | `grade.Card` | new | 0 | api grade.Card | 0 % (0/3) |
| 🟡 | `grade.Unused` | new | 0 | api grade.Unused | 0 % (0/2) |
| 🟢 | `grade.Letter` | body | 2 | api grade.Card, api grade.Letter +1 | 100 % (3/3) |

🧪 Tests touched in this MR: `grade/grade_test.go`

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested or a branch removed · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🔴 grade.Report"]:::red
  n1["🟡 grade.Bonus"]:::yellow
  n2["🟡 grade.Card"]:::yellow
  n3["🟡 grade.Unused"]:::yellow
  n4["🟢 grade.Letter"]:::green
  n2 --> n0
  n0 --> n4
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

<details><summary>All callers (3)</summary>

**`grade.Report`** (1)

- depth 1: `grade.Card`

**`grade.Letter`** (2)

- depth 1: `grade.Report`
- depth 2: `grade.Card`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers; where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, or a branch removed · 🟢 covered (no added lines: reached by a test) · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴 · mrimpact v0.2.0 · 0.0 s</sub>
