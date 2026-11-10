package orders

import "example.com/md/internal/orders/store"

// Place places an order (same module: free).
func Place(id int) int { return store.Get(id) }
