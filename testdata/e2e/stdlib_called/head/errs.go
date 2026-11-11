package sc

type candidateErrors []error

func (c candidateErrors) Error() string { return "candidates failed" }

// Unwrap lets errors.Is look inside the collected errors.
func (c candidateErrors) Unwrap() error {
	if len(c) == 0 {
		return nil
	}
	return c[0]
}
