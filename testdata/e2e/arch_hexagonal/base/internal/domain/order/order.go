package order

import "example.com/hx/internal/ports/clock"

// Order is one order.
type Order struct{ ID, At int }

// New stamps an order (domain -> ports: a violation already in base).
func New(id int) Order { return Order{ID: id, At: clock.Now()} }
