package limits

import "testing"

func TestCheck(t *testing.T) {
	if err := Check(3); err != nil {
		t.Fatal(err)
	}
}
