package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/Kevinbarrero/mr-impact/internal/github"
	"github.com/Kevinbarrero/mr-impact/internal/report"
)

// poster upserts the note on the merge/pull request it was built for.
type poster interface {
	Upsert(ctx context.Context, body string) error
}

// githubUpserter is the part of github.Client that main uses; tests stub it.
type githubUpserter interface {
	Upsert(ctx context.Context, repo string, number int, marker, body string) error
}

// newGitHubClient builds the GitHub client; tests replace it.
var newGitHubClient = func(baseURL, token, author string) githubUpserter {
	return &github.Client{BaseURL: baseURL, Token: token, Author: author, HTTP: http.DefaultClient}
}

// ciEnv is what the environment says about where the run happens.
type ciEnv struct {
	Kind    string // "gitlab", "github", "local"
	BaseSHA string // GitHub: pull_request.base.sha
	Fork    bool
	Summary string // GITHUB_STEP_SUMMARY path, "" when unset
	Poster  poster // nil in local mode and when there is no token
	// IIDErr is a GitLab CI_MERGE_REQUEST_IID that is not a positive integer.
	IIDErr error
}

type gitlabPoster struct {
	c interface {
		Upsert(ctx context.Context, projectID string, mrIID int, marker, body string) error
	}
	projectID string
	iid       int
}

func (p gitlabPoster) Upsert(ctx context.Context, body string) error {
	return p.c.Upsert(ctx, p.projectID, p.iid, report.Marker, body)
}

type githubPoster struct {
	c      githubUpserter
	repo   string
	number int
}

func (p githubPoster) Upsert(ctx context.Context, body string) error {
	return p.c.Upsert(ctx, p.repo, p.number, report.Marker, body)
}

// detectCI classifies the run. GitLab wins when both are present. An
// unreadable or malformed GitHub event file is an error naming the path; a
// non-pull-request event is local mode.
func detectCI(env func(string) string, readFile func(string) ([]byte, error)) (ciEnv, error) {
	token := env("MRIMPACT_TOKEN")
	if iid, projectID, apiURL := env("CI_MERGE_REQUEST_IID"), env("CI_PROJECT_ID"), env("CI_API_V4_URL"); iid != "" && projectID != "" && apiURL != "" {
		ci := ciEnv{Kind: "gitlab"}
		n, err := strconv.Atoi(iid)
		if err != nil || n < 1 {
			ci.IIDErr = fmt.Errorf("CI_MERGE_REQUEST_IID must be a positive integer, got %q", iid)
			return ci, nil
		}
		if token != "" {
			ci.Poster = gitlabPoster{c: newClient(apiURL, token), projectID: projectID, iid: n}
		}
		return ci, nil
	}
	path := env("GITHUB_EVENT_PATH")
	if env("GITHUB_ACTIONS") != "true" || path == "" {
		return ciEnv{Kind: "local"}, nil
	}
	data, err := readFile(path)
	if err != nil {
		return ciEnv{}, fmt.Errorf("cannot read GitHub event file %s: %w", path, err)
	}
	var ev struct {
		PullRequest *struct {
			Number int `json:"number"`
			Base   struct {
				SHA  string `json:"sha"`
				Repo struct {
					FullName string `json:"full_name"`
				} `json:"repo"`
			} `json:"base"`
			Head struct {
				Repo struct {
					FullName string `json:"full_name"`
				} `json:"repo"`
			} `json:"head"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return ciEnv{}, fmt.Errorf("malformed GitHub event file %s: %w", path, err)
	}
	pr := ev.PullRequest
	if pr == nil {
		return ciEnv{Kind: "local"}, nil
	}
	ci := ciEnv{
		Kind:    "github",
		BaseSHA: pr.Base.SHA,
		Fork:    pr.Head.Repo.FullName != pr.Base.Repo.FullName,
		Summary: env("GITHUB_STEP_SUMMARY"),
	}
	if token != "" && !ci.Fork {
		api := env("GITHUB_API_URL")
		if api == "" {
			api = "https://api.github.com"
		}
		ci.Poster = githubPoster{c: newGitHubClient(api, token, env("MRIMPACT_COMMENT_AUTHOR")), repo: env("GITHUB_REPOSITORY"), number: pr.Number}
	}
	return ci, nil
}

// appendSummary appends note to the GitHub job summary file.
func appendSummary(path, note string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(note); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
