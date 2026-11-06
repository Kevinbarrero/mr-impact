package orders

import "example.com/fx/pricing"

// Create prices an order of n items.
func Create(n int) int {
	return pricing.Total(n, 10)
}
