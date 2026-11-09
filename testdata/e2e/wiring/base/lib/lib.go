package lib

import "fmt"

type Service struct{ name string }

func NewService(name string) *Service {
	return &Service{name: name}
}

func Build(items []string) int {
	n := 0
	for _, it := range items {
		n += len(it)
	}
	return n
}

func Name(s *Service) string { return fmt.Sprint(s.name) }
