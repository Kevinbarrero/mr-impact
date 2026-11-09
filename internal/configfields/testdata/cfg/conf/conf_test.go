package conf

import "testing"

func TestDebug(t *testing.T) {
	if (Config{}).Debug {
		t.Fatal()
	}
}
