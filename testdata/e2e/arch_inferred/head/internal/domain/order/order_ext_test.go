package order_test

import (
	"testing"

	"example.com/ai/internal/domain/order"
	// An external test package is a package of its own under a core path:
	// as a test variant it is skipped, so this import is never judged.
	"example.com/ai/internal/service/clock"
)

func TestTotalExt(t *testing.T) {
	if order.Total(clock.Now()) == 0 {
		t.Fatal("zero")
	}
}
