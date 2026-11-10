package coverage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const mod = "github.com/Kevinbarrero/mr-impact"

// writeProfile writes a set-mode profile with the given block lines.
func writeProfile(t *testing.T, path string, blocks ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "mode: set\n" + strings.Join(blocks, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func info(lines int) FileInfo {
	return func(p string) (int, bool) {
		if p == "a/a.go" {
			return lines, true
		}
		return 0, false
	}
}

func blk(file string, start, end, count int) string {
	return mod + "/" + file + ":" + itoa(start) + ".1," + itoa(end) + ".2 1 " + itoa(count)
}

func itoa(n int) string { return strconv.Itoa(n) }

func mustLoad(t *testing.T, patterns []string, fi FileInfo) (*Profile, string) {
	t.Helper()
	p, w, err := Load(patterns, mod, fi)
	if err != nil {
		t.Fatal(err)
	}
	return p, w
}

func TestLinesCoveredPartial(t *testing.T) {
	f := filepath.Join(t.TempDir(), "c.out")
	writeProfile(t, f, blk("a/a.go", 10, 12, 1), blk("a/a.go", 13, 15, 0))
	p, w := mustLoad(t, []string{f}, info(100))
	if p == nil {
		t.Fatalf("nil profile, warning %q", w)
	}
	c, tot := p.Lines("a/a.go", []int{11, 14, 20})
	if c != 1 || tot != 2 {
		t.Fatalf("got covered=%d total=%d, want 1, 2", c, tot)
	}
}

func TestMergeProfiles(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a.out"), filepath.Join(d, "b.out")
	writeProfile(t, a, blk("a/a.go", 10, 12, 0))
	writeProfile(t, b, blk("a/a.go", 10, 12, 1))
	p, _ := mustLoad(t, []string{a, b}, info(100))
	if c, tot := p.Lines("a/a.go", []int{11}); c != 1 || tot != 1 {
		t.Fatalf("got %d/%d, want 1/1", c, tot)
	}
}

func TestGlob(t *testing.T) {
	d := t.TempDir()
	writeProfile(t, filepath.Join(d, "cov", "1.out"), blk("a/a.go", 10, 12, 1))
	writeProfile(t, filepath.Join(d, "cov", "2.out"), blk("a/a.go", 20, 22, 1))
	// overlapping patterns must not double-parse
	p, _ := mustLoad(t, []string{filepath.Join(d, "cov", "*.out"), filepath.Join(d, "cov", "1.out")}, info(100))
	if c, tot := p.Lines("a/a.go", []int{11, 21}); c != 2 || tot != 2 {
		t.Fatalf("got %d/%d, want 2/2", c, tot)
	}
}

func TestPartialNoMatchWarns(t *testing.T) {
	d := t.TempDir()
	writeProfile(t, filepath.Join(d, "1.out"), blk("a/a.go", 10, 12, 1))
	p, w := mustLoad(t, []string{filepath.Join(d, "1.out"), filepath.Join(d, "nope*.out")}, info(100))
	if p == nil || !strings.Contains(w, "nope*.out") {
		t.Fatalf("profile=%v warning=%q", p, w)
	}
}

func TestStaleMissingFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "c.out")
	writeProfile(t, f, blk("a/gone.go", 1, 2, 1))
	if p, w := mustLoad(t, []string{f}, info(100)); p != nil || w == "" {
		t.Fatalf("profile=%v warning=%q", p, w)
	}
}

func TestStaleLineBeyondEOF(t *testing.T) {
	f := filepath.Join(t.TempDir(), "c.out")
	writeProfile(t, f, blk("a/a.go", 10, 50, 1))
	if p, w := mustLoad(t, []string{f}, info(40)); p != nil || w == "" {
		t.Fatalf("profile=%v warning=%q", p, w)
	}
}

func TestForeignModule(t *testing.T) {
	f := filepath.Join(t.TempDir(), "c.out")
	writeProfile(t, f, "example.com/other/a/a.go:1.1,2.2 1 1")
	if p, w := mustLoad(t, []string{f}, info(100)); p != nil || w == "" {
		t.Fatalf("profile=%v warning=%q", p, w)
	}
}

func TestNoMatch(t *testing.T) {
	pat := filepath.Join(t.TempDir(), "zzz*.out")
	p, w := mustLoad(t, []string{pat}, info(100))
	if p != nil || !strings.Contains(w, pat) {
		t.Fatalf("profile=%v warning=%q", p, w)
	}
}

func TestMalformedIsError(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bad.out")
	if err := os.WriteFile(f, []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load([]string{f}, mod, info(100))
	if err == nil || !strings.Contains(err.Error(), "bad.out") {
		t.Fatalf("err=%v", err)
	}
}

func TestLinesNilAndUnknown(t *testing.T) {
	var p *Profile
	if c, tot := p.Lines("a/a.go", []int{1}); c != 0 || tot != 0 {
		t.Fatal("nil profile")
	}
	f := filepath.Join(t.TempDir(), "c.out")
	writeProfile(t, f, blk("a/a.go", 10, 12, 1))
	q, _ := mustLoad(t, []string{f}, info(100))
	if c, tot := q.Lines("other.go", []int{11}); c != 0 || tot != 0 {
		t.Fatal("unknown file")
	}
}
