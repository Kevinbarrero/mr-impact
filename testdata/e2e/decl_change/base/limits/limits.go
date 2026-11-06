package limits

// Max is the largest allowed value.
const Max = 10

// Range is a closed interval.
type Range struct {
	Lo, Hi int
}

// Clamp limits x to Max.
func Clamp(x int) int {
	if x > Max {
		return Max
	}
	return x
}
