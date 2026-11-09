package dead

import "testing"

func TestTestOnly(t *testing.T) {
	if testOnly() != 4 {
		t.Fatal()
	}
}
