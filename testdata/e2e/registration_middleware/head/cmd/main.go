package main

import (
	"example.com/rm/api"
	"example.com/rm/router"
	"example.com/rm/svc"
)

func main() {
	api.Register(&router.Engine{}, svc.NewService("trade"))
}
