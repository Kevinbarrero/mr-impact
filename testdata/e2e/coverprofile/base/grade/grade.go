package grade

// Letter maps a score to a letter.
func Letter(score int) string {
	if score >= 90 {
		return "A"
	}
	return "F"
}

// Report formats a score.
func Report(score int) string {
	return "grade " + Letter(score)
}
