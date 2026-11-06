package api

import "example.com/fx/store"

// Handle reads "x" through the interface.
func Handle(s store.Store) string {
	return s.Get("x")
}
