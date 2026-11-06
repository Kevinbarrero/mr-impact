package app

import "example.com/fx/ui"

// Greet returns the greeting.
func Greet(name string) string {
	return ui.Title() + ": hi " + name + "!"
}

// Serve renders the default page.
func Serve() string {
	return Greet("world")
}
