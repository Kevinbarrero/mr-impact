// Package router is a minimal gin-like router, so the fixture needs no
// dependency.
package router

type Context struct{ Params map[string]string }

type HandlerFunc func(c *Context)

type Engine struct{}

type Group struct{}

func (e *Engine) Group(p string) *Group { return &Group{} }

func (g *Group) GET(p string, hs ...HandlerFunc) {}

func (g *Group) POST(p string, hs ...HandlerFunc) {}
