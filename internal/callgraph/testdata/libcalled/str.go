package lc

type bad struct{}

// Unwrap has the wrong signature: errors never calls it.
func (bad) Unwrap() string { return "" }
