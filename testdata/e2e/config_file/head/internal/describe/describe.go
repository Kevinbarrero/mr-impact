package describe

import "reflect"

// Name names v's type, with its package.
func Name(v any) string {
	return reflect.TypeOf(v).String()
}
