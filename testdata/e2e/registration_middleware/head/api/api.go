package api

import (
	"example.com/rm/router"
	"example.com/rm/svc"
)

var s *svc.Service

func mw(c *router.Context) {}

func GetTrade(c *router.Context) {
	_ = s.Lookup(c.Params["id"])
}

func CreateTrade(c *router.Context) {}

func Register(e *router.Engine, svcs *svc.Service) {
	s = svcs
	api := e.Group("/trade")
	api.GET("/:id", mw, GetTrade)
	api.POST("/", CreateTrade)
}
