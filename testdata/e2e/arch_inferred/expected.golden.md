<!-- mrimpact:v1 -->
### 🔍 MR impact — 1 functions changed
**🔴 0 · 🟡 1 · 🟢 0** · coverage: `static` · base `0000000`

👉 **Look at first:** `order.Total` (added lines not measured)

🏛 **Architecture (inferred — hints, not enforced)** · conformance 100.0 % → 50.0 % · 4/4 packages classified · layers: `internal/domain/order` → core

- ❌ new: `core → adapters` — `internal/domain/order/order.go:3` imports `internal/repository/pg`

<details><summary>Coupling (1 package changed)</summary>

| Package | Ca | Ce | I | A | D |
|---|---|---|---|---|---|
| `internal/domain/order` | 1 → 1 | 0 → 1 | 0.00 → 0.50 | 0.00 → 0.00 | 1.00 → 0.50 |

</details>

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🟡 | `order.Total` | body | 1 | reached by test · added lines not measured |

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested, branch removed or added lines not measured · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🟡 order.Total"]:::yellow
  n1["checkout.Checkout"]
  n1 --> n0
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

<details><summary>All callers (1)</summary>

**`order.Total`** (1)

- depth 1: `checkout.Checkout`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers (methods the standard library calls on interface values, such as `Error`, `Unwrap` or `ServeHTTP`, count as reached from the code that converts their type to an interface; see the list in the reference); where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, a branch removed, or added lines not measured · 🟢 covered, or no added lines and reached by a test · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴 · mrimpact test · 0.0 s</sub>
