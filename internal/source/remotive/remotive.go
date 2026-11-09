// Package remotive fetches remote jobs from Remotive's public API (M7-03, ADR-0029).
// Step 0 (docs/research/m7-sources.md): the API's legal notice allows use with a link back
// to the job's Remotive URL and Remotive named as the source; it asks for at most ~4
// fetches a day and no resubmission to other sites. We fetch at most every 6 hours and only
// show the jobs to the owner, linked and attributed.
//
// One fetch returns jobs of many companies: a board-wide source. Each job keeps its
// company's name in job.Raw.Company and is stored as the JSON object Remotive sent.
package remotive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
	"github.com/shinraxtensei/job-hunt-agent/internal/source/fetch"
)

// URL is the production API. Its `category` parameter is ignored by the API (2026-11-09),
// so categories are filtered here.
const URL = "https://remotive.com/api/remote-jobs"

// Slug is the board's one watch-list slug: data/companies.yaml has a single Remotive row.
const Slug = "remote-jobs"

// categories are the software ones (AC1); the preset filters roles further.
var categories = map[string]bool{"Software Development": true, "DevOps / Sysadmin": true}

// Client reads the Remotive API. HTTP and Backoff come from the embedded fetch.Client.
type Client struct {
	*fetch.Client
	url string
}

// New returns a client for url (URL in production; tests pass their server's).
func New(url string) *Client { return &Client{Client: fetch.New(), url: url} }

// Name is the board type this client serves.
func (c *Client) Name() string { return "remotive" }

// Host is the API host.
func (c *Client) Host() string { return "remotive.com" }

// BoardWide: jobs of many companies (ADR-0029).
func (c *Client) BoardWide() bool { return true }

// MinInterval keeps us within Remotive's "max. 4 times a day".
func (c *Client) MinInterval() time.Duration { return 6 * time.Hour }

// Fetch returns the software jobs on Remotive and how many entries were skipped.
func (c *Client) Fetch(ctx context.Context, slug string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	if slug != Slug {
		return nil, job.Skipped{}, fmt.Errorf("remotive: fetch %q: the board's slug is %q", slug, Slug)
	}
	body, err := c.Get(ctx, c.url, limit)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("remotive: fetch: %w", err)
	}
	raws, skipped, err := parse(body)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("remotive: fetch: %w", err)
	}
	return raws, skipped, nil
}

// entry is what we read of a job; the stored payload is the job object as sent.
type entry struct {
	ID       json.RawMessage `json:"id"`
	URL      string          `json:"url"`
	Title    string          `json:"title"`
	Company  string          `json:"company_name"`
	Category string          `json:"category"`
}

// parse reads the response's jobs. Other categories are dropped (not counted as
// skipped); a job without an id, title, company or a Remotive URL, or with NUL, is
// skipped and counted. A response without a jobs list is an error: an empty result would
// close every Remotive posting.
func parse(body []byte) ([]job.Raw, job.Skipped, error) {
	var doc struct {
		Jobs *[]json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, job.Skipped{}, fmt.Errorf("decoding response: %w", err)
	}
	if doc.Jobs == nil {
		return nil, job.Skipped{}, errors.New("decoding response: no jobs list")
	}
	raws := []job.Raw{}
	var skipped job.Skipped
	for _, obj := range *doc.Jobs {
		var e entry
		if err := json.Unmarshal(obj, &e); err != nil {
			skipped.Add("")
			continue
		}
		id := strings.Trim(string(e.ID), `"`)
		if id == "null" {
			id = ""
		}
		if !categories[e.Category] {
			continue
		}
		e.Title, e.Company = strings.TrimSpace(e.Title), strings.TrimSpace(e.Company)
		if id == "" || e.Title == "" || e.Company == "" ||
			!strings.HasPrefix(e.URL, "https://remotive.com/") || fetch.HasNUL(obj) {
			skipped.Add(id)
			continue
		}
		raws = append(raws, job.Raw{Source: "remotive", SourceID: id, URL: e.URL, Title: e.Title, Company: e.Company, Payload: obj})
	}
	return raws, skipped, nil
}
