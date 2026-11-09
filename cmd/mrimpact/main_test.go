package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/wh1t3hat/mrimpact/internal/gitlab"
	"gitlab.com/wh1t3hat/mrimpact/internal/testutil"
)

func init() {
	newClient = func(baseURL, token string) *gitlab.Client {
		return &gitlab.Client{BaseURL: baseURL, Token: token, HTTP: http.DefaultClient, Sleep: func(time.Duration) {}}
	}
}

type env map[string]string

func (e env) get(k string) string { return e[k] }

func fixture(t *testing.T, name string) (repo, base string) {
	t.Helper()
	dir := filepath.Join("..", "..", "testdata", "e2e", name)
	repo, base, _ = testutil.MakeRepo(t, filepath.Join(dir, "base"), filepath.Join(dir, "head"))
	return repo, base
}

func server(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if r.Method == http.MethodGet {
			w.Write([]byte("[]"))
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("{}"))
	}))
	t.Cleanup(s.Close)
	return s, &n
}

func exec(t *testing.T, e env, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var o, er bytes.Buffer
	code = run(context.Background(), args, e.get, &o, &er)
	return code, o.String(), er.String()
}

func ciEnv(url string) env {
	return env{"CI_MERGE_REQUEST_IID": "7", "CI_PROJECT_ID": "42", "CI_API_V4_URL": url, "MRIMPACT_TOKEN": "tok"}
}

func TestLocalModeNoPost(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	_, n := server(t, 0)
	out := filepath.Join(t.TempDir(), "r.md")
	code, stdout, stderr := exec(t, env{}, "--dir", repo, "--base", base, "--out", out)
	if code != 0 || n.Load() != 0 {
		t.Fatalf("code=%d requests=%d stderr=%s", code, n.Load(), stderr)
	}
	b, err := os.ReadFile(out)
	if err != nil || string(b) != stdout || !strings.Contains(stdout, "<!-- mrimpact:v1 -->") {
		t.Fatalf("report file mismatch: %v", err)
	}
}

func TestPostsInMRPipeline(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	s, n := server(t, 0)
	code, _, stderr := exec(t, ciEnv(s.URL), "--dir", repo, "--base", base, "--out", filepath.Join(t.TempDir(), "r.md"))
	if code != 0 || n.Load() != 2 {
		t.Fatalf("code=%d requests=%d stderr=%s", code, n.Load(), stderr)
	}
}

func TestBaseFromEnv(t *testing.T) {
	repo, base := fixture(t, "no_go_changes")
	e := env{"CI_MERGE_REQUEST_DIFF_BASE_SHA": base}
	if code, _, stderr := exec(t, e, "--dir", repo, "--out", filepath.Join(t.TempDir(), "r.md")); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}

func TestNoPostFlag(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	s, n := server(t, 0)
	code, _, _ := exec(t, ciEnv(s.URL), "--dir", repo, "--base", base, "--no-post", "--out", filepath.Join(t.TempDir(), "r.md"))
	if code != 0 || n.Load() != 0 {
		t.Fatalf("code=%d requests=%d", code, n.Load())
	}
}

func TestSkipEmpty(t *testing.T) {
	repo, base := fixture(t, "no_go_changes")
	s, n := server(t, 0)
	out := filepath.Join(t.TempDir(), "r.md")
	code, _, _ := exec(t, ciEnv(s.URL), "--dir", repo, "--base", base, "--skip-empty", "--out", out)
	if code != 0 || n.Load() != 0 {
		t.Fatalf("code=%d requests=%d", code, n.Load())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal("report file should still be written")
	}
	// Without --skip-empty it posts.
	exec(t, ciEnv(s.URL), "--dir", repo, "--base", base, "--out", out)
	if n.Load() == 0 {
		t.Fatal("expected a post without --skip-empty")
	}
}

func TestFailOnRed(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	out := filepath.Join(t.TempDir(), "r.md")
	if code, _, stderr := exec(t, env{}, "--dir", repo, "--base", base, "--fail-on", "red", "--out", out); code != 1 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if code, _, _ := exec(t, env{}, "--dir", repo, "--base", base, "--out", out); code != 0 {
		t.Fatalf("without --fail-on code=%d", code)
	}
}

func TestStrictOnPostFailure(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	s, _ := server(t, http.StatusForbidden)
	out := filepath.Join(t.TempDir(), "r.md")
	code, _, stderr := exec(t, ciEnv(s.URL), "--dir", repo, "--base", base, "--strict", "--out", out)
	if code != 2 || !strings.Contains(stderr, "mrimpact: ") {
		t.Fatalf("strict: code=%d stderr=%s", code, stderr)
	}
	if code, _, _ := exec(t, ciEnv(s.URL), "--dir", repo, "--base", base, "--out", out); code != 0 {
		t.Fatalf("non-strict code=%d", code)
	}
}

func TestStrictOnPipelineFailure(t *testing.T) {
	repo, _ := fixture(t, "exported_signature")
	out := filepath.Join(t.TempDir(), "r.md")
	code, stdout, _ := exec(t, env{}, "--dir", repo, "--base", "deadbeefdeadbeef", "--strict", "--out", out)
	if code != 2 || !strings.Contains(stdout, "<!-- mrimpact:v1 -->") {
		t.Fatalf("code=%d stdout=%s", code, stdout)
	}
	if code, _, _ := exec(t, env{}, "--dir", repo, "--base", "deadbeefdeadbeef", "--out", out); code != 0 {
		t.Fatalf("non-strict code=%d", code)
	}
}

