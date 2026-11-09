package svc

import "example.com/fx/internal/bus"

// Svc selects a mode for each event.
type Svc struct{ b *bus.Bus }

// New subscribes Select.
func New(b *bus.Bus) *Svc {
	s := &Svc{b: b}
	b.Subscribe("select", s.Select)
	return s
}

// Mode is how an event is handled.
type Mode int

const (
	Normal Mode = iota
	Fast
	Hedge
)

// Select routes one event.
func (s *Svc) Select(mode int) {
	switch Mode(mode) {
	case Fast:
		return
	}
	normal()
}

func normal() {}

// Count adds a and b.
func Count(a, b int) int {
	return a + b
}

func topics() []string { return []string{"select"} }

// Topics lists every topic.
func Topics() []string { return topicList() }

func topicList() []string { return []string{"select"} }
