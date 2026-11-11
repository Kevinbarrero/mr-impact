package callgraph

import (
	"go/types"
	"regexp"
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

// uint8Word and int32Word find the basic types the list spells byte and
// rune: go/types prints whichever the source wrote.
var (
	uint8Word = regexp.MustCompile(`\buint8\b`)
	int32Word = regexp.MustCompile(`\bint32\b`)
)

// listSpelling spells s's identical types as the list does: any for
// interface{}, byte for uint8, rune for int32.
func listSpelling(s string) string {
	s = strings.ReplaceAll(s, "interface{}", "any")
	s = uint8Word.ReplaceAllString(s, "byte")
	return int32Word.ReplaceAllString(s, "rune")
}

// libraryCalled reports whether fn's name and signature are on
// libCalledList. The receiver and the parameter and result names are not
// part of the compared signature.
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
			plain := types.NewSignatureType(nil, nil, nil, unnamed(sig.Params()), unnamed(sig.Results()), sig.Variadic())
			text = listSpelling(types.TypeString(plain, nil))
		}
		if text == e.sig {
			return true
		}
	}
	return false
}

// unnamed returns t's types as a tuple of unnamed variables, so TypeString
// prints "func(error) bool" for "func(target error) bool".
func unnamed(t *types.Tuple) *types.Tuple {
	vars := make([]*types.Var, t.Len())
	for i := range vars {
		vars[i] = types.NewParam(0, nil, "", t.At(i).Type())
	}
	return types.NewTuple(vars...)
}
