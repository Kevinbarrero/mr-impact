package main

import (
	"example.com/fx/api"
	"example.com/fx/store"
)

func main() {
	println(api.Handle(&store.Mem{}))
}
