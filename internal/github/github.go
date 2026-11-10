// Package github posts or updates a single marker-tagged comment on a pull request.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.github.com"
	defaultAuthor  = "github-actions[bot]"
	maxPages       = 100
)

var retryDelays = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// Client talks to the GitHub REST API.
type Client struct {
	BaseURL string // GITHUB_API_URL, default https://api.github.com
	Token   string
	Author  string // expected comment author; "" → "github-actions[bot]"
	HTTP    *http.Client
	Sleep   func(time.Duration) // nil → time.Sleep; tests inject a no-op
}

type comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

// statusError is a non-2xx answer; the caller can inspect the code.
type statusError struct {
	method, path string
	code         int
	snippet      string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("github: %s %s: %d: %s", e.method, e.path, e.code, e.snippet)
}

// Upsert finds the newest comment (highest id) on the pull request whose body contains marker and whose
// author is c.Author, and PATCHes body onto it; with none, or when the PATCH is refused (403/404), it POSTs.
// repo is "owner/name".
func (c *Client) Upsert(ctx context.Context, repo string, number int, marker, body string) error {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("github: repository %q is not owner/name", repo)
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	author := c.Author
	if author == "" {
		author = defaultAuthor
	}
	repoURL := fmt.Sprintf("%s/repos/%s/%s", base, url.PathEscape(owner), url.PathEscape(name))
	commentsURL := fmt.Sprintf("%s/issues/%d/comments", repoURL, number)

	var newest int64
	next := commentsURL + "?per_page=100"
	for page := 1; next != ""; page++ {
		if page > maxPages {
			return fmt.Errorf("github: more than %d pages of comments; giving up", maxPages)
		}
		data, hdr, err := c.do(ctx, http.MethodGet, next, nil)
		if err != nil {
			return err
		}
		var cs []comment
		if err := json.Unmarshal(data, &cs); err != nil {
			return fmt.Errorf("github: decode comments: %w", err)
		}
		for _, cm := range cs {
			if cm.ID > newest && cm.User.Login == author && strings.Contains(cm.Body, marker) {
				newest = cm.ID
			}
		}
		next, err = c.followable(base, next, hdr.Get("Link"))
		if err != nil {
			return err
		}
	}

	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return err
	}
	if newest > 0 {
		_, _, err = c.do(ctx, http.MethodPatch, fmt.Sprintf("%s/issues/comments/%d", repoURL, newest), payload)
		var se *statusError
		if err == nil || !errors.As(err, &se) || (se.code != http.StatusForbidden && se.code != http.StatusNotFound) {
			return err
		}
	}
	_, _, err = c.do(ctx, http.MethodPost, commentsURL, payload)
	return err
}

// followable resolves the rel="next" URL of a Link header against the request URL cur and accepts it only
// when its scheme and host equal base's, so the token never goes to another host. "" means no next page.
func (c *Client) followable(base, cur, link string) (string, error) {
	n := nextLink(link)
	if n == "" {
		return "", nil
	}
	cu, err := url.Parse(cur)
	if err != nil {
		return "", fmt.Errorf("github: bad request URL: %w", err)
	}
	nu, err := url.Parse(n)
	if err != nil {
		return "", fmt.Errorf("github: unparseable pagination link")
	}
	nu = cu.ResolveReference(nu)
	bu, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("github: bad base URL: %w", err)
	}
	if !strings.EqualFold(nu.Scheme, bu.Scheme) || !strings.EqualFold(nu.Host, bu.Host) {
		return "", fmt.Errorf("github: pagination link points to another host")
	}
	return nu.String(), nil
}

var nextRe = regexp.MustCompile(`<([^>]*)>\s*;[^,]*\brel="?next"?`)

// nextLink returns the rel="next" URL of a Link header, or "".
func nextLink(h string) string {
	if m := nextRe.FindStringSubmatch(h); m != nil {
		return m[1]
	}
	return ""
}

// do performs one request with retries on network errors and 5xx.
func (c *Client) do(ctx context.Context, method, u string, payload []byte) ([]byte, http.Header, error) {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		data, hdr, retry, err := c.once(ctx, hc, method, u, payload)
		if err == nil {
			return data, hdr, nil
		}
		if !retry {
			return nil, nil, err
		}
		if cerr := ctx.Err(); cerr != nil {
			return nil, nil, cerr
		}
		if attempt >= len(retryDelays) {
			return nil, nil, err
		}
		sleep(retryDelays[attempt])
	}
}

func (c *Client) once(ctx context.Context, hc *http.Client, method, u string, payload []byte) (data []byte, hdr http.Header, retry bool, err error) {
	var rd io.Reader
	if payload != nil {
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, nil, false, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	path := u
	if pu, perr := url.Parse(u); perr == nil {
		path = pu.RequestURI()
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, true, fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body := io.Reader(resp.Body)
	if resp.StatusCode >= 400 { // only a snippet of an error body is used
		body = io.LimitReader(resp.Body, 1<<20)
	}
	data, err = io.ReadAll(body)
	if err != nil {
		return nil, nil, true, fmt.Errorf("github: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, nil, resp.StatusCode >= 500, &statusError{method, path, resp.StatusCode, snippet(data)}
	}
	return data, resp.Header, false, nil
}

func snippet(b []byte) string {
	const max = 500
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		s = s[:max] + "..."
	}
	return s
}
