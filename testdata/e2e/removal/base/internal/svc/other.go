package svc

import "example.com/fx/internal/bus"

func other(int) {}

func onHedge(int) {}

// NewOther subscribes the other handlers.
func NewOther(b *bus.Bus) {
	b.Subscribe("other", other)
	b.Subscribe("hedge", onHedge)
}
