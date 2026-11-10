package svc

import "testing"

func TestLookup(t *testing.T) {
	s := &Service{name: "t"}
	if got := s.Lookup("1"); got == "" {
		t.Fatal("empty")
	}
}
