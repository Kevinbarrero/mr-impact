package describe

import "reflect"

// Name names v's type.
func Name(v any) string {
	return reflect.TypeOf(v).Name()
}
