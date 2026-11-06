package store

// Store is a key-value lookup.
type Store interface {
	Get(key string) string
}

// New returns the default Store.
func New() Store {
	return &memStore{m: map[string]string{"k": "v"}}
}
