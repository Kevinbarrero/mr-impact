package svc

import (
	"testing"

	"example.com/iface/store"
)

func TestLookup(t *testing.T) {
	if got := Lookup(store.New()); got != "v" {
		t.Fatalf("Lookup = %q", got)
	}
}
