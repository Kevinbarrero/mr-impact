package svc

type Service struct {
	name string
}

func NewService(name string) *Service {
	return &Service{name: name}
}

func (s *Service) Lookup(id string) string {
	return s.name + ":" + id
}
