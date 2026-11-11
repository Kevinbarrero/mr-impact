package pipeline

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Kevinbarrero/mr-impact/internal/arch"
	"github.com/Kevinbarrero/mr-impact/internal/config"
	"github.com/Kevinbarrero/mr-impact/internal/diff"
	"github.com/Kevinbarrero/mr-impact/internal/report"
	"github.com/Kevinbarrero/mr-impact/internal/risk"
	"github.com/Kevinbarrero/mr-impact/internal/sensitive"
	"github.com/Kevinbarrero/mr-impact/internal/testutil"
)

var update = flag.Bool("update", false, "rewrite expected.golden.md files")

// fixedBase replaces the per-run base SHA so goldens are stable.
const fixedBase = "0000000"

func TestE2E(t *testing.T) {
	cases := []struct {
		name     string
		profiles []string
		check    func(t *testing.T, m report.Model) // extra assertions beyond the golden
	}{
		{name: "interface_call"},
		{name: "exported_signature"},
		{name: "deleted_func"},
		{name: "callback_value"},
		{name: "build_tags"},
		{name: "head_broken"},
		{name: "embed_missing"},
		{name: "decl_change"},
		{name: "no_go_changes"},
		{name: "coverprofile", profiles: []string{"coverage.out"}},
		{name: "removal"},
		{name: "cross_pkg_coverage", profiles: []string{"coverage.out"}},
		{name: "wiring"},
		{name: "new_subtree", profiles: []string{"coverage.out"}},
		{name: "sensitive_zero_config", check: checkSensitiveZeroConfig},
		{name: "config_file", check: checkConfigFile},
		{name: "arch_inferred", check: checkArchInferred},
		{name: "arch_hexagonal", check: checkArchHexagonal},
		{name: "arch_modular", check: checkArchModular},
		{name: "registration_middleware", profiles: []string{"coverage.out"}, check: checkRegistrationMiddleware},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("..", "..", "testdata", "e2e", tc.name)
			repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
			now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
			// As main does: the optional .mrimpact.yml at the module root.
			fc, err := config.Load(repo, "")
			if err != nil {
				t.Fatal(err)
			}
			table, err := sensitive.NewTable(fc.SensitiveAdd, fc.SensitiveRemove)
			if err != nil {
				t.Fatal(err)
			}
			am, err := arch.FromConfig(fc.Architecture)
			if err != nil {
				t.Fatal(err)
			}
			m, err := Run(context.Background(), Config{
				Dir: repo, Base: baseSHA, Head: "HEAD", Depth: 2, Version: "test",
				CoverProfiles: tc.profiles, Timeout: 5 * time.Minute,
				Now:       func() time.Time { return now },
				Sensitive: &table, Critical: fc.Critical,
				File: &fc, FileName: ".mrimpact.yml", FileRel: ".mrimpact.yml",
				Arch: &am,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if m.BaseSHA != baseSHA {
				t.Errorf("BaseSHA = %q, want %q", m.BaseSHA, baseSHA)
			}
			checkCallers(t, filepath.Join(dir, "expected_callers.txt"), m)
			if tc.check != nil {
				tc.check(t, m)
			}

			m.BaseSHA = fixedBase
			got := report.Render(m)
			golden := filepath.Join(dir, "expected.golden.md")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if got != string(want) {
				t.Errorf("report mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", tc.name, got, want)
			}
		})
	}
}

func rowOf(t *testing.T, m report.Model, display string) report.FuncRow {
	t.Helper()
	for _, r := range m.Funcs {
		if r.Display == display {
			return r
		}
	}
	t.Fatalf("no row %s", display)
	return report.FuncRow{}
}

func sensitiveOf(m report.Model, display string) (report.SensitiveRow, bool) {
	for _, r := range m.Sensitive {
		if r.Display == display {
			return r, true
		}
	}
	return report.SensitiveRow{}, false
}

