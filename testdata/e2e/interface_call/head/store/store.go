package store

// Store reads values by key.
type Store interface {
	Get(k string) string
}
