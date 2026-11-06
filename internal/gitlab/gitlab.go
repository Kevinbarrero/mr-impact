// Package gitlab posts or updates a single marker-tagged note on a merge request.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrPermission is returned on 401/403 responses.
var ErrPermission = errors.New("GitLab rejected the token: it needs a Project Access Token with api scope and at least Reporter role")

const maxPages = 100

var retryDelays = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// Client talks to the GitLab v4 REST API.
type Client struct {
	BaseURL string // CI_API_V4_URL, e.g. https://gitlab.example.com/api/v4
	Token   string
	HTTP    *http.Client
	Sleep   func(time.Duration) // nil → time.Sleep; tests inject a no-op
}

type note struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

// Upsert finds the newest note (highest id) on the MR whose body contains marker and PUTs body to it, else POSTs.
func (c *Client) Upsert(ctx context.Context, projectID string, mrIID int, marker, body string) error {
	notesURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes", strings.TrimRight(c.BaseURL, "/"), url.PathEscape(projectID), mrIID)

	newest := 0
	next := "1"
	for page := 1; next != ""; page++ {
		if page > maxPages {
			return fmt.Errorf("gitlab: more than %d pages of notes; giving up", maxPages)
		}
		resp, hdr, err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s?per_page=100&page=%s", notesURL, next), nil)
		if err != nil {
			return err
		}
		var ns []note
		if err := json.Unmarshal(resp, &ns); err != nil {
			return fmt.Errorf("gitlab: decode notes: %w", err)
		}
		for _, n := range ns {
			if n.ID > newest && strings.Contains(n.Body, marker) {
				newest = n.ID
			}
		}
		next = strings.TrimSpace(hdr.Get("X-Next-Page"))
		if _, err := strconv.Atoi(next); next != "" && err != nil {
			return fmt.Errorf("gitlab: bad X-Next-Page %q", next)
		}
	}

	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return err
	}
	if newest > 0 {
		_, _, err = c.do(ctx, http.MethodPut, fmt.Sprintf("%s/%d", notesURL, newest), payload)
	} else {
		_, _, err = c.do(ctx, http.MethodPost, notesURL, payload)
	}
	return err
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
	var lastErr error
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
		lastErr = err
		if attempt >= len(retryDelays) {
			return nil, nil, lastErr
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
		return nil, nil, false, fmt.Errorf("gitlab: build request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, true, fmt.Errorf("gitlab: %s %s: %w", method, u, err)
	}
	defer resp.Body.Close()
	body := io.Reader(resp.Body)
	if resp.StatusCode >= 400 { // only a snippet of an error body is used
		body = io.LimitReader(resp.Body, 1<<20)
	}
	data, err = io.ReadAll(body)
	if err != nil {
		return nil, nil, true, fmt.Errorf("gitlab: read response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, nil, false, ErrPermission
	case resp.StatusCode >= 500:
		return nil, nil, true, fmt.Errorf("gitlab: %s %s: status %d: %s", method, u, resp.StatusCode, snippet(data))
	case resp.StatusCode >= 400:
		return nil, nil, false, fmt.Errorf("gitlab: %s %s: status %d: %s", method, u, resp.StatusCode, snippet(data))
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
