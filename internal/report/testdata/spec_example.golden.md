<!-- mrimpact:v1 -->
### 🔍 MR impact — 4 functions changed
**🔴 2 · 🟡 1 · 🟢 1** · coverage: `coverprofile` · base `a1b2c3d`

👉 **Look at first:** `pricing.(*Calculator).Total` · `orders.applyDiscount` · `orders.Validate`

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🔴 | `pricing.(*Calculator).Total` | signature (exported) | 14 (4 pkgs) | 0 % (0/12) |
| 🔴 | `orders.applyDiscount` | body | 5 | 0 % (0/7) |
| 🟡 | `orders.Validate` | body | 2 | 40 % (2/5) |
| 🟢 | `httpapi.formatDate` | new | 1 | 100 % (4/4) |

➖ Deleted: `legacy.OldTotal`

🧪 Tests touched in this MR: `orders/validate_test.go`

**How to read the diagram:** coloured boxes are functions this MR changed (🔴 not tested · 🟡 partly tested or a branch removed · 🟢 tested); plain boxes are callers the MR did not change; an arrow points from a caller to the function it calls; a dashed arrow means the function is passed as a callback (registered) and called through it. Every caller is listed under *All callers*.

```mermaid
flowchart RL
  n0["🔴 pricing.(*Calculator).Total"]:::red
  n1["billing.Generate"]
  n2["checkout.Handle"]
  n3["reports.F00"]
  n4(["+10 more callers"])
  n5["🔴 orders.applyDiscount"]:::red
  n6["orders.Create"]
  n7["orders.Import"]
  n8["orders.Replay"]
  n9(["+2 more callers"])
  n10["🟡 orders.Validate"]:::yellow
  n11["orders.F00"]
  n12["orders.F01"]
  n13["🟢 httpapi.formatDate"]:::green
  n14["httpapi.F00"]
  n1 --> n0
  n2 --> n0
  n3 --> n0
  n4 -.-> n0
  n6 --> n5
  n7 --> n5
  n8 --> n5
  n9 -.-> n5
  n11 --> n10
  n12 --> n10
  n14 --> n13
  n5 --> n0
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

<details><summary>All callers (22)</summary>

**`pricing.(*Calculator).Total`** (14)

- depth 1: `billing.Generate`, `checkout.Handle`, `orders.applyDiscount`, `reports.F00`, `reports.F01`, `reports.F02`, `reports.F03`, `reports.F04`, `reports.F05`, `reports.F06`
- and 4 more

**`orders.applyDiscount`** (5)

- depth 1: `orders.Create`, `orders.Import`, `orders.Replay`, `orders.Retry`, `orders.Update`

**`orders.Validate`** (2)

- depth 1: `orders.F00`, `orders.F01`

**`httpapi.formatDate`** (1)

- depth 1: `httpapi.F00`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers (methods the standard library calls on interface values, such as `Error`, `Unwrap` or `ServeHTTP`, count as reached from the code that converts their type to an interface; see the list in the reference); where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, a branch removed, or added lines not measured · 🟢 covered, or no added lines and reached by a test · wiring (straight-line setup) at most 🟡 · sensitive and untested 🔴 · mrimpact v0.1.0 · 3.2 s</sub>
