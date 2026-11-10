package trade

type HandlerFunc func(string) string

type Router struct{}

func (r *Router) Group(path string) *Router           { return r }
func (r *Router) GET(path string, hs ...HandlerFunc)  {}
func (r *Router) POST(path string, hs ...HandlerFunc) {}

type controller struct{ auth, limit HandlerFunc }

func (c *controller) signatureMiddleware(s string) string { return s }
func (c *controller) getStatus(s string) string           { return s }
func (c *controller) create(s string) string              { return s }
func (c *controller) list(s string) string                { return s }

// Register registers the trade routes.
func (c *controller) Register(r *Router) {
	api := r.Group("/trade")
	api.GET("/:id", c.signatureMiddleware, c.getStatus)
	api.POST("", c.limit, c.create)
	api.GET("/all", c.auth, c.list)
}