func checkSensitiveZeroConfig(t *testing.T, m report.Model) {
	if r := rowOf(t, m, "run.Exec"); r.Level != risk.Red || r.Kind != "new · sensitive" {
		t.Errorf("run.Exec = %v %q, want red \"new · sensitive\"", r.Level, r.Kind)
	}
	if s, ok := sensitiveOf(m, "run.Exec"); !ok || !s.Untested || s.APIs[0] != "os/exec.Command" {
		t.Errorf("run.Exec section row = %+v, %v", s, ok)
	}
	if r := rowOf(t, m, "store.Save"); r.Level != risk.Green || r.Kind != "body · sensitive" {
		t.Errorf("store.Save = %v %q, want green \"body · sensitive\"", r.Level, r.Kind)
	}
	if s, ok := sensitiveOf(m, "store.Save"); !ok || s.Untested || strings.Join(s.APIs, ",") != "database/sql.(*DB).ExecContext" {
		t.Errorf("store.Save section row = %+v, %v", s, ok)
	}
	if _, ok := sensitiveOf(m, "util.Label"); ok || rowOf(t, m, "util.Label").Kind != "body" {
		t.Error("util.Label must not be sensitive")
	}
	out := report.Render(m)
	if i, j := strings.Index(out, "`run.Exec` —"), strings.Index(out, "`store.Save` —"); i < 0 || j < i {
		t.Errorf("run.Exec must be listed first in the section:\n%s", out)
	}
}

func checkConfigFile(t *testing.T, m report.Model) {
	if r := rowOf(t, m, "pay.Settle"); r.Level != risk.Red || r.Kind != "body · sensitive" {
		t.Errorf("pay.Settle = %v %q, want red \"body · sensitive\"", r.Level, r.Kind)
	}
	if s, ok := sensitiveOf(m, "pay.Settle"); !ok || s.Critical != "internal/pay/..." || len(s.APIs) != 0 {
		t.Errorf("pay.Settle section row = %+v, %v", s, ok)
	}
	if _, ok := sensitiveOf(m, "describe.Tool"); ok || strings.Contains(rowOf(t, m, "describe.Tool").Kind, "sensitive") {
		t.Error("describe.Tool calls only os/exec, which the config removed: must not be sensitive")
	}
	// The base config had the critical glob; the MR added the removal.
	if c := m.Config; c == nil || !c.Changed || c.BaseUnreadable || len(c.Changes) != 1 ||
		c.Changes[0].What != "removes from defaults" || strings.Join(c.Changes[0].Items, ",") != "os/exec.*" {
		t.Errorf("Config = %+v", m.Config)
	}
	const line = "⚙️ .mrimpact.yml: critical `internal/pay/...` · removes `os/exec.*` — changed in this MR (MR removes from defaults: `os/exec.*`)"
	if !strings.Contains(report.Render(m), "\n"+line+"\n") {
		t.Errorf("missing config line %q", line)
	}
}

// checkRegistrationMiddleware: a middleware added to a route is one changed
// registration; make(map…) keeps NewService wiring (capped 🟡 although it is
// untested with a caller); an added line away from db.Ping is not sensitive.
func checkRegistrationMiddleware(t *testing.T, m report.Model) {
	if r := rowOf(t, m, "svc.NewService"); r.Level != risk.Yellow || r.Kind != "body · wiring" {
		t.Errorf("svc.NewService = %v %q, want yellow \"body · wiring\"", r.Level, r.Kind)
	}
	if r := rowOf(t, m, "store.Check"); strings.Contains(r.Kind, "sensitive") {
		t.Errorf("store.Check kind %q: the added line is not on the db.Ping call", r.Kind)
	}
	if _, ok := sensitiveOf(m, "store.Check"); ok {
		t.Error("store.Check must not be listed under 🔐")
	}
	out := report.Render(m)
	for _, want := range []string{
		"\n- `GET …/trade/:id` → `api.GetTrade`: + `mw` — `api/api.go:21`\n",
		"| `svc.(*Service).Lookup` | body | 1 | `GET …/trade/:id`,",
		"\n- `svc.(*Service).Lookup` — `svc/svc.go:14-16`\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "Registrations removed") || strings.Contains(out, "Registrations added") {
		t.Errorf("the changed registration is also listed as removed or added:\n%s", out)
	}
}

