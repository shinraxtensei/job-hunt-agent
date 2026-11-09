// Package remoteok fetches remote jobs from Remote OK's public API (M7-03, ADR-0029).
// Step 0 (docs/research/m7-sources.md): the API's terms ask for a link back to the job's
// Remote OK URL and "Remote OK" named as the source, and no use of its logo. We show each
// job to the owner only, linked and attributed, and never load the logo.
//
// One fetch returns jobs of many companies: a board-wide source. The response is a JSON
// array whose first element is the legal notice; each job is stored as Remote OK sent it.
package remoteok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/source/fetch"
)

// URL is the production API.
const URL = "https://remoteok.com/api"

// Slug is the board's one watch-list slug.
const Slug = "jobs"

// prefix is where every job's URL must point (Remote OK writes the host "remoteOK.com").
const prefix = "https://remoteok.com/"

// tech tags: Remote OK has no categories and tags generously ("dev" on an accountant), so
// this is a first cut; the preset's role filter does the rest (AC4).
var tech = []string{"dev", "engineer"}

// Client reads the Remote OK API. HTTP and Backoff come from the embedded fetch.Client.
type Client struct {
	*fetch.Client
	url string
}

// New returns a client for url (URL in production; tests pass their server's).
func New(url string) *Client { return &Client{Client: fetch.New(), url: url} }

// Name is the board type this client serves.
func (c *Client) Name() string { return "remoteok" }

// Host is the API host.
func (c *Client) Host() string { return "remoteok.com" }

// BoardWide: jobs of many companies (ADR-0029).
func (c *Client) BoardWide() bool { return true }

// MinInterval: Remote OK states no limit; its feed changes slowly, so as Remotive.
func (c *Client) MinInterval() time.Duration { return 6 * time.Hour }

// Fetch returns Remote OK's jobs tagged dev or engineer and how many were skipped.
func (c *Client) Fetch(ctx context.Context, slug string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	if slug != Slug {
		return nil, job.Skipped{}, fmt.Errorf("remoteok: fetch %q: the board's slug is %q", slug, Slug)
	}
	body, err := c.Get(ctx, c.url, limit)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("remoteok: fetch: %w", err)
	}
	raws, skipped, err := parse(body)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("remoteok: fetch: %w", err)
	}
	return raws, skipped, nil
}

type entry struct {
	Legal    *string  `json:"legal"`
	ID       string   `json:"id"`
	Position string   `json:"position"`
	Company  string   `json:"company"`
	Tags     []string `json:"tags"`
	URL      string   `json:"url"`
}

// parse reads the array. The legal notice (the element with "legal") is required: without
// it the response isn't Remote OK's job list, and an empty result would close every Remote
// OK posting. Jobs without the tech tags are dropped; a job without id, position, company
// or a Remote OK URL, or with NUL, is skipped and counted.
func parse(body []byte) ([]job.Raw, job.Skipped, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, job.Skipped{}, fmt.Errorf("decoding response: %w", err)
	}
	raws := []job.Raw{}
	var skipped job.Skipped
	legal := false
	for _, obj := range items {
		var e entry
		if err := json.Unmarshal(obj, &e); err != nil {
			skipped.Add(probeID(obj))
			continue
		}
		if e.Legal != nil {
			legal = true
			continue
		}
		if !slices.ContainsFunc(e.Tags, func(t string) bool { return slices.Contains(tech, t) }) {
			continue
		}
		e.Position, e.Company = strings.TrimSpace(job.FixText(e.Position)), strings.TrimSpace(job.FixText(e.Company))
		if len(e.URL) >= len(prefix) && strings.EqualFold(e.URL[:len(prefix)], prefix) {
			e.URL = prefix + e.URL[len(prefix):]
		} else {
			e.URL = ""
		}
		if e.ID == "" || e.Position == "" || e.Company == "" || e.URL == "" || fetch.HasNUL(obj) {
			skipped.Add(e.ID)
			continue
		}
		raws = append(raws, job.Raw{Source: "remoteok", SourceID: e.ID, URL: e.URL, Title: e.Position, Company: e.Company, Payload: obj})
	}
	if !legal {
		return nil, job.Skipped{}, errors.New("decoding response: no legal notice, not Remote OK's job list")
	}
	return raws, skipped, nil
}

// probeID reads the id of an entry that didn't decode, so it counts as seen.
func probeID(obj json.RawMessage) string {
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(obj, &p) // unreadable: no id
	return p.ID
}
