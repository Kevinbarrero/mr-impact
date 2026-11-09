package svc

import (
	"example.com/fx/internal/bus"
	"example.com/fx/internal/conf"
	"example.com/fx/internal/db"
)

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
	case Hedge:
		hedge()
		return
	}
	normal()
}

func hedge() {}

func normal() {}

// Count adds a.
func Count(a int) int {
	return a
}

func topics() []string { return []string{"select"} }

// Topics lists every topic.
func Topics() []string { return topicList() }

func topicList() []string { return []string{"select"} }

// Admin is the admin view.
func Admin() int {
	var c conf.Config
	var q db.Querier = &db.Queries{}
	return len(topics()) + len(Topics()) + c.AdminPort + len(q.GetTopic(1))
}