func TestOutWriteFailureStrict(t *testing.T) {
	repo, base := fixture(t, "no_go_changes")
	bad := filepath.Join(t.TempDir(), "missing", "r.md")
	code, _, stderr := exec(t, env{}, "--dir", repo, "--base", base, "--out", bad)
	if code != 0 || !strings.Contains(stderr, "cannot write") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if code, _, _ := exec(t, env{}, "--dir", repo, "--base", base, "--out", bad, "--strict"); code != 2 {
		t.Fatalf("strict code=%d", code)
	}
}

func TestUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"missing base": {},
		"bad fail-on":  {"--base", "x", "--fail-on", "yellow"},
		"depth 0":      {"--base", "x", "--depth", "0"},
		"timeout 0":    {"--base", "x", "--timeout", "0s"},
		"unknown flag": {"--base", "x", "--bogus"},
		"positional":   {"--base", "x", "extra"},
	}
	for name, args := range cases {
		if code, _, _ := exec(t, env{}, args...); code != 2 {
			t.Errorf("%s: code=%d, want 2", name, code)
		}
	}
	e := ciEnv("http://127.0.0.1:1")
	e["CI_MERGE_REQUEST_IID"] = "abc"
	if code, _, stderr := exec(t, e, "--base", "x"); code != 2 || !strings.Contains(stderr, "CI_MERGE_REQUEST_IID") {
		t.Errorf("bad iid: code=%d stderr=%s", code, stderr)
	}
}

func TestMissingBase(t *testing.T) {
	code, _, stderr := exec(t, env{})
	if code != 2 || !strings.Contains(stderr, "--base") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}

func TestMissingToken(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	s, n := server(t, 0)
	e := ciEnv(s.URL)
	delete(e, "MRIMPACT_TOKEN")
	code, _, stderr := exec(t, e, "--dir", repo, "--base", base, "--out", filepath.Join(t.TempDir(), "r.md"))
	if code != 0 || n.Load() != 0 || !strings.Contains(stderr, "MRIMPACT_TOKEN") {
		t.Fatalf("code=%d requests=%d stderr=%s", code, n.Load(), stderr)
	}
}

// A missing token is a configuration warning, not an internal failure: the note
// is not posted and --strict still exits 0 (spec §7).
func TestMissingTokenStrict(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	s, n := server(t, 0)
	e := ciEnv(s.URL)
	delete(e, "MRIMPACT_TOKEN")
	code, _, stderr := exec(t, e, "--strict", "--dir", repo, "--base", base, "--out", filepath.Join(t.TempDir(), "r.md"))
	if code != 0 || n.Load() != 0 || !strings.Contains(stderr, "MRIMPACT_TOKEN") {
		t.Fatalf("code=%d requests=%d stderr=%s", code, n.Load(), stderr)
	}
}

func TestMain_InvalidConfigExits2(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	if err := os.WriteFile(filepath.Join(repo, ".mrimpact.yml"), []byte("critcal: [x]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "r.md")
	code, stdout, stderr := exec(t, env{}, "--dir", repo, "--base", base, "--out", out)
	if code != 2 || !strings.Contains(stderr, "critcal") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !strings.HasPrefix(stderr, "mrimpact: config: ") || strings.Count(stderr, "\n") != 1 || strings.Contains(stderr, "  ") {
		t.Errorf("want one collapsed line \"mrimpact: config: …\", got %q", stderr)
	}
	if stdout != "" {
		t.Errorf("a note was printed: %q", stdout)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("--out written despite the config error: %v", err)
	}
}

func TestMain_ExplicitMissingConfigExits2(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	out := filepath.Join(t.TempDir(), "r.md")
	missing := filepath.Join(t.TempDir(), "nope.yml")
	code, _, stderr := exec(t, env{}, "--dir", repo, "--base", base, "--config", missing, "--out", out)
	if code != 2 || !strings.HasPrefix(stderr, "mrimpact: config: ") || !strings.Contains(stderr, "nope.yml") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("--out written despite the config error: %v", err)
	}
}

func TestMain_BadSensitivePatternExits2(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	cfg := filepath.Join(t.TempDir(), "c.yml")
	if err := os.WriteFile(cfg, []byte("sensitive:\n  remove: [os.Exit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := exec(t, env{}, "--dir", repo, "--base", base, "--config", cfg, "--out", filepath.Join(t.TempDir(), "r.md"))
	if code != 2 || !strings.HasPrefix(stderr, "mrimpact: config: ") || !strings.Contains(stderr, "os.Exit") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

// Review Focus 4: .mrimpact.yml is read from --dir, not the working directory.
func TestRun_ConfigReadFromDir(t *testing.T) {
	repo, base := fixture(t, "config_file")
	out := filepath.Join(t.TempDir(), "r.md")
	elsewhere := t.TempDir()
	// A broken file in the working directory: reading it would exit 2.
	if err := os.WriteFile(filepath.Join(elsewhere, ".mrimpact.yml"), []byte("critcal: [x]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(elsewhere)
	code, stdout, stderr := exec(t, env{}, "--dir", repo, "--base", base, "--out", out)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"| 🔴 | `pay.Settle` | body · sensitive |",
		"- 🔴 `pay.Settle` — critical: internal/pay/... · ",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in\n%s", want, stdout)
		}
	}
}
