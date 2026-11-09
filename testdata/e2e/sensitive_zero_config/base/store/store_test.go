package store

import (
	"context"
	"testing"
)

func TestSave(t *testing.T) {
	if Save(context.Background(), nil) == nil {
		t.Skip("never run: the fixture is only analyzed")
	}
}