// checkCallers asserts that every row in m.Funcs is declared in path as
// "Display: caller, caller" and that its callers match exactly. Lines
// starting with '#' are comments (known over-approximations are recorded
// as "# over-approx:" comments next to the entry they affect).
func checkCallers(t *testing.T, path string, m report.Model) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	want := map[string][]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fn, list, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("%s: bad line %q", path, line)
		}
		var callers []string
		for _, c := range strings.Split(list, ",") {
			if c = strings.TrimSpace(c); c != "" {
				callers = append(callers, c)
			}
		}
		sort.Strings(callers)
		want[strings.TrimSpace(fn)] = callers
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	got := map[string][]string{}
	for _, r := range m.Funcs {
		var callers []string
		for _, c := range r.Callers {
			callers = append(callers, c.Display)
		}
		sort.Strings(callers)
		got[r.Display] = callers
	}
	for fn, w := range want {
		g, ok := got[fn]
		if !ok {
			t.Errorf("expected function %s not in report (have %v)", fn, keys(got))
			continue
		}
		if strings.Join(g, ", ") != strings.Join(w, ", ") {
			t.Errorf("callers of %s = [%s], want [%s]", fn, strings.Join(g, ", "), strings.Join(w, ", "))
		}
	}
	for fn := range got {
		if _, ok := want[fn]; !ok {
			t.Errorf("function %s in report but not declared in %s", fn, path)
		}
	}
}

func keys(m map[string][]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestTimeoutDegrades(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "interface_call")
	repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA, Depth: 2, Timeout: time.Nanosecond})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !m.Degraded || len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "timeout") {
		t.Fatalf("Degraded=%v Warnings=%q, want degraded with a timeout warning", m.Degraded, m.Warnings)
	}
	if len(m.Funcs) != 1 {
		t.Fatalf("Funcs = %+v, want 1 row", m.Funcs)
	}
	r := m.Funcs[0]
	// Callers are unknown, so an untested func is Yellow, never Red.
	if len(r.Callers) != 0 || r.CoverText != "n/a" || r.Level != risk.Yellow {
		t.Errorf("row = %+v, want no callers, n/a, yellow", r)
	}
}

// TestRun_DegradedGraphNoGroups: without a call graph nothing is known
// about callers, so nothing is folded and every function keeps its row.
func TestRun_DegradedGraphNoGroups(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "new_subtree")
	repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA, Depth: 2, Timeout: time.Nanosecond})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !m.Degraded {
		t.Fatal("Degraded = false, want the call graph timeout")
	}
	if m.Folded != 0 || m.Groups != 0 {
		t.Errorf("Folded=%d Groups=%d, want 0", m.Folded, m.Groups)
	}
	for _, r := range m.Funcs {
		if len(r.Members) > 0 {
			t.Errorf("row %s has members %+v", r.Display, r.Members)
		}
	}
	if len(m.Funcs) != 6 {
		t.Errorf("Funcs = %d rows, want 6 (main.main and the five new svc functions)", len(m.Funcs))
	}
}

func TestShallowBaseIsInternalFailure(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "interface_call")
	repo, _, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	m, err := Run(context.Background(), Config{Dir: repo, Base: "1111111111111111111111111111111111111111", Version: "v1.2.3"})
	if !errors.Is(err, diff.ErrShallow) {
		t.Fatalf("err = %v, want ErrShallow", err)
	}
	if m.Failure != diff.ErrShallow.Error() || m.Version != "v1.2.3" {
		t.Errorf("Failure=%q Version=%q, want ErrShallow's text and the version", m.Failure, m.Version)
	}
}

