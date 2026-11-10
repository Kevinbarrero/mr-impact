package trade

type HandlerFunc func(string) string

type Router struct{}

func (r *Router) Group(path string) *Router          { return r }
func (r *Router) GET(path string, hs ...HandlerFunc) {}

func register(f func(*Router)) int { return 0 }

func h(s string) string { return s }

// Each func literal is its own scope: g is assigned once in each.
var (
	_ = register(func(r *Router) {
		g := r.Group("/a")
		g.GET("/q", h)
	})
	_ = register(func(r *Router) {
		g := r.Group("/b")
		g.GET("/q", h)
	})
)
