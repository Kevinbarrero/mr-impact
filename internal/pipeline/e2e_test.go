package pipeline

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gitlab.com/wh1t3hat/mrimpact/internal/diff"
	"gitlab.com/wh1t3hat/mrimpact/internal/report"
	"gitlab.com/wh1t3hat/mrimpact/internal/risk"
	"gitlab.com/wh1t3hat/mrimpact/internal/testutil"
)

var update = flag.Bool("update", false, "rewrite expected.golden.md files")

// fixedBase replaces the per-run base SHA so goldens are stable.
const fixedBase = "0000000"

func TestE2E(t *testing.T) {
	cases := []struct {
		name     string
		profiles []string
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("..", "..", "testdata", "e2e", tc.name)
			repo, baseSHA, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
			now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
			m, err := Run(context.Background(), Config{
				Dir: repo, Base: baseSHA, Head: "HEAD", Depth: 2,
				CoverProfiles: tc.profiles, Timeout: 5 * time.Minute,
				Now: func() time.Time { return now },
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if m.BaseSHA != baseSHA {
				t.Errorf("BaseSHA = %q, want %q", m.BaseSHA, baseSHA)
			}
			checkCallers(t, filepath.Join(dir, "expected_callers.txt"), m)

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

func TestShallowBaseIsInternalFailure(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "e2e", "interface_call")
	repo, _, _ := testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	m, err := Run(context.Background(), Config{Dir: repo, Base: "1111111111111111111111111111111111111111"})
	if !errors.Is(err, diff.ErrShallow) {
		t.Fatalf("err = %v, want ErrShallow", err)
	}
	if m.Failure != diff.ErrShallow.Error() || m.Version != Version {
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
	m, err := Run(context.Background(), Config{Dir: repo, Base: "1111111111111111111111111111111111111111"})
	if !errors.Is(err, diff.ErrShallow) {
		t.Fatalf("err = %v, want ErrShallow", err)
	}
	assertNoLeak(t, m, err, diff.ErrShallow.Error())
}