func TestStaleProfileFallsBackToStatic(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "interface_call")
	repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	stale := filepath.Join(t.TempDir(), "stale.out")
	if err := os.WriteFile(stale, []byte("mode: set\nexample.com/fx/store/gone.go:1.1,2.2 1 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA, CoverProfiles: []string{stale}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if m.Coverage != "static" || !m.Degraded || len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "stale") {
		t.Fatalf("Coverage=%q Degraded=%v Warnings=%q, want static fallback with a stale warning", m.Coverage, m.Degraded, m.Warnings)
	}
	if len(m.Funcs) != 1 || m.Funcs[0].CoverText != "not reached" {
		t.Errorf("Funcs = %+v, want one static row", m.Funcs)
	}
}

func TestHeadParseErrorIsBuildFailure(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	mod := []byte("module example.com/fx\n\ngo 1.26.0\n")
	for dir, src := range map[string]string{
		base: "package calc\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n",
		head: "package calc\n\nfunc Add(a, b int) int {\n\treturn a +\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), mod, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo, baseSHA, _ := testutil.MakeRepo(t, base, head)
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := "build errors in `calc.go`"; m.Failure != want {
		t.Errorf("Failure = %q, want %q", m.Failure, want)
	}
}

// Local mode diffs against merge-base(base, head): a function changed only on
// the base branch after head branched off is not part of the MR.
func TestDiffsAgainstMergeBase(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	const src = "package calc\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\nfunc Sub(a, b int) int {\n\treturn a - b\n}\n"
	for dir, s := range map[string]string{base: src, head: strings.Replace(src, "a + b", "b + a", 1)} {
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fx\n\ngo 1.26.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo, forkSHA, _ := testutil.MakeRepo(t, base, head)
	moved := testutil.AdvanceBase(t, repo, forkSHA, map[string]string{
		"calc.go": strings.Replace(src, "a - b", "-b + a", 1),
	})
	m, err := Run(context.Background(), Config{Dir: repo, Base: moved})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if m.BaseSHA != forkSHA {
		t.Errorf("BaseSHA = %s, want merge-base %s", m.BaseSHA, forkSHA)
	}
	if len(m.Funcs) != 1 || m.Funcs[0].Display != "calc.Add" || m.Funcs[0].Kind != "body" {
		t.Errorf("Funcs = %+v, want only calc.Add (body)", m.Funcs)
	}
}

const secretURL = "https://user:secret@host/repo"

// assertNoLeak checks that err carries the secret (it goes to the job log)
// while the public Failure and rendered note do not.
func assertNoLeak(t *testing.T, m report.Model, err error, wantFailure string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v, want the full error including the stderr text", err)
	}
	if m.Failure != wantFailure {
		t.Errorf("Failure = %q, want %q", m.Failure, wantFailure)
	}
	if out := report.Render(m); strings.Contains(out, "secret") {
		t.Errorf("rendered note leaks the secret:\n%s", out)
	}
}

func TestGitErrorDoesNotLeakIntoNote(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "interface_call")
	repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	// git diff fails and echoes the bad revision on stderr.
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA, Head: secretURL})
	assertNoLeak(t, m, err, "internal error (see job log)")
}

func TestLoadErrorDoesNotLeakIntoNote(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	mods := map[string]string{
		base: "module example.com/fx\n\ngo 1.26.0\n",
		// go.mod treats "//" as a comment, so only the userinfo part fits;
		// go list echoes the bad version on stderr.
		head: "module example.com/fx\n\ngo user:secret@host\n",
	}
	for dir, mod := range mods {
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
			t.Fatal(err)
		}
		src := "package fx\n\nfunc F() int { return " + map[string]string{base: "1", head: "2"}[dir] + " }\n"
		if err := os.WriteFile(filepath.Join(dir, "fx.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo, baseSHA, _ := testutil.MakeRepo(t, base, head)
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA})
	assertNoLeak(t, m, err, "no Go packages could be loaded (see job log)")
}

