package lc

import "net/http"

type H struct{}

func (H) ServeHTTP(http.ResponseWriter, *http.Request) {}

func Routes() { http.Handle("/x", H{}) }
