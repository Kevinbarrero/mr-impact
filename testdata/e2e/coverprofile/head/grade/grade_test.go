package grade

import "testing"

func TestLetter(t *testing.T) {
	if Letter(95) != "A" {
		t.Fatal("want A")
	}
	if Letter(60) != "C" {
		t.Fatal("want C")
	}
}

func TestBonus(t *testing.T) {
	if Bonus(99) != 100 {
		t.Fatal("want 100")
	}
}
