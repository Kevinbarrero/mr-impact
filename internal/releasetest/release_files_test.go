package releasetest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func repoFile(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func loadYAML(t *testing.T, rel string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal(repoFile(t, rel), &m); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	return m
}

func sortedKeys(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func strs(v any) []string {
	var out []string
	for _, e := range v.([]any) {
		out = append(out, e.(string))
	}
	sort.Strings(out)
	return out
}

func TestReleaseFiles_ParseAsYAML(t *testing.T) {
	for _, f := range []string{
		".goreleaser.yaml", "action.yml", "ci/mrimpact.gitlab-ci.yml",
		".github/workflows/ci.yml", ".github/workflows/release.yml", ".github/dependabot.yml",
	} {
		if m := loadYAML(t, f); len(m) == 0 {
			t.Errorf("%s is empty", f)
		}
	}
}

func TestAction_InputsMatchSpec(t *testing.T) {
	a := loadYAML(t, "action.yml")
	want := []string{"args", "config", "coverprofile", "depth", "fail-on", "run-tests", "tags", "token", "version"}
	if got := sortedKeys(a["inputs"].(map[string]any)); !reflect.DeepEqual(got, want) {
		t.Errorf("inputs = %v, want %v", got, want)
	}
	if _, ok := a["outputs"].(map[string]any)["report"]; !ok {
		t.Error("output report missing")
	}
	runs := a["runs"].(map[string]any)
	if runs["using"] != "composite" {
		t.Errorf("using = %v", runs["using"])
	}
	for i, s := range runs["steps"].([]any) {
		step := s.(map[string]any)
		if run, ok := step["run"].(string); ok {
			if strings.Contains(run, "${{") {
				t.Errorf("step %d: run contains an expression", i)
			}
			if step["shell"] != "bash" {
				t.Errorf("step %d: shell = %v", i, step["shell"])
			}
		}
	}
}

func TestGoreleaser_Targets(t *testing.T) {
	g := loadYAML(t, ".goreleaser.yaml")
	if g["project_name"] != "mrimpact" {
		t.Errorf("project_name = %v", g["project_name"])
	}
	b := g["builds"].([]any)[0].(map[string]any)
	if b["main"] != "./cmd/mrimpact" || b["binary"] != "mrimpact" {
		t.Errorf("build main/binary = %v/%v", b["main"], b["binary"])
	}
	if got, want := strs(b["goos"]), []string{"darwin", "linux", "windows"}; !reflect.DeepEqual(got, want) {
		t.Errorf("goos = %v", got)
	}
	if got, want := strs(b["goarch"]), []string{"amd64", "arm64"}; !reflect.DeepEqual(got, want) {
		t.Errorf("goarch = %v", got)
	}
	if got := strs(b["env"]); !reflect.DeepEqual(got, []string{"CGO_ENABLED=0"}) {
		t.Errorf("env = %v", got)
	}
	if !strings.Contains(strings.Join(strs(b["ldflags"]), " "), "-X main.version=v{{.Version}}") {
		t.Errorf("ldflags = %v", b["ldflags"])
	}
	ar := g["archives"].([]any)[0].(map[string]any)
	files := strings.Join(strs(ar["files"]), "\n")
	for _, f := range []string{"LICENSE", "README.md", "CHANGELOG.md", "third_party_licenses/**"} {
		if !strings.Contains(files, f) {
			t.Errorf("archive files lack %s: %s", f, files)
		}
	}
}

func TestGitLabTemplate_Variables(t *testing.T) {
	m := loadYAML(t, "ci/mrimpact.gitlab-ci.yml")
	job := m["mrimpact"].(map[string]any)
	vars := job["variables"].(map[string]any)
	want := map[string]string{
		"MRIMPACT_GO_VERSION": "1.26", "MRIMPACT_VERSION": "latest", "MRIMPACT_COVERPROFILE": "",
		"MRIMPACT_RUN_TESTS": "false", "MRIMPACT_ARGS": "", "GIT_DEPTH": "0",
	}
	for k, v := range want {
		if got, ok := vars[k].(string); !ok || got != v {
			t.Errorf("%s = %#v, want %q", k, vars[k], v)
		}
	}
	rules := job["rules"].([]any)
	if len(rules) != 1 || rules[0].(map[string]any)["if"] != `$CI_PIPELINE_SOURCE == "merge_request_event"` {
		t.Errorf("rules = %v", rules)
	}
}

func TestGitLabTemplate_VersionNormalisation(t *testing.T) {
	m := loadYAML(t, "ci/mrimpact.gitlab-ci.yml")
	script := m["mrimpact"].(map[string]any)["script"].([]any)[0].(string)
	start := strings.Index(script, "ref=$MRIMPACT_VERSION")
	end := strings.Index(script, "esac")
	if start < 0 || end < 0 {
		t.Fatalf("script lacks the version normalisation:\n%s", script)
	}
	snippet := script[start:end+len("esac")] + "\necho \"$ref\"\n"
	for in, want := range map[string]string{"latest": "main", "v0.3.0": "v0.3.0", "0.3.0": "v0.3.0"} {
		cmd := exec.Command("sh", "-c", snippet)
		cmd.Env = append(os.Environ(), "MRIMPACT_VERSION="+in)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("version %q -> ref %q, want %q", in, got, want)
		}
	}
}

func TestCI_ReleaseCheckAndDogfoodGuard(t *testing.T) {
	ci := loadYAML(t, ".github/workflows/ci.yml")
	jobs := ci["jobs"].(map[string]any)
	rc, ok := jobs["release-check"].(map[string]any)
	if !ok {
		t.Fatal("ci.yml has no release-check job")
	}
	found := false
	for _, s := range rc["steps"].([]any) {
		step := s.(map[string]any)
		if uses, _ := step["uses"].(string); strings.HasPrefix(uses, "goreleaser/goreleaser-action@") {
			args := step["with"].(map[string]any)["args"].(string)
			if args != "release --snapshot --clean --skip=publish" {
				t.Errorf("args = %q", args)
			}
			found = true
		}
	}
	if !found {
		t.Error("release-check does not run goreleaser-action")
	}
	cond := jobs["mrimpact"].(map[string]any)["if"].(string)
	if !strings.Contains(cond, "dependabot[bot]") {
		t.Errorf("dogfood job not guarded against dependabot: %q", cond)
	}
}

func TestRelease_PrereleaseHandling(t *testing.T) {
	rel := loadYAML(t, ".github/workflows/release.yml")
	steps := rel["jobs"].(map[string]any)["release"].(map[string]any)["steps"].([]any)
	ok := false
	for _, s := range steps {
		step := s.(map[string]any)
		if step["name"] == "Move floating v0 tag" {
			ok = strings.Contains(step["if"].(string), "contains(github.ref_name, '-')")
		}
	}
	if !ok {
		t.Error("v0 move step is not skipped for pre-release tags")
	}
	g := loadYAML(t, ".goreleaser.yaml")
	if g["release"].(map[string]any)["prerelease"] != "auto" {
		t.Errorf("release = %v", g["release"])
	}
}
