package bus

// Bus dispatches to handlers by topic.
type Bus struct {
	m map[string]func(int)
}

func New() *Bus { return &Bus{m: map[string]func(int){}} }

// Subscribe registers h for topic.
func (b *Bus) Subscribe(topic string, h func(int)) {
	b.m[topic] = func(x int) { h(x) }
}

// Publish runs topic's handler.
func (b *Bus) Publish(topic string, x int) {
	if f := b.m[topic]; f != nil {
		f(x)
	}
}
