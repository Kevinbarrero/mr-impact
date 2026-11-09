<!-- mrimpact:v1 -->
### 🔍 MR impact — 6 functions changed
**🔴 0 · 🟡 2 · 🟢 0** · coverage: `coverprofile` · base `0000000`\
+4 new functions folded into 1 groups

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🟡 | `svc.New` | new · +4 | 1 | 79 % (19/24) · 1 untested |
| 🟡 | `main.main` | body · wiring | 0 | no added lines · not reached |

<details><summary>svc.New +4 new</summary>

- 🟢 `svc.(*S).apply` · new · 100 % (4/4) — `svc/svc.go`
- 🔴 `svc.fmt` · new · 0 % (0/5) — `svc/svc.go`
- 🟢 `svc.load` · new · 100 % (5/5) — `svc/svc.go`
- 🟢 `svc.price` · new · 100 % (5/5) — `svc/svc.go`

</details>

📝 Declarations changed: `type S in svc/svc.go`

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested or a branch removed · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🟡 svc.New"]:::yellow
  n1["🟡 main.main"]:::yellow
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

**`svc.New`** (1)

- depth 1: `main.main`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers; where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, or a branch removed · 🟢 covered (no added lines: reached by a test) · wiring (straight-line setup) at most 🟡 · mrimpact v0.2.0 · 0.0 s</sub>
