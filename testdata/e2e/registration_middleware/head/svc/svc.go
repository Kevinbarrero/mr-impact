package svc

type Service struct {
	name string
	m    map[string]int
}

func NewService(name string) *Service {
	return &Service{name: name, m: make(map[string]int)}
}

func (s *Service) Lookup(id string) string {
	if id == "" {
		s.m["empty"]++
		return ""
	}
	s.m[id]++
	return s.name + ":" + id
}
