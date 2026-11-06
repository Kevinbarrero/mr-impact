<!-- mrimpact:v1 -->
### 🔍 MR impact — 5 functions changed
**🔴 1 · 🟡 3 · 🟢 1** · coverage: `coverprofile` · base `0000000`

| Risk | Function | Change | Callers | Added-line coverage |
|---|---|---|---|---|
| 🔴 | `grade.Report` | body | 1 | 0 % (0/2) |
| 🟡 | `grade.Bonus` | new | 0 | 80 % (4/5) |
| 🟡 | `grade.Card` | new | 0 | 0 % (0/3) |
| 🟡 | `grade.Unused` | new | 0 | 0 % (0/2) |
| 🟢 | `grade.Letter` | body | 2 | 100 % (3/3) |

🧪 Tests touched in this MR: `grade/grade_test.go`

```mermaid
graph RL
  n0["🔴 grade.Report"]:::red
  n1["🟡 grade.Card"]:::yellow
  n2["🟡 grade.Bonus"]:::yellow
  n3["🟡 grade.Unused"]:::yellow
  n4["🟢 grade.Letter"]:::green
  n1 --> n0
  n0 --> n4
  classDef red fill:#fdd,stroke:#c00
  classDef yellow fill:#ffd,stroke:#c90
  classDef green fill:#dfd,stroke:#2a2
```

<details><summary>All callers (3)</summary>

**`grade.Report`** (1)

- depth 1: `grade.Card`

**`grade.Letter`** (2)

- depth 1: `grade.Report`
- depth 2: `grade.Card`

</details>

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed · 🟡 partial, or untested with no callers · 🟢 covered · mrimpact v0.1.0 · 0.0 s</sub>
