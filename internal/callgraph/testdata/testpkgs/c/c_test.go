package c

import "testing"

// TestC reaches a.A at depth 3: C -> b.Relay -> a.A.
func TestC(t *testing.T) { C() }
