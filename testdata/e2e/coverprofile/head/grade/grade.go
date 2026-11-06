package grade

// Letter maps a score to a letter.
func Letter(score int) string {
	if score >= 90 {
		return "A"
	}
	if score >= 50 {
		return "C"
	}
	return "F"
}

// Report formats a score.
func Report(score int) string {
	return "grade: " + Letter(score)
}

// Bonus adds extra credit.
func Bonus(score int) int {
	if score > 95 {
		return 100
	}
	return score + 5
}

// Card renders a report card.
func Card(score int) string {
	return "[" + Report(score) + "]"
}

// Unused is new and never called nor tested.
func Unused() int {
	return 7
}
