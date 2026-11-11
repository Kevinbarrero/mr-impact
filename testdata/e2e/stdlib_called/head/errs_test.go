package sc

import (
	"errors"
	"testing"
)

func TestIs(t *testing.T) {
	var err error = candidateErrors{errors.New("a")}
	if !errors.Is(err, errors.ErrUnsupported) && err == nil {
		t.Fatal("unexpected")
	}
}
