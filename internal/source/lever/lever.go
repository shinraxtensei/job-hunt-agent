// Package lever fetches published postings from Lever's public postings API
// (https://github.com/lever/postings-api): GET /v0/postings/<site>?mode=json.
// Terms and robots are recorded in docs/stories/M1-03-lever-adapter.md.
package lever

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/source/fetch"
)

// BaseURL is Lever's global postings API. (EU sites use api.eu.lever.co.)
const BaseURL = "https://api.lever.co"

// Client talks to one Lever API host. HTTP and Backoff come from the embedded
// fetch.Client.
type Client struct {
	*fetch.Client
	base string
}

// Name is the board type this client serves.
func (c *Client) Name() string { return "lever" }

// Host is the API host requests go to, for per-host rate limiting.
func (c *Client) Host() string {
	if u, err := url.Parse(c.base); err == nil && u.Host != "" {
		return u.Host
	}
	return c.base
}

// New returns a client for baseURL (BaseURL in production) with a 30s timeout and a
// 1s first retry delay.
func New(baseURL string) *Client {
	return &Client{Client: fetch.New(), base: strings.TrimRight(baseURL, "/")}
}

// Fetch returns every published posting on site's board, plus how many entries were
// skipped as malformed or unstorable. A bad entry never fails the whole fetch.
// Requests go through fetch.Client.Get: every attempt waits on limit (robots.txt asks
// for Crawl-delay: 1), with retries and Retry-After.
func (c *Client) Fetch(ctx context.Context, site string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	body, err := c.Get(ctx, c.base+"/v0/postings/"+url.PathEscape(site)+"?mode=json", limit)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("lever: fetch %s: %w", site, err)
	}
	raws, skipped, err := parse(body)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("lever: fetch %s: %w", site, err)
	}
	return raws, skipped, nil
}

// parse splits a postings list into raw records. Each record keeps its element's bytes
// exactly; only id, text and hostedUrl are read, to fill the columns every posting needs.
// Entries without an id or URL, or containing NUL (which Postgres refuses in text and
// jsonb), are skipped and counted.
func parse(body []byte) ([]job.Raw, job.Skipped, error) {
	var elems []json.RawMessage
	if err := json.Unmarshal(body, &elems); err != nil {
		return nil, job.Skipped{}, fmt.Errorf("decoding postings list: %w", err)
	}
	raws := make([]job.Raw, 0, len(elems))
	var skipped job.Skipped
	for _, e := range elems {
		var p struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			HostedURL string `json:"hostedUrl"`
		}
		err := json.Unmarshal(e, &p)
		if err != nil || p.ID == "" || p.HostedURL == "" || fetch.HasNUL(e) {
			if err != nil {
				p.ID = "" // a partly decoded entry has no trustworthy ID
			}
			skipped.Add(p.ID)
			continue
		}
		raws = append(raws, job.Raw{
			Source:   "lever",
			SourceID: p.ID,
			URL:      p.HostedURL,
			Title:    p.Text,
			Payload:  e,
		})
	}
	return raws, skipped, nil
}
