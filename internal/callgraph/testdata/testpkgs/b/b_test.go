package b

import (
	"testing"

	"example.com/tp/a"
)

func Helper() { a.A() }

func TestB(t *testing.T) { Helper() }
