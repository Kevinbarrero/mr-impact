package limits

// Max is the largest allowed value.
const Max = 20

// Range is a closed interval.
type Range struct {
	Lo, Hi, Step int
}

// Clamp limits x to Max.
func Clamp(x int) int {
	if x > Max {
		return Max
	}
	return x
}
