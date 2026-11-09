// Package greenhouse fetches published jobs from Greenhouse's public Job Board API
// (https://docs.greenhouse.io/job-board.html):
// GET /v1/boards/<board_token>/jobs?content=true.
// Terms and robots are recorded in docs/stories/M1-04-greenhouse-adapter.md.
package greenhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
	"github.com/shinraxtensei/job-hunt-agent/internal/source/fetch"
)

// BaseURL is Greenhouse's Job Board API.
const BaseURL = "https://boards-api.greenhouse.io"

// Client talks to the Greenhouse Job Board API. HTTP and Backoff come from the embedded
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
func (c *Client) Name() string { return "greenhouse" }

// Host is the API host requests go to, for per-host rate limiting.
func (c *Client) Host() string {
	if u, err := url.Parse(c.base); err == nil && u.Host != "" {
		return u.Host
	}
	return c.base
}

// Fetch returns every published job on the board, with descriptions (content=true), and
// how many jobs were skipped as malformed or unstorable. A bad job never fails the whole
// fetch. Requests go through fetch.Client.Get: every attempt waits on limit, with
// retries and Retry-After.
func (c *Client) Fetch(ctx context.Context, board string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	body, err := c.Get(ctx, c.base+"/v1/boards/"+url.PathEscape(board)+"/jobs?content=true", limit)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("greenhouse: fetch %s: %w", board, err)
	}
	raws, skipped, err := parse(body)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("greenhouse: fetch %s: %w", board, err)
	}
	return raws, skipped, nil
}

// FetchOne returns one job by board and id from Greenhouse's API (M2-07). The URL is built
// here from the parsed board and id; a pasted link is never requested. A 404 is
// fetch.ErrNotFound.
func (c *Client) FetchOne(ctx context.Context, board, id string, limit *rate.Limiter) (job.Raw, error) {
	body, err := c.Get(ctx, c.base+"/v1/boards/"+url.PathEscape(board)+"/jobs/"+url.PathEscape(id), limit)
	if err != nil {
		return job.Raw{}, fmt.Errorf("greenhouse: fetch %s/%s: %w", board, id, err)
	}
	raws, _, err := parse(append(append([]byte(`{"jobs":[`), body...), ']', '}'))
	if err != nil || len(raws) != 1 || raws[0].SourceID != id {
		return job.Raw{}, fmt.Errorf("greenhouse: fetch %s/%s: not the job asked for", board, id)
	}
	return raws[0], nil
}

// parse splits a jobs document into raw records. Each record keeps its job's bytes
// exactly; only id, title and absolute_url are read, to fill the columns every posting
// needs. Jobs without a positive integer id or a URL, or containing NUL (which Postgres
// refuses), are skipped and counted. A document with no jobs array is an error: the API
// changed, and an empty result would hide it.
func parse(body []byte) ([]job.Raw, job.Skipped, error) {
	var doc struct {
		Jobs *[]json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, job.Skipped{}, fmt.Errorf("decoding jobs: %w", err)
	}
	if doc.Jobs == nil {
		return nil, job.Skipped{}, errors.New(`decoding jobs: no "jobs" array in the response`)
	}
	raws := make([]job.Raw, 0, len(*doc.Jobs))
	var skipped job.Skipped
	for _, e := range *doc.Jobs {
		var j struct {
			ID          json.Number `json:"id"`
			Title       string      `json:"title"`
			AbsoluteURL string      `json:"absolute_url"`
		}
		err := json.Unmarshal(e, &j)
		id, idErr := strconv.ParseInt(j.ID.String(), 10, 64)
		readable := ""
		if err == nil && idErr == nil && id > 0 {
			readable = strconv.FormatInt(id, 10)
		}
		if readable == "" || j.AbsoluteURL == "" || fetch.HasNUL(e) {
			skipped.Add(readable)
			continue
		}
		raws = append(raws, job.Raw{
			Source:   "greenhouse",
			SourceID: strconv.FormatInt(id, 10),
			URL:      j.AbsoluteURL,
			Title:    j.Title,
			Payload:  e,
		})
	}
	return raws, skipped, nil
}
