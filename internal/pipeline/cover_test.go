package pipeline

import (
	"reflect"
	"testing"
)

func TestCrossPkgTests(t *testing.T) {
	got := crossPkgTests("m/a", []string{"m/a", "m/a_test", "m/b"})
	if want := []string{"m/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCoverText_ExternalTestPackageIsSamePackage(t *testing.T) {
	cross := crossPkgTests("m/a", []string{"m/a_test"})
	if len(cross) != 0 {
		t.Fatalf("cross = %v, want none", cross)
	}
	if got := coverText(true, 0, 3, true, true, cross, "m"); got != "0 % (0/3)" {
		t.Errorf("got %q", got)
	}
}

func TestCoverText_CrossPkg(t *testing.T) {
	got := coverText(true, 0, 3, true, true, []string{"m/b", "m/c", "m/d"}, "m")
	if want := "0 % in profile · reached by tests in b, c +1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCoverText_Existing(t *testing.T) {
	cases := []struct {
		prof        bool
		cov, total  int
		graph, test bool
		want        string
	}{
		{true, 3, 5, true, false, "60 % (3/5)"},
		{true, 0, 0, true, true, "no added lines · reached by test"},
		{true, 0, 0, true, false, "no added lines · not reached"},
		{true, 0, 0, false, false, "no added lines"},
		{false, 0, 0, true, false, "not reached"},
		{false, 0, 0, true, true, "reached by test"},
		{false, 0, 0, false, false, "n/a"},
	}
	for _, c := range cases {
		if got := coverText(c.prof, c.cov, c.total, c.graph, c.test, nil, "m"); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
