//go:build special

package main

// Special is only compiled with -tags special.
func Special() int {
	return 1
}
