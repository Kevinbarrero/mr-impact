<!-- mrimpact:v1 -->
### 🔍 MR impact — 21 functions changed
**🔴 3 · 🟡 12 · 🟢 6** · coverage: `static` · base `abcdef0`

👉 **Look at first:** `red.F00` (r) · `red.F01` (r) · `red.F02` (r) +12

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🔴 | `red.F00` | body | 3 | n/a |
| 🔴 | `red.F01` | body | 3 | n/a |
| 🔴 | `red.F02` | body | 3 | n/a |
| 🟡 | `yel.F00` | body | 3 | n/a |
| 🟡 | `yel.F01` | body | 3 | n/a |
| 🟡 | `yel.F02` | body | 3 | n/a |
| 🟡 | `yel.F03` | body | 3 | n/a |
| 🟡 | `yel.F04` | body | 3 | n/a |
| 🟡 | `yel.F05` | body | 3 | n/a |
| 🟡 | `yel.F06` | body | 3 | n/a |
| 🟡 | `yel.F07` | body | 3 | n/a |
| 🟡 | `yel.F08` | body | 3 | n/a |
| 🟡 | `yel.F09` | body | 3 | n/a |

<details><summary>8 more changed functions (🟡 2 · 🟢 6)</summary>

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🟡 | `yel.F10` | body | 3 | n/a |
| 🟡 | `yel.F11` | body | 3 | n/a |
| 🟢 | `grn.F00` | body | 3 | n/a |
| 🟢 | `grn.F01` | body | 3 | n/a |
| 🟢 | `grn.F02` | body | 3 | n/a |
| 🟢 | `grn.F03` | body | 3 | n/a |
| 🟢 | `grn.F04` | body | 3 | n/a |
| 🟢 | `grn.F05` | body | 3 | n/a |

</details>

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested, branch removed or added lines not measured · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🔴 red.F00"]:::red
  n1["cred.F00"]
  n2["cred.F01"]
  n3["cred.F02"]
  n4["🔴 red.F01"]:::red
  n5["🔴 red.F02"]:::red
  n6["🟡 yel.F00"]:::yellow
  n7["cyel.F00"]
  n8["cyel.F01"]
  n9["cyel.F02"]
  n10["🟡 yel.F01"]:::yellow
  n11["🟡 yel.F02"]:::yellow
  n12["🟡 yel.F03"]:::yellow
  n13["🟡 yel.F04"]:::yellow
  n14["🟡 yel.F05"]:::yellow
  n15["🟡 yel.F06"]:::yellow
  n16["🟡 yel.F07"]:::yellow
  n17["🟡 yel.F08"]:::yellow
  n18["🟡 yel.F09"]:::yellow
  n1 --> n0
  n2 --> n0
  n3 --> n0
  n1 --> n4
  n2 --> n4
  n3 --> n4
  n1 --> n5
  n2 --> n5
  n3 --> n5
  n7 --> n6
  n8 --> n6
  n9 --> n6
  n7 --> n10
  n8 --> n10
  n9 --> n10
  n7 --> n11
  n8 --> n11
  n9 --> n11
  n7 --> n12
  n8 --> n12
  n9 --> n12
  n7 --> n13
  n8 --> n13
  n9 --> n13
  n7 --> n14
  n8 --> n14
  n9 --> n14
  n7 --> n15
  n8 --> n15
  n9 --> n15
  n7 --> n16
  n8 --> n16
  n9 --> n16
  n7 --> n17
  n8 --> n17
  n9 --> n17
  n7 --> n18
  n8 --> n18
  n9 --> n18
  subgraph legend ["Legend"]
    direction LR
    lr["🔴 changed, not tested"]:::red
    ly["🟡 partly tested, branch removed or added lines not measured"]:::yellow
    lg["🟢 changed, tested"]:::green
    lc["caller, not changed"]
  end
  classDef red fill:#fdd,stroke:#c00
  classDef yellow fill:#ffd,stroke:#c90
  classDef green fill:#dfd,stroke:#2a2
```

<details><summary>All callers (39)</summary>

**`red.F00`** (3)

- depth 1: `cred.F00`, `cred.F01`, `cred.F02`

**`red.F01`** (3)

- depth 1: `cred.F00`, `cred.F01`, `cred.F02`

**`red.F02`** (3)

- depth 1: `cred.F00`, `cred.F01`, `cred.F02`

**`yel.F00`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

**`yel.F01`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

**`yel.F02`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

**`yel.F03`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

**`yel.F04`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

**`yel.F05`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

**`yel.F06`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

**`yel.F07`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

**`yel.F08`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

**`yel.F09`** (3)

- depth 1: `cyel.F00`, `cyel.F01`, `cyel.F02`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers (methods the standard library calls on interface values, such as `Error`, `Unwrap` or `ServeHTTP`, count as reached from the code that converts their type to an interface; see the list in the reference); where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, a branch removed, or added lines not measured · 🟢 covered, or no added lines and reached by a test · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴 · mrimpact v0.1.0 · 1.0 s</sub>
