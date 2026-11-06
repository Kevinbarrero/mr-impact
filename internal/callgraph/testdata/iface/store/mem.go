package store

type memStore struct {
	m map[string]string
}

func (s *memStore) Get(key string) string {
	return s.m[key]
}
