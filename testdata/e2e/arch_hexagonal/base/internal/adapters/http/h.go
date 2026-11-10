package http

import "example.com/hx/internal/domain/order"

// Handle serves one order.
func Handle(id int) int { return order.New(id).ID }
