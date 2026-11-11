# Examples

Notes mrimpact wrote on merged pull requests of well-known Go projects, run locally from a clone at the pull request's head with `--no-post`. Each file gives the range, the exact commands, the toolchain, the coverage mode and a short reading, then the note unedited.

| Example | Pull request | Coverage | What the note showed |
|---|---|---|---|
| [gin-4800.md](gin-4800.md) | [gin-gonic/gin#4800](https://github.com/gin-gonic/gin/pull/4800) | coverprofile | Three `net/http.Server` call sites under 🔐, the `ginS` wrappers reached, and a callers list that leaves out `RunUnix`, which kept no `ReadHeaderTimeout` |
| [cobra-1956.md](cobra-1956.md) | [spf13/cobra#1956](https://github.com/spf13/cobra/pull/1956) | coverprofile | The one function still executing `text/template`, reached only from the `Set*Template` API; exported getters with untested changed lines |
| [caddy-8346.md](caddy-8346.md) | [caddyserver/caddy#8346](https://github.com/caddyserver/caddy/pull/8346) | coverprofile | A 🔴 row that is 84 % covered: an exported signature change in an importable package, which breaks third-party modules |
| [prometheus-20028.md](prometheus-20028.md) | [prometheus/prometheus#20028](https://github.com/prometheus/prometheus/pull/20028) | static | New HTTP routes, an exported constructor's signature change, an inferred architecture hint; and one misfire: a registration call with new arguments reported as removed and added (fixed in v0.3.2: it now shows under "Registrations changed") |

Nothing was posted to these projects.
