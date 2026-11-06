package calc

func apply(xs []int, f func(int) int) []int {
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

func double(x int) int {
	return x + x
}

// Run doubles every element.
func Run(xs []int) []int {
	return apply(xs, double)
}
