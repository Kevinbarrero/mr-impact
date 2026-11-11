<!-- mrimpact:v1 -->
### 🔍 MR impact — 60 functions changed
**🔴 0 · 🟡 0 · 🟢 60** · coverage: `static` · base `abcdef0`

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🟢 | `p.F00` | body | 0 | n/a |
| 🟢 | `p.F01` | body | 0 | n/a |
| 🟢 | `p.F02` | body | 0 | n/a |
| 🟢 | `p.F03` | body | 0 | n/a |
| 🟢 | `p.F04` | body | 0 | n/a |
| 🟢 | `p.F05` | body | 0 | n/a |
| 🟢 | `p.F06` | body | 0 | n/a |
| 🟢 | `p.F07` | body | 0 | n/a |
| 🟢 | `p.F08` | body | 0 | n/a |
| 🟢 | `p.F09` | body | 0 | n/a |
| 🟢 | `p.F10` | body | 0 | n/a |
| 🟢 | `p.F11` | body | 0 | n/a |
| 🟢 | `p.F12` | body | 0 | n/a |
| 🟢 | `p.F13` | body | 0 | n/a |
| 🟢 | `p.F14` | body | 0 | n/a |
| 🟢 | `p.F15` | body | 0 | n/a |
| 🟢 | `p.F16` | body | 0 | n/a |
| 🟢 | `p.F17` | body | 0 | n/a |
| 🟢 | `p.F18` | body | 0 | n/a |
| 🟢 | `p.F19` | body | 0 | n/a |
| 🟢 | `p.F20` | body | 0 | n/a |
| 🟢 | `p.F21` | body | 0 | n/a |
| 🟢 | `p.F22` | body | 0 | n/a |
| 🟢 | `p.F23` | body | 0 | n/a |
| 🟢 | `p.F24` | body | 0 | n/a |
| 🟢 | `p.F25` | body | 0 | n/a |
| 🟢 | `p.F26` | body | 0 | n/a |
| 🟢 | `p.F27` | body | 0 | n/a |
| 🟢 | `p.F28` | body | 0 | n/a |
| 🟢 | `p.F29` | body | 0 | n/a |
| 🟢 | `p.F30` | body | 0 | n/a |
| 🟢 | `p.F31` | body | 0 | n/a |
| 🟢 | `p.F32` | body | 0 | n/a |
| 🟢 | `p.F33` | body | 0 | n/a |
| 🟢 | `p.F34` | body | 0 | n/a |
| 🟢 | `p.F35` | body | 0 | n/a |
| 🟢 | `p.F36` | body | 0 | n/a |
| 🟢 | `p.F37` | body | 0 | n/a |
| 🟢 | `p.F38` | body | 0 | n/a |
| 🟢 | `p.F39` | body | 0 | n/a |
| 🟢 | `p.F40` | body | 0 | n/a |
| 🟢 | `p.F41` | body | 0 | n/a |
| 🟢 | `p.F42` | body | 0 | n/a |
| 🟢 | `p.F43` | body | 0 | n/a |
| 🟢 | `p.F44` | body | 0 | n/a |
| 🟢 | `p.F45` | body | 0 | n/a |
| 🟢 | `p.F46` | body | 0 | n/a |
| 🟢 | `p.F47` | body | 0 | n/a |
| 🟢 | `p.F48` | body | 0 | n/a |
| 🟢 | `p.F49` | body | 0 | n/a |

_… and 10 more_

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested or a branch removed · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🟢 p.F00"]:::green
  n1["🟢 p.F01"]:::green
  n2["🟢 p.F02"]:::green
  n3["🟢 p.F03"]:::green
  n4["🟢 p.F04"]:::green
  n5["🟢 p.F05"]:::green
  n6["🟢 p.F06"]:::green
  n7["🟢 p.F07"]:::green
  n8["🟢 p.F08"]:::green
  n9["🟢 p.F09"]:::green
  n10["🟢 p.F10"]:::green
  n11["🟢 p.F11"]:::green
  n12["🟢 p.F12"]:::green
  n13["🟢 p.F13"]:::green
  n14["🟢 p.F14"]:::green
  n15["🟢 p.F15"]:::green
  n16["🟢 p.F16"]:::green
  n17["🟢 p.F17"]:::green
  n18["🟢 p.F18"]:::green
  n19["🟢 p.F19"]:::green
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

40 more changed functions are not drawn; see the table.

<details><summary>All callers (0)</summary>

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers; where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, a branch removed, or added lines not measured · 🟢 covered, or no added lines and reached by a test · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴 · mrimpact v0.1.0 · 1.0 s</sub>
