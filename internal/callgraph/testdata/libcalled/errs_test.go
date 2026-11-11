package lc

import (
	"errors"
	"fmt"
	"testing"
)

func TestIs(t *testing.T) {
	if !errors.Is(candidateErrors{}, errBase) {
		t.Fatal("not base")
	}
}

func TestGeneric(t *testing.T) {
	var err error = E[int]{}
	_ = errors.Unwrap(err)
}

func TestValue(t *testing.T) {
	_ = fmt.Sprint(P{})
}

func TestBad(t *testing.T) {
	_ = fmt.Sprint(bad{})
}
