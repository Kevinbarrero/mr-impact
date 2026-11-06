<!-- mrimpact:v1 -->
### 🔍 MR impact — 4 functions changed
**🔴 2 · 🟡 1 · 🟢 1** · coverage: `coverprofile` · base `a1b2c3d`

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🔴 | `pricing.(*Calculator).Total` | signature (exported) | 14 (4 pkgs) | 0 % (0/12) |
| 🔴 | `orders.applyDiscount` | body | 5 | 0 % (0/7) |
| 🟡 | `orders.Validate` | body | 2 | 40 % (2/5) |
| 🟢 | `httpapi.formatDate` | new | 1 | 100 % (4/4) |

➖ Deleted: `legacy.OldTotal`
🧪 Tests touched in this MR: `orders/validate_test.go`

```mermaid
graph RL
  n0["🔴 pricing.(*Calculator).Total"]:::red
  n1["billing.Generate"]
  n2["checkout.Handle"]
  n3["🔴 orders.applyDiscount"]:::red
  n4["reports.F00"]
  n5["reports.F01"]
  n6["reports.F02"]
  n7["reports.F03"]
  n8["reports.F04"]
  n9["reports.F05"]
  n10["reports.F06"]
  n11["reports.F07"]
  n12["reports.F08"]
  n13["reports.F09"]
  n14["reports.F10"]
  n15["orders.Create"]
  n16["orders.Import"]
  n17["orders.Replay"]
  n18["orders.Retry"]
  n19["orders.Update"]
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
  n15 --> n3
  n16 --> n3
  n17 --> n3
  n18 --> n3
  n19 --> n3
  classDef red fill:#fdd,stroke:#c00
  %% +5 not shown
```

+5 nodes not shown

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
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed · 🟡 partial, or untested with no callers · 🟢 covered · mrimpact v0.1.0 · 3.2 s</sub>
