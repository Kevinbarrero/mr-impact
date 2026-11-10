package order

import (
	"testing"

	// A core -> application import in a test file: test variants are not
	// part of the graph, so it is never judged.
	"example.com/ai/internal/service/clock"
)

func TestTotal(t *testing.T) {
	if Total(clock.Now()) == 0 {
		t.Fatal("zero")
	}
}
