package b

import (
	"testing"

	"example.com/cp/a"
)

func TestF(t *testing.T) {
	if a.F(20) != 10 {
		t.Fatal("want 10")
	}
}