// The failed fetch's stderr goes to the job log via err, while the note keeps
// ErrShallow's fixed text.
func TestShallowFetchErrorDoesNotLeakIntoNote(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "interface_call")
	repo, _, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	// git echoes the remote path on stderr when it is not a repository.
	if out, err := exec.Command("git", "-C", repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "user:secret@host")).CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	m, err := Run(context.Background(), Config{Dir: repo, Base: "1111111111111111111111111111111111111111", Version: "v1.2.3"})
	if !errors.Is(err, diff.ErrShallow) {
		t.Fatalf("err = %v, want ErrShallow", err)
	}
	assertNoLeak(t, m, err, diff.ErrShallow.Error())
}

// A library caller that sets no table gets the default one.
func TestRun_NilSensitiveUsesDefaults(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "sensitive_zero_config")
	repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA})
	if err != nil {
		t.Fatal(err)
	}
	if r := rowOf(t, m, "run.Exec"); r.Level != risk.Red || r.Kind != "new · sensitive" {
		t.Errorf("run.Exec = %v %q, want red \"new · sensitive\"", r.Level, r.Kind)
	}
}

// TestRun_DegradedStillSensitive: without a call graph and a profile, test
// reach is unknown: sensitive functions are still listed in the section but
// the untested-sensitive rule does not rate them, and no entry points are
// shown.
func TestRun_DegradedStillSensitive(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "sensitive_zero_config")
	repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	table, err := sensitive.NewTable(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA, Depth: 2, Timeout: time.Nanosecond, Sensitive: &table})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !m.Degraded {
		t.Fatal("Degraded = false, want the call graph timeout")
	}
	s, ok := sensitiveOf(m, "run.Exec")
	if !ok {
		t.Fatal("run.Exec not listed in the sensitive section")
	}
	if s.Untested {
		t.Error("run.Exec: untested-sensitive rule fired with test reach unknown")
	}
	// Today's rule for an unknown reach with no callers known: yellow.
	if r := rowOf(t, m, "run.Exec"); r.Level != risk.Yellow {
		t.Errorf("run.Exec = %v, want yellow (today's rating)", r.Level)
	}
	for _, r := range m.Funcs {
		if len(r.Entries) > 0 {
			t.Errorf("row %s has entries %+v", r.Display, r.Entries)
		}
	}
}

// main and init touching sensitive APIs are listed under 🔐 but keep
// today's rating: the untested-sensitive rule does not apply to them.
func TestRun_SensitiveRootExempt(t *testing.T) {
	write := func(dir, name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base, head := t.TempDir(), t.TempDir()
	for _, d := range []string{base, head} {
		write(d, "go.mod", "module example.com/root\n\ngo 1.26\n")
	}
	write(base, "main.go", "package main\n\nfunc init() {}\n\nfunc main() {}\n")
	write(head, "main.go", `package main

import (
	"os"
	"os/exec"
)

func init() {
	for _, a := range os.Args[1:] {
		_ = os.Remove(a)
	}
}

func main() {
	if len(os.Args) > 1 {
		_ = exec.Command(os.Args[1]).Run()
	}
}
`)
	repo, baseSHA, _ := testutil.MakeRepo(t, base, head)
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"main.main", "main.init"} {
		s, ok := sensitiveOf(m, d)
		if !ok {
			t.Errorf("%s not listed under 🔐", d)
			continue
		}
		if s.Untested {
			t.Errorf("%s: untested-sensitive rule applied", d)
		}
		if r := rowOf(t, m, d); r.Level != risk.Yellow || strings.Contains(r.Kind, "wiring") {
			t.Errorf("%s = %v, want yellow, not wiring (today's rule: untested, no callers)", d, r.Level)
		}
	}
}

