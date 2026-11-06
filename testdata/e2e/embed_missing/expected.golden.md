<!-- mrimpact:v1 -->
### 🔍 MR impact — 1 functions changed
**🔴 1 · 🟡 0 · 🟢 0** · coverage: `static` · base `0000000`

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🔴 | `app.Greet` | body | 1 | not reached |

```mermaid
graph RL
  n0["🔴 app.Greet"]:::red
  n1["app.Serve"]
  n1 --> n0
  classDef red fill:#fdd,stroke:#c00
```

<details><summary>All callers (1)</summary>

**`app.Greet`** (1)

- depth 1: `app.Serve`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers.
- example.com/fx/ui: ui/ui.go:7:12: pattern dist: no matching files found
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed · 🟡 partial, or untested with no callers · 🟢 covered · mrimpact v0.1.0 · 0.0 s</sub>
