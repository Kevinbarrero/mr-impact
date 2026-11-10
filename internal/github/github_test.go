package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const marker = "<!-- mrimpact:v1 -->"

type rec struct {
	method, path, body string
	hdr                http.Header
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

func (f *fake) last(method string) rec {
	for i := len(f.reqs) - 1; i >= 0; i-- {
		if f.reqs[i].method == method {
			return f.reqs[i]
		}
	}
	return rec{}
}

func newServer(t *testing.T, h func(srvURL string, w http.ResponseWriter, r *http.Request)) (*Client, *fake, *[]time.Duration) {
	t.Helper()
	f := &fake{}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, rec{r.Method, r.URL.RequestURI(), string(b), r.Header.Clone()})
		f.mu.Unlock()
		h(srv.URL, w, r)
	}))
	t.Cleanup(srv.Close)
	var sleeps []time.Duration
	c := &Client{BaseURL: srv.URL, Token: "secret-tok", Sleep: func(d time.Duration) { sleeps = append(sleeps, d) }}
	return c, f, &sleeps
}

func cm(id int, login, body string) map[string]any {
	return map[string]any{"id": id, "body": body, "user": map[string]any{"login": login}}
}

func list(w http.ResponseWriter, cs ...map[string]any) { json.NewEncoder(w).Encode(cs) }

const bot = "github-actions[bot]"

func TestUpsert_CreatesWhenNone(t *testing.T) {
	c, f, _ := newServer(t, func(_ string, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			list(w, cm(1, bot, "hello"))
			return
		}
		w.WriteHeader(201)
	})
	if err := c.Upsert(context.Background(), "own er/na me", 7, marker, marker+"\nbody"); err != nil {
		t.Fatal(err)
	}
	if f.count("POST") != 1 || f.count("PATCH") != 0 {
		t.Fatalf("reqs: %+v", f.reqs)
	}
	if got := f.reqs[0].path; got != "/repos/own%20er/na%20me/issues/7/comments?per_page=100" {
		t.Errorf("list path %q", got)
	}
	p := f.last("POST")
	if p.path != "/repos/own%20er/na%20me/issues/7/comments" {
		t.Errorf("post path %q", p.path)
	}
	var pl map[string]string
	json.Unmarshal([]byte(p.body), &pl)
	if pl["body"] != marker+"\nbody" {
		t.Errorf("payload %q", p.body)
	}
}

func TestUpsert_UpdatesNewestOwnMarker(t *testing.T) {
	c, f, _ := newServer(t, func(_ string, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			list(w, cm(5, bot, marker+" old"), cm(9, bot, marker+" newer"), cm(12, bot, "no marker"))
			return
		}
		w.WriteHeader(200)
	})
	if err := c.Upsert(context.Background(), "o/r", 1, marker, "new"); err != nil {
		t.Fatal(err)
	}
	if f.count("PATCH") != 1 || f.count("POST") != 0 {
		t.Fatalf("reqs: %+v", f.reqs)
	}
	if got := f.last("PATCH").path; got != "/repos/o/r/issues/comments/9" {
		t.Errorf("patch path %q", got)
	}
}

func TestUpsert_IgnoresMarkerFromOtherAuthor(t *testing.T) {
	c, f, _ := newServer(t, func(_ string, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			list(w, cm(3, "mallory", marker+" spoof"))
			return
		}
		w.WriteHeader(201)
	})
	if err := c.Upsert(context.Background(), "o/r", 1, marker, "new"); err != nil {
		t.Fatal(err)
	}
	if f.count("PATCH") != 0 || f.count("POST") != 1 {
		t.Fatalf("reqs: %+v", f.reqs)
	}
}

