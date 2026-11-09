package svc

import (
	"net/http"
	"testing"
)

func TestHelper(t *testing.T) {
	helper()
	onlyTested()
	http.NewServeMux().HandleFunc("/t", func(http.ResponseWriter, *http.Request) {})
}
