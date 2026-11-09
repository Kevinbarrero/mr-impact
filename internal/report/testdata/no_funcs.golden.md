<!-- mrimpact:v1 -->
### 🔍 MR impact — 0 functions changed
**🔴 0 · 🟡 0 · 🟢 0** · coverage: `static` · base `abcdef0`

➖ Deleted: `legacy.OldTotal`

📝 Declarations changed: `const Max in limits/limits.go`

🧪 Tests touched in this MR: `limits/limits_test.go`

<details><summary>⚠️ Limits of this analysis</summary>

- Calls via `reflect` or dynamic registration (e.g. handlers by name) are not detected.
- This repo only: callers in other services (HTTP/gRPC) are not visible.
- Functions called only from dependencies (e.g. HTTP handlers registered with net/http) or from package-level initializers show no callers; where such a function is registered is listed under Registered.
- Now unused: uses through `reflect`, `//go:linkname` or other repositories are not seen; exported methods, and exported functions of packages other modules can import, are never listed.
</details>

<sub>Rules: 🔴 untested with callers, or exported signature changed in an importable package · 🟡 partial, untested with no callers, or a branch removed · 🟢 covered (no added lines: reached by a test) · wiring (straight-line setup) at most 🟡 · mrimpact v0.1.0 · 1.0 s</sub>
