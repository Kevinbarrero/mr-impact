package x

import (
	"example.com/md/internal/orders"
	"example.com/md/internal/orders/store"
)

// Bill bills an order, reading the other module's store directly.
func Bill(id int) int { return orders.Place(id) + store.Get(id) }
