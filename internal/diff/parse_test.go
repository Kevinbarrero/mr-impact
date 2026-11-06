package diff

import (
	"reflect"
	"strings"
	"testing"
)

func parseOne(t *testing.T, in string) FileChange {
	t.Helper()
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 file, got %d: %+v", len(got), got)
	}
	return got[0]
}

func TestParseModifiedHunks(t *testing.T) {
	fc := parseOne(t, `diff --git a/pkg/a.go b/pkg/a.go
index 111..222 100644
--- a/pkg/a.go
+++ b/pkg/a.go
@@ -10,2 +10,3 @@ func F() {
-old1
-old2
+new1
+new2
+new3
@@ -40 +41,0 @@ func G() {
-gone
`)
	if fc.Status != Modified || fc.Path != "pkg/a.go" || fc.OldPath != "pkg/a.go" {
		t.Fatalf("bad file: %+v", fc)
	}
	if want := []int{10, 11, 12}; !reflect.DeepEqual(fc.AddedLines, want) {
		t.Errorf("added = %v, want %v", fc.AddedLines, want)
	}
	if want := []int{10, 11, 40}; !reflect.DeepEqual(fc.DeletedLines, want) {
		t.Errorf("deleted = %v, want %v", fc.DeletedLines, want)
	}
}

func TestParseAddedFile(t *testing.T) {
	fc := parseOne(t, `diff --git a/n.go b/n.go
new file mode 100644
index 0000000..333
--- /dev/null
+++ b/n.go
@@ -0,0 +1,3 @@
+package n
+
+func N() {}
`)
	if fc.Status != Added || fc.OldPath != "" || fc.Path != "n.go" {
		t.Fatalf("bad file: %+v", fc)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(fc.AddedLines, want) {
		t.Errorf("added = %v, want %v", fc.AddedLines, want)
	}
	if len(fc.DeletedLines) != 0 {
		t.Errorf("deleted = %v", fc.DeletedLines)
	}
}

func TestParseDeletedFile(t *testing.T) {
	fc := parseOne(t, `diff --git a/old_test.go b/old_test.go
deleted file mode 100644
index 333..0000000
--- a/old_test.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package x
-
`)
	if fc.Status != Deleted || fc.Path != "" || fc.OldPath != "old_test.go" {
		t.Fatalf("bad file: %+v", fc)
	}
	if !fc.IsTest {
		t.Error("IsTest should be computed from OldPath")
	}
	if want := []int{1, 2}; !reflect.DeepEqual(fc.DeletedLines, want) {
		t.Errorf("deleted = %v, want %v", fc.DeletedLines, want)
	}
}

func TestParsePureRename(t *testing.T) {
	fc := parseOne(t, `diff --git a/a.go b/b.go
similarity index 100%
rename from a.go
rename to b.go
`)
	if fc.Status != Renamed || fc.OldPath != "a.go" || fc.Path != "b.go" {
		t.Fatalf("bad file: %+v", fc)
	}
	if len(fc.AddedLines) != 0 || len(fc.DeletedLines) != 0 {
		t.Errorf("want no lines: %+v", fc)
	}
}

func TestParseTestFile(t *testing.T) {
	fc := parseOne(t, `diff --git a/x_test.go b/x_test.go
index 1..2 100644
--- a/x_test.go
+++ b/x_test.go
@@ -1 +1 @@
-a
+b
`)
	if !fc.IsTest {
		t.Error("IsTest = false")
	}
}

func TestParseNoNewlineMarker(t *testing.T) {
	got, err := Parse(strings.NewReader(`diff --git a/a.go b/a.go
index 1..2 100644
--- a/a.go
+++ b/a.go
@@ -1 +1 @@
-a
\ No newline at end of file
+b
\ No newline at end of file
diff --git a/c.go b/c.go
index 1..2 100644
--- a/c.go
+++ b/c.go
@@ -5,0 +6 @@
+x
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 files, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0].AddedLines, []int{1}) || !reflect.DeepEqual(got[0].DeletedLines, []int{1}) {
		t.Errorf("file0: %+v", got[0])
	}
	if !reflect.DeepEqual(got[1].AddedLines, []int{6}) || len(got[1].DeletedLines) != 0 {
		t.Errorf("file1: %+v", got[1])
	}
}
