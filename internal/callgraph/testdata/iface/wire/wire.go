package wire

import "example.com/iface/bus"

type A struct{ n int }

func (a *A) handle(x int) { a.n += x }

// NewA registers a method value.
func NewA(b *bus.Bus) *A {
	a := &A{}
	b.Subscribe("topic.a", a.handle)
	return a
}

func onB(x int) {}

// NewB registers a plain function on another topic.
func NewB(b *bus.Bus) {
	b.Subscribe("topic.b", onB)
}

var resolver func(int) string

// NewSvc installs a closure that resolve calls later.
func NewSvc() {
	resolver = func(id int) string { return "" }
}

func resolve(id int) string { return resolver(id) }

// Log reaches the closure created by NewSvc, not NewSvc.
func Log() string { return resolve(1) }

// Main wires everything.
func Main() {
	b := bus.New()
	NewA(b)
	NewB(b)
	NewSvc()
	b.Publish("topic.a", 1)
	_ = Log()
	_ = Save()
}

// Save runs its work in a transaction closure.
func Save() error {
	return bus.RunTx(func() error { return nil })
}
