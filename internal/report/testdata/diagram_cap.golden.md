<!-- mrimpact:v1 -->
### 🔍 MR impact — 1 functions changed
**🔴 1 · 🟡 0 · 🟢 0** · coverage: `static` · base `abcdef0`

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🔴 | `a.Target` | body | 30 | n/a |

```mermaid
graph RL
  n0["🔴 a.Target"]:::red
  n1["c.F00"]
  n2["c.F01"]
  n3["c.F02"]
  n4["c.F03"]
  n5["c.F04"]
  n6["c.F05"]
  n7["c.F06"]
  n8["c.F07"]
  n9["c.F08"]
  n10["c.F09"]
  n11["c.F10"]
  n12["c.F11"]
  n13["c.F12"]
  n14["c.F13"]
  n15["c.F14"]
  n16["c.F15"]
  n17["c.F16"]
  n18["c.F17"]
  n19["c.F18"]
  n1 --> n0
  n2 --> n0
  n3 --> n0
  n4 --> n0
  n5 --> n0
  n6 --> n0
  n7 --> n0
  n8 --> n0
  n9 --> n0
  n10 --> n0
  n11 --> n0
  n12 --> n0
  n13 --> n0
  n14 --> n0
  n15 --> n0
  n16 --> n0
  n17 --> n0
  n18 --> n0
  n19 --> n0
  classDef red fill:#fdd,stroke:#c00
  %% +11 not shown
```

+11 nodes not shown

<details><summary>All callers (30)</summary>

**`a.Target`** (30)

- depth 1: `c.F00`, `c.F01`, `c.F02`, `c.F03`, `c.F04`, `c.F05`, `c.F06`, `c.F07`, `c.F08`, `c.F09`
- and 20 more

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed · 🟡 partial, or untested with no callers · 🟢 covered · mrimpact v0.1.0 · 1.0 s</sub>
