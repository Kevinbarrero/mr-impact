package limits

import "errors"

const (
	max   = 10
	scale = 1
)

var errTooBig = errors.New("too big")

// Check reports whether n is allowed.
func Check(n int) error {
	return exceeds(n * scale)
}

func exceeds(n int) error {
	if n > max {
		return errTooBig
	}
	return nil
}
