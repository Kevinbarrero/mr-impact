package web

import "strings"

type HandlerFunc func(string) string

type Router struct{ routes map[string]HandlerFunc }

func (r *Router) GET(path string, h HandlerFunc) { r.routes[path] = h }

type ctl struct{ name func(string) string }

func (c *ctl) list(s string) string   { return s }
func (c *ctl) delete(s string) string { return s }
func show(s string) string            { return s }

// Setup registers the routes.
func Setup(r *Router) {
	c := &ctl{}
	r.GET("/list", c.list)
	r.GET("/delete", HandlerFunc(c.delete))
	r.GET("/show", show)
	r.GET("/upper", strings.ToUpper) // not a module function
	r.GET("/field", c.name)          // a field, not a function
}
