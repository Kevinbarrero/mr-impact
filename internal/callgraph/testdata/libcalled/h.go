package lc

import "net/http"

type H struct{}

func (H) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func Routes() { http.Handle("/x", H{}) }
