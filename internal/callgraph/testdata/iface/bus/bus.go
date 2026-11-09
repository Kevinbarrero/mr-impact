package bus

// Bus dispatches ints to handlers by topic, like an event bus.
type Bus struct {
	m map[string]func(int)
}

func New() *Bus { return &Bus{m: map[string]func(int){}} }

// Subscribe wraps h in a closure, so the dynamic call to h is made from
// Subscribe's closure.
func (b *Bus) Subscribe(topic string, h func(int)) {
	b.m[topic] = func(x int) { h(x) }
}

// Publish calls the wrapped handler of topic.
func (b *Bus) Publish(topic string, x int) {
	if f := b.m[topic]; f != nil {
		f(x)
	}
}

// RunTx runs f, like a store's InTx.
func RunTx(f func() error) error { return f() }
