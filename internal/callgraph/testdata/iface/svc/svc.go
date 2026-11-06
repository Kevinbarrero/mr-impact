package svc

import "example.com/iface/store"

// Lookup reads "k" from s.
func Lookup(s store.Store) string {
	return s.Get("k")
}