func TestUpsert_FollowsLinkPagination(t *testing.T) {
	c, f, _ := newServer(t, func(base string, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if r.URL.Query().Get("page") == "2" {
				list(w, cm(8, bot, marker))
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/issues/1/comments?per_page=100&page=2>; rel="next", <%s/x?page=2>; rel="last"`, base, base))
			list(w, cm(1, bot, "x"))
			return
		}
		w.WriteHeader(200)
	})
	if err := c.Upsert(context.Background(), "o/r", 1, marker, "new"); err != nil {
		t.Fatal(err)
	}
	if f.count("GET") != 2 || f.count("PATCH") != 1 || f.count("POST") != 0 {
		t.Fatalf("reqs: %+v", f.reqs)
	}
	if got := f.last("PATCH").path; got != "/repos/o/r/issues/comments/8" {
		t.Errorf("patch path %q", got)
	}
}

func TestUpsert_PatchRefusedFallsBackToCreate(t *testing.T) {
	for _, code := range []int{403, 404} {
		c, f, _ := newServer(t, func(_ string, w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "GET":
				list(w, cm(4, bot, marker))
			case "PATCH":
				w.WriteHeader(code)
			default:
				w.WriteHeader(201)
			}
		})
		if err := c.Upsert(context.Background(), "o/r", 1, marker, "new"); err != nil {
			t.Fatalf("%d: %v", code, err)
		}
		if f.count("PATCH") != 1 || f.count("POST") != 1 {
			t.Fatalf("%d reqs: %+v", code, f.reqs)
		}
	}
}

func TestUpsert_PatchOtherErrorReturned(t *testing.T) {
	c, f, _ := newServer(t, func(_ string, w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			list(w, cm(4, bot, marker))
		default:
			w.WriteHeader(422)
		}
	})
	if err := c.Upsert(context.Background(), "o/r", 1, marker, "new"); err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("err %v", err)
	}
	if f.count("POST") != 0 {
		t.Fatal("must not POST")
	}
}

func TestUpsert_Unauthorized(t *testing.T) {
	c, _, _ := newServer(t, func(_ string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"message":"Bad credentials"}`)
	})
	err := c.Upsert(context.Background(), "o/r", 1, marker, "b")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), "secret-tok") {
		t.Fatal("token leaked")
	}
}

func TestUpsert_RetriesServerErrors(t *testing.T) {
	n := 0
	c, _, sleeps := newServer(t, func(_ string, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			n++
			if n <= 2 {
				w.WriteHeader(502)
				return
			}
			list(w)
			return
		}
		w.WriteHeader(201)
	})
	if err := c.Upsert(context.Background(), "o/r", 1, marker, "b"); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{time.Second, 2 * time.Second}
	if len(*sleeps) != 2 || (*sleeps)[0] != want[0] || (*sleeps)[1] != want[1] {
		t.Errorf("sleeps %v", *sleeps)
	}
}

func TestUpsert_Headers(t *testing.T) {
	c, f, _ := newServer(t, func(_ string, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			list(w)
			return
		}
		w.WriteHeader(201)
	})
	if err := c.Upsert(context.Background(), "o/r", 1, marker, "b"); err != nil {
		t.Fatal(err)
	}
	if len(f.reqs) != 2 {
		t.Fatalf("reqs %d", len(f.reqs))
	}
	for _, r := range f.reqs {
		if r.hdr.Get("Authorization") != "Bearer secret-tok" ||
			r.hdr.Get("Accept") != "application/vnd.github+json" ||
			r.hdr.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Errorf("%s headers %v", r.method, r.hdr)
		}
	}
}

func TestUpsert_CustomAuthor(t *testing.T) {
	c, f, _ := newServer(t, func(_ string, w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			list(w, cm(2, bot, marker), cm(3, "ci-user", marker))
			return
		}
		w.WriteHeader(200)
	})
	c.Author = "ci-user"
	if err := c.Upsert(context.Background(), "o/r", 1, marker, "b"); err != nil {
		t.Fatal(err)
	}
	if got := f.last("PATCH").path; got != "/repos/o/r/issues/comments/3" {
		t.Errorf("patch path %q", got)
	}
}
