<!-- mrimpact:v1 -->
### 🔍 MR impact — 4 functions changed
**🔴 0 · 🟡 1 · 🟢 1** · coverage: `coverprofile` · base `abcdef0`\
+2 new functions folded into 1 group

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🟡 | `svc.New` | new · +2 | 1 | 67 % (4/6) · 1 untested |
| 🟢 | `svc.Old` | body | 1 | 100 % (1/1) |

<details><summary>svc.New +2 new</summary>

- 🟢 `svc.load` · new · 100 % (2/2) — `svc/load.go`
- 🟡 `svc.fmt` · new · 0 % (0/2) — `svc/fmt.go`

</details>

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested or a branch removed · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🟡 svc.New"]:::yellow
  n1["main.main"]
  n2["🟢 svc.Old"]:::green
  n1 --> n0
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

<details><summary>All callers (2)</summary>

**`svc.New`** (1)

- depth 1: `main.main`

**`svc.Old`** (1)

- depth 1: `main.main`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers (methods the standard library calls on interface values, such as `Error`, `Unwrap` or `ServeHTTP`, count as reached from the code that converts their type to an interface; see the list in the reference); where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, a branch removed, or added lines not measured · 🟢 covered, or no added lines and reached by a test · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴 · mrimpact v0.1.0 · 1.0 s</sub>
