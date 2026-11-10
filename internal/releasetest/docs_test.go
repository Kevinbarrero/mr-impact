package releasetest_test

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readmeHeadings are the ##/### headings of the README before the first-release
// rewrite. Each must survive as a heading in README.md or docs/reference.md.
var readmeHeadings = []string{
	"Local usage",
	"Configuration",
	"Architecture",
	"CI integration",
	"Token setup",
	"Shadow mode",
	"Exit codes",
	"What the note shows",
	"Limits",
	"License",
}

var headingRE = regexp.MustCompile(`(?m)^#{2,6} +(.+?) *$`)

func headings(t *testing.T, rel string) map[string]bool {
	t.Helper()
	hs := map[string]bool{}
	for _, m := range headingRE.FindAllStringSubmatch(string(repoFile(t, rel)), -1) {
		hs[m[1]] = true
	}
	return hs
}

func TestDocs_ReferenceKeepsReadmeSections(t *testing.T) {
	readme := headings(t, "README.md")
	ref := headings(t, "docs/reference.md")
	for _, h := range readmeHeadings {
		if !readme[h] && !ref[h] {
			t.Errorf("heading %q is in neither README.md nor docs/reference.md", h)
		}
	}
}

func TestDocs_ReferenceListsEveryFlag(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}
	out, _ := exec.Command("go", "run", "../../cmd/mrimpact", "--help").CombinedOutput()
	flags := regexp.MustCompile(`(?m)^  -([a-z-]+)`).FindAllStringSubmatch(string(out), -1)
	if len(flags) < 10 {
		t.Fatalf("parsed only %d flags from --help:\n%s", len(flags), out)
	}
	ref := string(repoFile(t, "docs/reference.md"))
	for _, f := range flags {
		if !strings.Contains(ref, "`--"+f[1]) {
			t.Errorf("flag --%s is not documented in docs/reference.md", f[1])
		}
	}
}

// linkRE matches the target of an inline Markdown link or image.
var linkRE = regexp.MustCompile(`\]\(([^)\s]+)\)`)

func TestDocs_RelativeLinksResolve(t *testing.T) {
	for _, doc := range []string{"README.md", "docs/reference.md", "CONTRIBUTING.md", "SECURITY.md", "CHANGELOG.md"} {
		for _, m := range linkRE.FindAllStringSubmatch(string(repoFile(t, doc)), -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			target, _, _ = strings.Cut(target, "#")
			rel := path.Join(path.Dir(doc), target)
			if _, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(rel))); err != nil {
				t.Errorf("%s: link %q does not resolve to a repository file", doc, m[1])
			}
		}
	}
}

func TestIssueTemplates_ParseAsForms(t *testing.T) {
	for _, f := range []string{"bug.yml", "false-positive.yml", "feature.yml"} {
		m := loadYAML(t, ".github/ISSUE_TEMPLATE/"+f)
		for _, k := range []string{"name", "description", "body"} {
			if _, ok := m[k]; !ok {
				t.Errorf("%s: missing %q", f, k)
			}
		}
	}
}
