package svc

import "testing"

func TestLookup(t *testing.T) {
	s := &Service{name: "t", m: map[string]int{}}
	if got := s.Lookup("1"); got == "" {
		t.Fatal("empty")
	}
}
