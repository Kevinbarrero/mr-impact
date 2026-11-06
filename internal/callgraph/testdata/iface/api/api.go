package api

import (
	"example.com/iface/store"
	"example.com/iface/svc"
)

// Handle serves one lookup.
func Handle() string {
	return svc.Lookup(store.New())
}
