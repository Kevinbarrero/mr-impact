package svc

import "net/http"

func Routes(mux *http.ServeMux) {
	mux.HandleFunc("/x", handle)
}

func handle(w http.ResponseWriter, r *http.Request) { helper() }
