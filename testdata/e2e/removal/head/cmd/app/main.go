package main

import (
	"example.com/fx/internal/bus"
	"example.com/fx/internal/conf"
	"example.com/fx/internal/db"
	"example.com/fx/internal/svc"
)

func main() {
	var c conf.Config
	_ = c.Port
	var q db.Querier = &db.Queries{}
	_ = q.ListTopics()
	b := bus.New()
	svc.New(b)
	svc.NewOther(b)
	b.Publish("select", svc.Count(1, 2))
}
