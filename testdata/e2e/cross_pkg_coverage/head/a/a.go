package a

func F(n int) int {
	if n > 10 {
		n = 10
	}
	if n < 0 {
		n = 0
	}
	return n
}
