package callgraph

import (
	"go/types"
	"strings"
)

// libCalledList is the methods the standard library calls on an interface
// value (errors, fmt, encoding/json, encoding, net/http, sort,
// container/heap, database/sql): name and signature, written as
// types.TypeString with full package paths and "any" for interface{}.
var libCalledList = []struct{ name, sig string }{
	{"Error", "func() string"},
	{"String", "func() string"},
	{"GoString", "func() string"},
	{"Format", "func(fmt.State, rune)"},
	{"Unwrap", "func() error"},
	{"Unwrap", "func() []error"},
	{"Is", "func(error) bool"},
	{"As", "func(any) bool"},
	{"MarshalJSON", "func() ([]byte, error)"},
	{"UnmarshalJSON", "func([]byte) error"},
	{"MarshalText", "func() ([]byte, error)"},
	{"UnmarshalText", "func([]byte) error"},
	{"MarshalBinary", "func() ([]byte, error)"},
	{"UnmarshalBinary", "func([]byte) error"},
	{"ServeHTTP", "func(net/http.ResponseWriter, *net/http.Request)"},
	{"Len", "func() int"},
	{"Less", "func(int, int) bool"},
	{"Swap", "func(int, int)"},
	{"Push", "func(any)"},
	{"Pop", "func() any"},
	{"Scan", "func(any) error"},
	{"Value", "func() (database/sql/driver.Value, error)"},
}

// libraryCalled reports whether fn's name and signature are on
// libCalledList. The receiver is not part of the compared signature.
func libraryCalled(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return false
	}
	name := fn.Name()
	var text string
	for _, e := range libCalledList {
		if e.name != name {
			continue
		}
		if text == "" {
			plain := types.NewSignatureType(nil, nil, nil, sig.Params(), sig.Results(), sig.Variadic())
			text = strings.ReplaceAll(types.TypeString(plain, nil), "interface{}", "any")
		}
		if text == e.sig {
			return true
		}
	}
	return false
}
