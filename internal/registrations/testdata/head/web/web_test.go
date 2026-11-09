package web

import "testing"

func TestSetup(t *testing.T) {
	r := &Router{routes: map[string]HandlerFunc{}}
	r.GET("/test-only", show)
	Setup(r)
}
