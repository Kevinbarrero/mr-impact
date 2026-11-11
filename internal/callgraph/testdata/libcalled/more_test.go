package lc

import (
	"fmt"
	"testing"
)

func TestPrintln(t *testing.T) {
	v := S{}
	fmt.Println(v)
}

func TestPointer(t *testing.T) {
	_ = fmt.Sprint(&Q{})
}

func TestClosure(t *testing.T) {
	f := func() error { return C{} }
	_ = f()
}
