package router

// G is a fake router group whose verbs take their handlers variadically.
type G struct{}

func (g *G) POST(p string, hs ...func()) {}
func (g *G) Use(hs ...func())            {}

// Subscribe takes its handler as a plain (non-variadic) argument.
func Subscribe(topic string, h func()) {}

func Register(g *G) {
	g.POST("/x", mw, h)
	Subscribe("topic.a", direct)
	g.Use(u1, u2)
}

func sink(hs []func())  {}
func sink2(hs []func()) {}

func Loop() {
	for _, f := range []func(){r1, r2} {
		f()
	}
}

func Len() {
	hs := []func(){l1}
	_ = len(hs)
	sink(hs)
}

func Append() {
	hs := []func(){a1}
	hs = append(hs, a2)
	sink(hs)
}

func Twice() {
	hs := []func(){t1}
	sink(hs)
	sink2(hs)
}

func Spread(g *G) {
	mws := []func(){s1, s2}
	g.Use(mws...)
	g.Use([]func(){s3}...)
}

func mw()     {}
func h()      {}
func direct() {}
func u1()     {}
func u2()     {}
func r1()     {}
func r2()     {}
func l1()     {}
func a1()     {}
func a2()     {}
func t1()     {}
func s1()     {}
func s2()     {}
func s3()     {}
