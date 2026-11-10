package checkout

import "example.com/ai/internal/domain/order"

// Checkout totals an order.
func Checkout(n int) int { return order.Total(n) }
