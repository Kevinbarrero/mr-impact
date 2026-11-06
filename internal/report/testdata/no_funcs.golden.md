<!-- mrimpact:v1 -->
### 🔍 MR impact — 0 functions changed
**🔴 0 · 🟡 0 · 🟢 0** · coverage: `static` · base `abcdef0`

➖ Deleted: `legacy.OldTotal`
📝 Declarations changed: `const Max in limits/limits.go`
🧪 Tests touched in this MR: `limits/limits_test.go`

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed · 🟡 partial, or untested with no callers · 🟢 covered · mrimpact v0.1.0 · 1.0 s</sub>
