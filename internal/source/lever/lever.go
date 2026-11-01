// Package lever fetches published postings from Lever's public postings API
// (https://github.com/lever/postings-api): GET /v0/postings/<site>?mode=json.
// Terms and robots are recorded in docs/stories/M0-04-first-lever-posting-end-to-end.md.
package lever

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shinraxtensei/job-hunter/internal/job"
)

const (
	// BaseURL is Lever's global postings API. (EU sites use api.eu.lever.co.)
	BaseURL = "https://api.lever.co"

	userAgent = "job-hunter/0.1 (+https://github.com/shinraxtensei/job-hunter)"

	// maxBody caps how much of a response we read: the payload is untrusted. The
	// largest real board seen (palantir, 301 postings) is ~6 MiB.
	maxBody = 32 << 20
)

// Client talks to one Lever API host.
// ponytail: one request per Fetch, no rate limiter or retries; M1-03 adds both.
type Client struct {
	base string
	HTTP *http.Client
}

// New returns a client for baseURL (BaseURL in production) with a 30s timeout.
func New(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{Timeout: 30 * time.Second},
	}
}

// Fetch returns every published posting on site's board, plus how many entries were
// skipped as malformed. A malformed entry never fails the whole fetch.
func (c *Client) Fetch(ctx context.Context, site string) ([]job.Raw, int, error) {
	u := c.base + "/v0/postings/" + url.PathEscape(site) + "?mode=json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("lever: fetch %s: %w", site, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("lever: fetch %s: %w", site, err)
	}
	defer func() { _ = resp.Body.Close() }() // read-only body; a close error loses nothing

	if resp.StatusCode != http.StatusOK {
		// The body is untrusted and may be large; only the status goes in the error.
		return nil, 0, fmt.Errorf("lever: fetch %s: unexpected status %s", site, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, 0, fmt.Errorf("lever: fetch %s: reading body: %w", site, err)
	}
	if len(body) > maxBody {
		return nil, 0, fmt.Errorf("lever: fetch %s: response larger than %d bytes", site, maxBody)
	}
	raws, skipped, err := parse(body)
	if err != nil {
		return nil, 0, fmt.Errorf("lever: fetch %s: %w", site, err)
	}
	return raws, skipped, nil
}

// parse splits a postings list into raw records. Each record keeps its element's bytes
// exactly; only id, text and hostedUrl are read, to fill the columns every posting needs.
func parse(body []byte) ([]job.Raw, int, error) {
	var elems []json.RawMessage
	if err := json.Unmarshal(body, &elems); err != nil {
		return nil, 0, fmt.Errorf("decoding postings list: %w", err)
	}
	raws := make([]job.Raw, 0, len(elems))
	skipped := 0
	for _, e := range elems {
		var p struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			HostedURL string `json:"hostedUrl"`
		}
		if err := json.Unmarshal(e, &p); err != nil || p.ID == "" || p.HostedURL == "" {
			skipped++
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
