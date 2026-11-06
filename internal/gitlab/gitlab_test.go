package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const marker = "<!-- mrimpact:v1 -->"

type rec struct {
	method, path, body, token string
}

type fake struct {
	mu   sync.Mutex
	reqs []rec
}

func (f *fake) count(method string) int {
	n := 0
	for _, r := range f.reqs {
		if r.method == method {
			n++
		}
	}
	return n
}

func newServer(t *testing.T, h func(f *fake, w http.ResponseWriter, r *http.Request)) (*Client, *fake, *[]time.Duration) {
	t.Helper()
	f := &fake{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, rec{r.Method, r.URL.RequestURI(), string(b), r.Header.Get("PRIVATE-TOKEN")})
		f.mu.Unlock()
		h(f, w, r)
	}))
	t.Cleanup(srv.Close)
	var sleeps []time.Duration
	c := &Client{BaseURL: srv.URL + "/api/v4", Token: "secret-tok", Sleep: func(d time.Duration) { sleeps = append(sleeps, d) }}
	return c, f, &sleeps
}

func notes(w http.ResponseWriter, next string, ns ...map[string]any) {
	if next != "" {
		w.Header().Set("X-Next-Page", next)
	}
	json.NewEncoder(w).Encode(ns)
}

func TestUpsertCreates(t *testing.T) {
	c, f, _ := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			notes(w, "", map[string]any{"id": 1, "body": "hello"})
			return
		}
		w.WriteHeader(201)
	})
	if err := c.Upsert(context.Background(), "grp/proj", 7, marker, marker+"\nbody"); err != nil {
		t.Fatal(err)
	}
	if f.count("POST") != 1 || f.count("PUT") != 0 {
		t.Fatalf("reqs: %+v", f.reqs)
	}
	last := f.reqs[len(f.reqs)-1]
	if last.path != "/api/v4/projects/grp%2Fproj/merge_requests/7/notes" {
		t.Errorf("path %q", last.path)
	}
	if last.token != "secret-tok" {
		t.Errorf("token header %q", last.token)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(last.body), &got); err != nil || got["body"] != marker+"\nbody" {
		t.Errorf("body %q err %v", last.body, err)
	}
	if !strings.HasPrefix(f.reqs[0].path, "/api/v4/projects/grp%2Fproj/merge_requests/7/notes?") || !strings.Contains(f.reqs[0].path, "per_page=100") {
		t.Errorf("list path %q", f.reqs[0].path)
	}
}

func TestUpsertUpdatesExisting(t *testing.T) {
	c, f, _ := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			notes(w, "", map[string]any{"id": 3, "body": "x"}, map[string]any{"id": 4, "body": marker + " old"})
		}
	})
	if err := c.Upsert(context.Background(), "12", 7, marker, "new"); err != nil {
		t.Fatal(err)
	}
	last := f.reqs[len(f.reqs)-1]
	if last.method != "PUT" || last.path != "/api/v4/projects/12/merge_requests/7/notes/4" || f.count("POST") != 0 {
		t.Fatalf("reqs: %+v", f.reqs)
	}
}

func TestUpsertPagination(t *testing.T) {
	c, f, _ := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			return
		}
		if strings.Contains(r.URL.RawQuery, "page=2") {
			notes(w, "", map[string]any{"id": 20, "body": marker})
			return
		}
		notes(w, "2", map[string]any{"id": 1, "body": "a"})
	})
	if err := c.Upsert(context.Background(), "12", 7, marker, "new"); err != nil {
		t.Fatal(err)
	}
	if f.count("PUT") != 1 || f.count("POST") != 0 || f.count("GET") != 2 {
		t.Fatalf("reqs: %+v", f.reqs)
	}
}

func TestUpsertDuplicatesNewest(t *testing.T) {
	c, f, _ := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			notes(w, "", map[string]any{"id": 9, "body": marker}, map[string]any{"id": 5, "body": marker})
		}
	})
	if err := c.Upsert(context.Background(), "12", 7, marker, "new"); err != nil {
		t.Fatal(err)
	}
	last := f.reqs[len(f.reqs)-1]
	if last.method != "PUT" || !strings.HasSuffix(last.path, "/notes/9") {
		t.Fatalf("reqs: %+v", f.reqs)
	}
}

func TestUpsert403(t *testing.T) {
	for _, code := range []int{401, 403} {
		c, f, sleeps := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) })
		err := c.Upsert(context.Background(), "12", 7, marker, "b")
		if !errors.Is(err, ErrPermission) {
			t.Fatalf("%d: err = %v", code, err)
		}
		if len(f.reqs) != 1 || len(*sleeps) != 0 {
			t.Fatalf("%d: reqs %d sleeps %v", code, len(f.reqs), *sleeps)
		}
		if strings.Contains(err.Error(), "secret-tok") {
			t.Fatal("token leaked")
		}
	}
}

func TestUpsertOther4xx(t *testing.T) {
	c, f, _ := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, `{"message":"404 Not Found"}`)
	})
	err := c.Upsert(context.Background(), "12", 7, marker, "b")
	if err == nil || errors.Is(err, ErrPermission) || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "Not Found") {
		t.Fatalf("err = %v", err)
	}
	if len(f.reqs) != 1 {
		t.Fatalf("reqs %d", len(f.reqs))
	}
}

func TestUpsertRetries5xx(t *testing.T) {
	n := 0
	c, _, sleeps := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		n++
		if n <= 2 {
			w.WriteHeader(502)
			return
		}
		if r.Method == "GET" {
			notes(w, "")
		}
	})
	if err := c.Upsert(context.Background(), "12", 7, marker, "b"); err != nil {
		t.Fatal(err)
	}
	if want := []time.Duration{time.Second, 2 * time.Second}; !reflect.DeepEqual(*sleeps, want) {
		t.Fatalf("sleeps %v", *sleeps)
	}
}

func TestUpsertGivesUp(t *testing.T) {
	c, f, sleeps := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	err := c.Upsert(context.Background(), "12", 7, marker, "b")
	if err == nil || len(f.reqs) != 4 {
		t.Fatalf("err %v reqs %d", err, len(f.reqs))
	}
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; !reflect.DeepEqual(*sleeps, want) {
		t.Fatalf("sleeps %v", *sleeps)
	}
}

func TestUpsertNetworkErrorRetries(t *testing.T) {
	c, _, sleeps := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) {})
	c.BaseURL = "http://127.0.0.1:1/api/v4" // connection refused
	err := c.Upsert(context.Background(), "12", 7, marker, "b")
	if err == nil || len(*sleeps) != 3 {
		t.Fatalf("err %v sleeps %v", err, *sleeps)
	}
	if strings.Contains(err.Error(), "secret-tok") {
		t.Fatal("token leaked")
	}
}

func TestUpsertContextCancel(t *testing.T) {
	c, f, _ := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	ctx, cancel := context.WithCancel(context.Background())
	c.Sleep = func(time.Duration) { cancel() }
	err := c.Upsert(ctx, "12", 7, marker, "b")
	if !errors.Is(err, context.Canceled) || len(f.reqs) != 1 {
		t.Fatalf("err %v reqs %d", err, len(f.reqs))
	}
}

func TestUpsertPageCap(t *testing.T) {
	c, f, _ := newServer(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		notes(w, "2", map[string]any{"id": 1, "body": "a"})
	})
	err := c.Upsert(context.Background(), "12", 7, marker, "b")
	if err == nil || f.count("GET") != 100 || f.count("POST") != 0 {
		t.Fatalf("err %v gets %d", err, f.count("GET"))
	}
}
