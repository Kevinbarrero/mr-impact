package main

import (
	"fmt"

	"example.com/wr/lib"
)

func main() { run() }

func run() {
	s := lib.NewService("a")
	fmt.Println(lib.Name(s), lib.Build(nil))
}
