<!-- mrimpact:v1 -->
### 🔍 MR impact — 1 functions changed
**🔴 1 · 🟡 0 · 🟢 0** · coverage: `static` · base `0000000`

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🔴 | `store.(*Mem).Get` | body | 2 (2 pkgs) | not reached |

```mermaid
graph RL
  n0["🔴 store.(*Mem).Get"]:::red
  n1["api.Handle"]
  n2["main.main"]
  n1 --> n0
  n2 --> n1
  classDef red fill:#fdd,stroke:#c00
```

<details><summary>All callers (2)</summary>

**`store.(*Mem).Get`** (2)

- depth 1: `api.Handle`
- depth 2: `main.main`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed · 🟡 partial, or untested with no callers · 🟢 covered · mrimpact v0.1.0 · 0.0 s</sub>
