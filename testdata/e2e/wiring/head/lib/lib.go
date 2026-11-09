package lib

import "fmt"

type Service struct{ name string }

func NewService(name string) *Service {
	s := &Service{name: name}
	fmt.Println("service", name)
	return s
}

func Build(items []string) int {
	n := 0
	for _, it := range items {
		n += len(it) + 1
	}
	return n
}

func Name(s *Service) string { return fmt.Sprint(s.name) }
