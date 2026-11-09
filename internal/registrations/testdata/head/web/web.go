package web

import "strings"

type HandlerFunc func(string) string

type Router struct{ routes map[string]HandlerFunc }

func (r *Router) GET(path string, h HandlerFunc) { r.routes[path] = h }

type ctl struct{ name func(string) string }

func (c *ctl) list(s string) string { return s }
func show(s string) string          { return s }
func health(s string) string        { return s }

// Setup registers the routes; the receiver variable was renamed.
func Setup(r *Router) {
	ctrl := &ctl{}
	r.GET("/list", ctrl.list)
	r.GET("/show", health)
	r.GET("/healthz", health)
	r.GET("/upper", strings.ToUpper)
	r.GET("/field", ctrl.name)
}