// A base config that does not parse never fails the run: the line says the
// file changed and the base is unreadable.
func TestRun_ConfigBaseUnreadable(t *testing.T) {
	write := func(dir, name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base, head := t.TempDir(), t.TempDir()
	for _, d := range []string{base, head} {
		write(d, "go.mod", "module example.com/cfg\n\ngo 1.26\n")
	}
	write(base, "a.go", "package cfg\n\nfunc F() int { return 1 }\n")
	write(head, "a.go", "package cfg\n\nfunc F() int { return 2 }\n")
	write(base, ".mrimpact.yml", "critcal: [x]\n")
	write(head, ".mrimpact.yml", "sensitive:\n  remove: [os/exec.*]\n")
	repo, baseSHA, _ := testutil.MakeRepo(t, base, head)
	fc, err := config.Load(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA, File: &fc, FileName: ".mrimpact.yml", FileRel: ".mrimpact.yml"})
	if err != nil {
		t.Fatal(err)
	}
	const line = "⚙️ .mrimpact.yml: removes `os/exec.*` — changed in this MR (base config unreadable)"
	if out := report.Render(m); !strings.Contains(out, "\n"+line+"\n") {
		t.Errorf("missing %q in\n%s", line, out)
	}
}

// A critical glob that matches no loaded package is called out in the
// note's warnings: it is most likely a typo.
func TestRun_CriticalGlobMatchesNothing(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "config_file")
	repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA, Critical: []string{"internal/pay/...", "internal/pyment/..."}})
	if err != nil {
		t.Fatal(err)
	}
	want := `critical glob "internal/pyment/..." matches no package`
	var n int
	for _, w := range m.Warnings {
		if strings.Contains(w, "critical glob") {
			n++
			if w != want {
				t.Errorf("warning %q, want %q", w, want)
			}
		}
	}
	if n != 1 {
		t.Errorf("warnings = %q, want exactly one critical-glob warning", m.Warnings)
	}
}

// archNew renders the new violations as "from -> to file:line" lines.
func archNew(a *arch.Result) []string {
	var out []string
	for _, v := range a.New {
		out = append(out, fmt.Sprintf("%s -> %s %s:%d iso=%v", v.FromLayer, v.ToLayer, v.Edge.File, v.Edge.Line, v.Isolated))
	}
	return out
}

func checkArchInferred(t *testing.T, m report.Model) {
	a := m.Arch
	if a == nil || !a.Inferred || a.BaseMissing {
		t.Fatalf("Arch = %+v, want an inferred result with a base", a)
	}
	// order_test.go imports internal/service/clock (core -> application):
	// test variants are not in the graph, so neither base nor head counts it.
	if got := strings.Join(archNew(a), "; "); got != "core -> adapters internal/domain/order/order.go:3 iso=false" || a.Existing != 0 {
		t.Errorf("New = %s, Existing = %d", got, a.Existing)
	}
}

func checkArchHexagonal(t *testing.T, m report.Model) {
	a := m.Arch
	if a == nil || a.Inferred || a.Model != "hexagonal" {
		t.Fatalf("Arch = %+v, want the declared hexagonal model", a)
	}
	if got := strings.Join(archNew(a), "; "); got != "adapters -> adapters internal/adapters/http/h.go:4 iso=true" || a.Existing != 1 {
		t.Errorf("New = %s, Existing = %d", got, a.Existing)
	}
}

func checkArchModular(t *testing.T, m report.Model) {
	a := m.Arch
	if a == nil || a.Model != "modular" {
		t.Fatalf("Arch = %+v, want the modular model", a)
	}
	// billing -> orders/store is new; billing -> orders (root) stays allowed.
	if got := strings.Join(archNew(a), "; "); got != "billing -> orders internal/billing/x/x.go:5 iso=false" || a.Existing != 0 {
		t.Errorf("New = %s, Existing = %d", got, a.Existing)
	}
}

