package x

import "example.com/md/internal/orders"

// Bill bills an order through the orders module root.
func Bill(id int) int { return orders.Place(id) }
