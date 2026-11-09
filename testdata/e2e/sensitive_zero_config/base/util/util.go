package util

// Label formats n.
func Label(n int) string {
	if n > 0 {
		return "positive"
	}
	return "other"
}
