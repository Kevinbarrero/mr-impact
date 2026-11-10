package trade

type HandlerFunc func(string) string

type Router struct{}

func (r *Router) Group(path string) *Router          { return r }
func (r *Router) GET(path string, hs ...HandlerFunc) {}

type controller struct{}

func (c *controller) mwA(s string) string { return s }
func (c *controller) mwB(s string) string { return s }
func (c *controller) mw(s string) string  { return s }
func (c *controller) h(s string) string   { return s }
func (c *controller) h2(s string) string  { return s }

// Register registers the routes.
func (c *controller) Register(r *Router) {
	api := r.Group("/trade")
	api.GET("/x", c.mw, c.h)
}
