package pg

import "example.com/hx/internal/domain/order"

// Save stores an order.
func Save(o order.Order) int { return o.ID }
