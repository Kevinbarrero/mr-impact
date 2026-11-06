package orders

import "testing"

func TestCreate(t *testing.T) {
	if Create(2) != 20 {
		t.Fatal("bad total")
	}
}
