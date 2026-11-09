package pay

// Settle rounds an amount down to tens: no sensitive calls, critical package.
func Settle(amount int) int {
	return amount / 10 * 10
}