// Review Focus 4: a base revision that fails to load does not stop the run;
// the section shows head conformance and lists no violations.
func TestRun_ArchBaseNotLoaded(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "arch_hexagonal")
	base := t.TempDir()
	if err := os.CopyFS(base, os.DirFS(filepath.Join(dir, "base"))); err != nil {
		t.Fatal(err)
	}
	// go list rejects the module file: the base load fails, head is fine.
	if err := os.WriteFile(filepath.Join(base, "go.mod"), []byte("module example.com/hx\n\ngo 1.26.0\nrequire (\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, baseSHA, _ := testutil.MakeRepo(t, base, filepath.Join(dir, "head"))
	am, _ := arch.Preset("hexagonal")
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA, Arch: &am})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	a := m.Arch
	if a == nil || !a.BaseMissing || a.HeadConf == nil || a.BaseConf != nil || len(a.New) != 0 || len(a.Coupling) != 0 {
		t.Fatalf("Arch = %+v, want BaseMissing with head conformance only", a)
	}
	out := report.Render(m)
	const want = "🏛 **Architecture** (hexagonal) · conformance 60.0 % · 5/5 packages classified · (base not loaded: new violations not computed)\n\n"
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in\n%s", want, out)
	}
	if strings.Contains(out, "❌") {
		t.Errorf("violations listed without a base:\n%s", out)
	}
	if !strings.Contains(out, "Registrations not compared: the base revision could not be loaded.") {
		t.Errorf("the base did not fail to load:\n%s", out)
	}
}

// A library caller that sets no model gets the inferred one.
func TestRun_NilArchIsInferred(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "arch_inferred")
	repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA})
	if err != nil {
		t.Fatal(err)
	}
	if m.Arch == nil || !m.Arch.Inferred || len(m.Arch.New) != 1 {
		t.Errorf("Arch = %+v, want the inferred result", m.Arch)
	}
}

// A package that only lost a file (deleted, or moved away by a rename) is
// still touched: its coupling change is reported.
func TestRun_ArchTouchedByDeletedAndRenamedFiles(t *testing.T) {
	write := func(dir, name, src string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Long enough that git -M pairs the moved file despite its new package clause.
	body := "\nimport \"example.com/tp/internal/repository/c\"\n\n"
	for i := range 20 {
		body += fmt.Sprintf("// Line %d of a file that moves to another package unchanged.\n", i)
	}
	body += "func Moved() int { return c.C() }\n"
	base, head := t.TempDir(), t.TempDir()
	for _, d := range []string{base, head} {
		write(d, "go.mod", "module example.com/tp\n\ngo 1.26\n")
		write(d, "internal/repository/c/c.go", "package c\n\nfunc C() int { return 1 }\n")
		write(d, "internal/service/a/a.go", "package a\n\nfunc A() int { return 1 }\n")
		write(d, "internal/service/d/d.go", "package d\n\nfunc D() int { return 1 }\n")
	}
	write(base, "internal/service/a/b.go", "package a\n\nimport \"example.com/tp/internal/repository/c\"\n\nfunc B() int { return c.C() }\n")
	write(base, "internal/service/d/moved.go", "package d\n"+body)
	write(head, "internal/service/e/moved.go", "package e\n"+body)
	repo, baseSHA, _ := testutil.MakeRepo(t, base, head)
	m, err := Run(context.Background(), Config{Dir: repo, Base: baseSHA})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range m.Arch.Coupling {
		got[c.Pkg] = fmt.Sprintf("Ce %d→%d new=%v", c.Base.Ce, c.Head.Ce, c.New)
	}
	want := map[string]string{
		"example.com/tp/internal/service/a": "Ce 1→0 new=false", // b.go deleted
		"example.com/tp/internal/service/d": "Ce 1→0 new=false", // moved.go renamed away
		"example.com/tp/internal/service/e": "Ce 0→1 new=true",
	}
	for p, w := range want {
		if got[p] != w {
			t.Errorf("%s: %q, want %q (coupling %v)", p, got[p], w, got)
		}
	}
}
