package store

// Mem is an in-memory Store.
type Mem struct{}

func (m *Mem) Get(k string) string {
	return "mem:" + k
}
