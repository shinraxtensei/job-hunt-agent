// Package ashby fetches published jobs from Ashby's public job posting API
// (https://developers.ashbyhq.com/docs/public-job-posting-api):
// GET /posting-api/job-board/<board>?includeCompensation=true.
// Terms and robots are recorded in docs/stories/M1-05-ashby-adapter.md.
package ashby

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/source/fetch"
)

// BaseURL is Ashby's public posting API.
const BaseURL = "https://api.ashbyhq.com"

// Client talks to the Ashby posting API. HTTP and Backoff come from the embedded
// fetch.Client.
type Client struct {
	*fetch.Client
	base string
}

// New returns a client for baseURL (BaseURL in production) with a 30s timeout and a
// 1s first retry delay.
func New(baseURL string) *Client {
	return &Client{Client: fetch.New(), base: strings.TrimRight(baseURL, "/")}
}

// Name is the board type this client serves.
func (c *Client) Name() string { return "ashby" }

// Host is the API host requests go to, for per-host rate limiting.
func (c *Client) Host() string {
	if u, err := url.Parse(c.base); err == nil && u.Host != "" {
		return u.Host
	}
	return c.base
}

// Fetch returns every listed job on the board, with descriptions and compensation, and
// how many jobs were skipped as malformed or unstorable. A bad job never fails the whole
// fetch. Requests go through fetch.Client.Get: every attempt waits on limit, with
// retries and Retry-After.
func (c *Client) Fetch(ctx context.Context, board string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	u := c.base + "/posting-api/job-board/" + url.PathEscape(board) + "?includeCompensation=true"
	body, err := c.Get(ctx, u, limit)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("ashby: fetch %s: %w", board, err)
	}
	raws, skipped, err := parse(body)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("ashby: fetch %s: %w", board, err)
	}
	return raws, skipped, nil
}

// parse splits a job board document into raw records. Each record keeps its job's
// bytes exactly; only id, title, jobUrl and isListed are read. id is in every real
// response though the docs don't list it; jobs without it or a jobUrl, or containing
// NUL (which Postgres refuses), are skipped and counted. Jobs with isListed false are
// meant only for people with the direct link, so they are left out and not counted.
// A document with no jobs array is an error: the API changed, and an empty result would
// hide it.
func parse(body []byte) ([]job.Raw, job.Skipped, error) {
	var doc struct {
		Jobs *[]json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, job.Skipped{}, fmt.Errorf("decoding job board: %w", err)
	}
	if doc.Jobs == nil {
		return nil, job.Skipped{}, errors.New(`decoding job board: no "jobs" array in the response`)
	}
	raws := make([]job.Raw, 0, len(*doc.Jobs))
	var skipped job.Skipped
	for _, e := range *doc.Jobs {
		var j struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			JobURL   string `json:"jobUrl"`
			IsListed *bool  `json:"isListed"`
		}
		err := json.Unmarshal(e, &j)
		if err != nil || j.ID == "" || j.JobURL == "" || fetch.HasNUL(e) {
			if err != nil {
				j.ID = "" // a partly decoded entry has no trustworthy ID
			}
			skipped.Add(j.ID)
			continue
		}
		if j.IsListed != nil && !*j.IsListed {
			continue
		}
		raws = append(raws, job.Raw{
			Source:   "ashby",
			SourceID: j.ID,
			URL:      j.JobURL,
			Title:    j.Title,
			Payload:  e,
		})
	}
	return raws, skipped, nil
}
