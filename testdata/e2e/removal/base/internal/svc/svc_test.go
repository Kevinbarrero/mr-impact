package svc

import (
	"testing"

	"example.com/fx/internal/bus"
)

func TestSelect(t *testing.T) {
	b := bus.New()
	New(b)
	b.Publish("select", 2)
	if Count(3) != 3 || len(topics()) != 1 {
		t.Fatal()
	}
}
