<!-- mrimpact:v1 -->
### 🔍 MR impact — 6 functions changed
**🔴 1 · 🟡 1 · 🟢 1** · coverage: `coverprofile` · base `0000000`\
+3 new functions folded into 1 group

🏛 **Architecture (inferred — hints, not enforced)** · conformance n/a → n/a · 1/2 packages classified · layers: `cmd` → entry

<details><summary>Coupling (2 packages changed)</summary>

| Package | Ca | Ce | I | A | D |
|---|---|---|---|---|---|
| `cmd` | 0 → 0 | 0 → 1 | 0.00 → 1.00 | 0.00 → 0.00 | 1.00 → 0.00 |
| `svc` | 0 → 1 | 0 → 0 | 0.00 → 0.00 | 0.00 → 0.00 | 0.00 → 1.00 |

</details>

All changes shown are reached from: main

| Risk | Function | Change | Callers | Entry points | Added-line coverage |
|---|---|---|---|---|---|
| 🔴 | `svc.fmt` | new | 2 (2 pkgs) | api `svc.New` | 0 % (0/5) |
| 🟡 | `main.main` | body · wiring | 0 | — | no added lines · not reached |
| 🟢 | `svc.New` | new · +3 | 1 | api `svc.New` | 100 % (19/19) |

<details><summary>svc.New +3 new</summary>

- 🟢 `svc.(*S).apply` · new · 100 % (4/4) — `svc/svc.go`
- 🟢 `svc.load` · new · 100 % (5/5) — `svc/svc.go`
- 🟢 `svc.price` · new · 100 % (5/5) — `svc/svc.go`

</details>

📝 Declarations changed: `type S in svc/svc.go`

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested or a branch removed · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🔴 svc.fmt"]:::red
  n1["🟡 main.main"]:::yellow
  n2["🟢 svc.New"]:::green
  n2 --> n0
  n1 --> n2
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

**`svc.fmt`** (2)

- depth 1: `svc.New`
- depth 2: `main.main`

**`svc.New`** (1)

- depth 1: `main.main`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers; where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, or a branch removed · 🟢 covered (no added lines: reached by a test) · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴 · mrimpact v0.2.0 · 0.0 s</sub>
