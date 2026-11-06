<!-- mrimpact:v1 -->
### 🔍 MR impact — 2 functions changed
**🔴 1 · 🟡 0 · 🟢 1** · coverage: `static` · base `0000000`

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🔴 | `pricing.Total` | signature (exported) | 1 | reached by test |
| 🟢 | `orders.Create` | body | 0 | reached by test |

```mermaid
graph RL
  n0["🔴 pricing.Total"]:::red
  n1["🟢 orders.Create"]:::green
  n1 --> n0
  classDef red fill:#fdd,stroke:#c00
  classDef green fill:#dfd,stroke:#2a2
```

<details><summary>All callers (1)</summary>

**`pricing.Total`** (1)

- depth 1: `orders.Create`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed · 🟡 partial, or untested with no callers · 🟢 covered · mrimpact v0.1.0 · 0.0 s</sub>
