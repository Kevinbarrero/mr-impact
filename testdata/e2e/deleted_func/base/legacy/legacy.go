package legacy

// New returns a fresh total.
func New() int {
	return 0
}

// OldTotal is the pre-v2 total.
func OldTotal(n int) int {
	return n * 9
}
