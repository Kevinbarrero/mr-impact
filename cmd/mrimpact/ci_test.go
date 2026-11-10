package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/report"
)

const prEvent = `{"pull_request":{"number":7,"base":{"sha":"basesha","repo":{"full_name":"owner/repo"}},"head":{"repo":{"full_name":"owner/repo"}}}}`
const forkEvent = `{"pull_request":{"number":7,"base":{"sha":"basesha","repo":{"full_name":"owner/repo"}},"head":{"repo":{"full_name":"fork/repo"}}}}`

func readerOf(files map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}
}

func TestDetectCI(t *testing.T) {
	gl := map[string]string{"CI_MERGE_REQUEST_IID": "7", "CI_PROJECT_ID": "42", "CI_API_V4_URL": "http://x"}
	gh := func(extra map[string]string) env {
		e := env{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_PATH": "/ev.json", "GITHUB_REPOSITORY": "owner/repo"}
		for k, v := range extra {
			e[k] = v
		}
		return e
	}
	both := gh(gl)
	cases := []struct {
		name    string
		env     env
		files   map[string]string
		kind    string
		base    string
		fork    bool
		summary string
		errPath string
	}{
		{name: "gitlab", env: env(gl), kind: "gitlab"},
		{name: "github pr", env: gh(map[string]string{"GITHUB_STEP_SUMMARY": "/s.md"}), files: map[string]string{"/ev.json": prEvent}, kind: "github", base: "basesha", summary: "/s.md"},
		{name: "github fork", env: gh(nil), files: map[string]string{"/ev.json": forkEvent}, kind: "github", base: "basesha", fork: true},
		{name: "push event", env: gh(nil), files: map[string]string{"/ev.json": `{"ref":"refs/heads/main"}`}, kind: "local"},
		{name: "missing event", env: gh(nil), kind: "", errPath: "/ev.json"},
		{name: "malformed event", env: gh(nil), files: map[string]string{"/ev.json": "{nope"}, errPath: "/ev.json"},
		{name: "nothing", env: env{}, kind: "local"},
		{name: "both", env: both, files: map[string]string{"/ev.json": prEvent}, kind: "gitlab"},
	}
	for _, c := range cases {
		got, err := detectCI(c.env.get, readerOf(c.files))
		if c.errPath != "" {
			if err == nil || !strings.Contains(err.Error(), c.errPath) {
				t.Errorf("%s: err=%v, want it to name %s", c.name, err, c.errPath)
			}
			continue
		}
		if err != nil || got.Kind != c.kind || got.BaseSHA != c.base || got.Fork != c.fork || got.Summary != c.summary {
			t.Errorf("%s: got %+v err=%v", c.name, got, err)
		}
	}
}

func init() {
	// No test may reach the real GitHub API: building a real client fails loudly.
	newGitHubClient = func(string, string, string) githubUpserter {
		panic("test built a real GitHub client; call stubGitHub first")
	}
}

type fakeGH struct {
	calls                          int
	repo, marker, body, token, who string
	number                         int
	err                            error
}

func stubGitHub(t *testing.T, f *fakeGH) {
	t.Helper()
	old := newGitHubClient
	newGitHubClient = func(baseURL, token, author string) githubUpserter {
		f.token, f.who = token, author
		return f
	}
	t.Cleanup(func() { newGitHubClient = old })
}

func (f *fakeGH) Upsert(_ context.Context, repo string, number int, marker, body string) error {
	f.calls++
	f.repo, f.number, f.marker, f.body = repo, number, marker, body
	return f.err
}

func ghRun(t *testing.T, event string, extra map[string]string) (e env, summary, out string) {
	t.Helper()
	d := t.TempDir()
	ev := filepath.Join(d, "event.json")
	if err := os.WriteFile(ev, []byte(event), 0o644); err != nil {
		t.Fatal(err)
	}
	summary = filepath.Join(d, "summary.md")
	e = env{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_PATH": ev, "GITHUB_REPOSITORY": "owner/repo",
		"GITHUB_STEP_SUMMARY": summary, "MRIMPACT_TOKEN": "tok", "MRIMPACT_COMMENT_AUTHOR": "bot"}
	for k, v := range extra {
		e[k] = v
	}
	return e, summary, filepath.Join(d, "r.md")
}

func TestRun_GitHubPostsComment(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	f := &fakeGH{}
	stubGitHub(t, f)
	e, summary, out := ghRun(t, prEvent, nil)
	code, stdout, stderr := exec(t, e, "--dir", repo, "--base", base, "--out", out)
	if code != 0 || f.calls != 1 {
		t.Fatalf("code=%d calls=%d stderr=%s", code, f.calls, stderr)
	}
	if f.repo != "owner/repo" || f.number != 7 || f.marker != report.Marker || f.body != stdout || f.token != "tok" || f.who != "bot" {
		t.Fatalf("upsert got %+v", f)
	}
	if b, _ := os.ReadFile(summary); string(b) != stdout {
		t.Fatalf("summary = %q", b)
	}
}

func TestRun_GitHubForkWritesSummaryOnly(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	f := &fakeGH{}
	stubGitHub(t, f)
	e, summary, out := ghRun(t, forkEvent, nil)
	code, stdout, stderr := exec(t, e, "--strict", "--dir", repo, "--base", base, "--out", out)
	if code != 0 || f.calls != 0 {
		t.Fatalf("code=%d calls=%d stderr=%s", code, f.calls, stderr)
	}
	if !strings.Contains(stderr, "fork pull request: the token is read-only, note written to the job summary") {
		t.Fatalf("stderr=%s", stderr)
	}
	if b, _ := os.ReadFile(summary); string(b) != stdout {
		t.Fatalf("summary = %q", b)
	}
}

func TestRun_GitHubNoToken(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	f := &fakeGH{}
	stubGitHub(t, f)
	e, summary, out := ghRun(t, prEvent, nil)
	delete(e, "MRIMPACT_TOKEN")
	code, stdout, stderr := exec(t, e, "--strict", "--dir", repo, "--base", base, "--out", out)
	if code != 0 || f.calls != 0 {
		t.Fatalf("code=%d calls=%d stderr=%s", code, f.calls, stderr)
	}
	if !strings.Contains(stderr, "set MRIMPACT_TOKEN (the action passes github.token; needs permissions: pull-requests: write); comment not posted") {
		t.Fatalf("stderr=%s", stderr)
	}
	if b, _ := os.ReadFile(summary); string(b) != stdout {
		t.Fatalf("summary = %q", b)
	}
}

func TestRun_GitHubPostFailureIsInternal(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	f := &fakeGH{err: errors.New("boom")}
	stubGitHub(t, f)
	e, _, out := ghRun(t, prEvent, nil)
	if code, _, stderr := exec(t, e, "--strict", "--dir", repo, "--base", base, "--out", out); code != 2 {
		t.Fatalf("strict code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := exec(t, e, "--dir", repo, "--base", base, "--out", out); code != 0 {
		t.Fatalf("lenient code=%d stderr=%s", code, stderr)
	}
}

func TestRun_GitHubNoPostStillSummarises(t *testing.T) {
	repo, base := fixture(t, "exported_signature")
	f := &fakeGH{}
	stubGitHub(t, f)
	e, summary, out := ghRun(t, prEvent, nil)
	code, stdout, _ := exec(t, e, "--no-post", "--dir", repo, "--base", base, "--out", out)
	b, _ := os.ReadFile(summary)
	if code != 0 || f.calls != 0 || string(b) != stdout {
		t.Fatalf("code=%d calls=%d summary=%q", code, f.calls, b)
	}
}

func TestRun_GitHubSkipEmptyStillSummarises(t *testing.T) {
	repo, base := fixture(t, "no_go_changes")
	f := &fakeGH{}
	stubGitHub(t, f)
	e, summary, out := ghRun(t, prEvent, nil)
	code, _, stderr := exec(t, e, "--skip-empty", "--dir", repo, "--base", base, "--out", out)
	b, _ := os.ReadFile(summary)
	if code != 0 || f.calls != 0 || len(b) == 0 {
		t.Fatalf("code=%d calls=%d summary=%q stderr=%s", code, f.calls, b, stderr)
	}
}

func TestRun_BasePrecedence(t *testing.T) {
	repo, base := fixture(t, "no_go_changes")
	// --strict makes an unresolvable base exit 2, so exit 0 proves the right
	// source was used; the other sources hold a bogus sha.
	bogus := func(sha string) string { return strings.Replace(prEvent, "basesha", sha, 1) }
	cases := []struct {
		name  string
		event string
		extra map[string]string
		args  []string
	}{
		{"flag beats var and event", bogus("bogus1"), map[string]string{"CI_MERGE_REQUEST_DIFF_BASE_SHA": "bogus2"}, []string{"--base", base}},
		{"var beats event", bogus("bogus1"), map[string]string{"CI_MERGE_REQUEST_DIFF_BASE_SHA": base}, nil},
		{"event base", bogus(base), nil, nil},
	}
	for _, c := range cases {
		stubGitHub(t, &fakeGH{})
		e, _, out := ghRun(t, c.event, c.extra)
		args := append([]string{"--strict", "--dir", repo, "--out", out}, c.args...)
		if code, _, stderr := exec(t, e, args...); code != 0 {
			t.Errorf("%s: code=%d stderr=%s", c.name, code, stderr)
		}
	}
	// Control: a bogus base alone does fail under --strict.
	stubGitHub(t, &fakeGH{})
	e, _, out := ghRun(t, bogus("bogus1"), nil)
	if code, _, _ := exec(t, e, "--strict", "--dir", repo, "--out", out); code != 2 {
		t.Errorf("control: bogus event base should exit 2, got %d", code)
	}
	// A push event with no --base keeps the usage error.
	e, _, out = ghRun(t, `{"ref":"x"}`, nil)
	if code, _, stderr := exec(t, e, "--dir", repo, "--out", out); code != 2 || !strings.Contains(stderr, "--base is required") {
		t.Fatalf("push: code=%d stderr=%s", code, stderr)
	}
}

func TestRun_BadRepositoryIsUsageError(t *testing.T) {
	for _, bad := range []string{"", "noslash", "a/b/c"} {
		e, _, out := ghRun(t, prEvent, map[string]string{"GITHUB_REPOSITORY": bad})
		code, _, stderr := exec(t, e, "--base", "x", "--out", out)
		if code != 2 || !strings.Contains(stderr, "GITHUB_REPOSITORY") || !strings.Contains(stderr, fmt.Sprintf("%q", bad)) {
			t.Errorf("%q: code=%d stderr=%s", bad, code, stderr)
		}
	}
}

func TestRun_MalformedEventIsUsageError(t *testing.T) {
	e, _, out := ghRun(t, "{nope", nil)
	code, _, stderr := exec(t, e, "--base", "x", "--out", out)
	if code != 2 || !strings.Contains(stderr, e["GITHUB_EVENT_PATH"]) {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}
