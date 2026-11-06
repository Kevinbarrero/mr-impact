package grade

import "testing"

func TestLetter(t *testing.T) {
	if Letter(95) != "A" {
		t.Fatal("want A")
	}
}
