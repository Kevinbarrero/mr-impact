package http

import (
	"example.com/hx/internal/adapters/pg"
	"example.com/hx/internal/domain/order"
)

// Handle serves and stores one order.
func Handle(id int) int { return pg.Save(order.New(id)) }
