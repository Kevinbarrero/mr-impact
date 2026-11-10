package trade

type HandlerFunc func(string) string

type Router struct{}

func (r *Router) Group(path string) *Router          { return r }
func (r *Router) GET(path string, hs ...HandlerFunc) {}

type controller struct{ auth HandlerFunc }

func (c *controller) h(s string) string { return s }

// Register registers the routes.
func (c *controller) Register(r *Router) {
	public := r.Group("/public")
	public.GET("/x", c.h)
}
