// Package lever fetches published postings from Lever's public postings API
// (https://github.com/lever/postings-api): GET /v0/postings/<site>?mode=json.
// Terms and robots are recorded in docs/stories/M1-03-lever-adapter.md.
package lever

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/job"
)

const (
	// BaseURL is Lever's global postings API. (EU sites use api.eu.lever.co.)
	BaseURL = "https://api.lever.co"

	userAgent = "job-hunter/0.1 (+https://github.com/shinraxtensei/job-hunter)"

	// maxBody caps how much of a response we read: the payload is untrusted. The
	// largest real board seen (palantir, 301 postings) is ~6 MiB.
	maxBody = 32 << 20

	// maxAttempts bounds retries of one Fetch (source-adapter skill: at most 3).
	maxAttempts = 3

	// maxRetryAfter is the longest Retry-After we sit out; longer means give up on this
	// board for this run rather than stall the whole run.
	maxRetryAfter = time.Minute
)

// Client talks to one Lever API host.
type Client struct {
	base string
	HTTP *http.Client
	// Backoff is the first retry delay; each retry doubles it, plus up to 50% jitter.
	Backoff time.Duration
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
	return &Client{
		base:    strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Backoff: time.Second,
	}
}

// Fetch returns every published posting on site's board, plus how many entries were
// skipped as malformed or unstorable. A bad entry never fails the whole fetch.
//
// Every request, retries included, first waits on limit: the host's limiter, shared
// by every company on this host (robots.txt asks for Crawl-delay: 1). It retries 429,
// 5xx, timeouts and network errors, at most maxAttempts times in all, with exponential
// backoff and jitter, honouring Retry-After.
func (c *Client) Fetch(ctx context.Context, site string, limit *rate.Limiter) ([]job.Raw, int, error) {
	u := c.base + "/v0/postings/" + url.PathEscape(site) + "?mode=json"
	var lastErr error
	for attempt := 1; ; attempt++ {
		if err := limit.Wait(ctx); err != nil {
			return nil, 0, fmt.Errorf("lever: fetch %s: %w", site, err)
		}
		body, retryAfter, err := c.get(ctx, u)
		if err == nil {
			raws, skipped, err := parse(body)
			if err != nil {
				return nil, 0, fmt.Errorf("lever: fetch %s: %w", site, err)
			}
			return raws, skipped, nil
		}
		lastErr = err
		var perm permanent
		if errors.As(err, &perm) || ctx.Err() != nil || attempt == maxAttempts {
			break
		}
		if retryAfter > maxRetryAfter {
			lastErr = fmt.Errorf("%w (Retry-After %v is longer than %v; not waiting)", err, retryAfter, maxRetryAfter)
			break
		}
		delay := max(retryAfter, c.backoff(attempt))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, 0, fmt.Errorf("lever: fetch %s: %w", site, ctx.Err())
		}
	}
	return nil, 0, fmt.Errorf("lever: fetch %s: %w", site, lastErr)
}

// permanent marks an error that retrying won't fix (4xx other than 429, an oversized
// body).
type permanent struct{ error }

// get makes one request. It returns the body on 200; otherwise an error, and for 429
// or 503 the server's Retry-After, if any.
func (c *Client) get(ctx context.Context, u string) ([]byte, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, permanent{err}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err // timeouts and network errors: retryable
	}
	defer func() { _ = resp.Body.Close() }() // read-only body; a close error loses nothing

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		// The body is untrusted and may be large; only the status goes in the error.
		return nil, retryAfter(resp.Header.Get("Retry-After")), fmt.Errorf("unexpected status %s", resp.Status)
	default:
		return nil, 0, permanent{fmt.Errorf("unexpected status %s", resp.Status)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, 0, fmt.Errorf("reading body: %w", err)
	}
	if len(body) > maxBody {
		return nil, 0, permanent{fmt.Errorf("response larger than %d bytes", maxBody)}
	}
	return body, 0, nil
}

// backoff is the delay before retry number attempt: Backoff * 2^(attempt-1), plus up to
// 50% jitter so many clients don't retry in lockstep.
func (c *Client) backoff(attempt int) time.Duration {
	d := c.Backoff << (attempt - 1)
	return d + rand.N(d/2+1) //nolint:gosec // jitter, not security
}

// retryAfter parses a Retry-After header: seconds, or an HTTP date. Zero if absent or
// unparseable.
func retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0)
	}
	return 0
}

// parse splits a postings list into raw records. Each record keeps its element's bytes
// exactly; only id, text and hostedUrl are read, to fill the columns every posting needs.
// Entries without an id or URL, or containing NUL (which Postgres refuses in text and
// jsonb), are skipped and counted.
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
		if err := json.Unmarshal(e, &p); err != nil || p.ID == "" || p.HostedURL == "" || hasNUL(e) {
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

// hasNUL reports whether JSON text contains a \u0000 escape that decodes to NUL: the
// backslash before "u0000" must itself be unescaped (an odd run of backslashes).
// Valid JSON can't hold a raw NUL byte, so the escape is the only form to look for.
func hasNUL(b []byte) bool {
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' {
			continue
		}
		j := i
		for j < len(b) && b[j] == '\\' {
			j++
		}
		if (j-i)%2 == 1 && bytes.HasPrefix(b[j:], []byte("u0000")) {
			return true
		}
		i = j - 1
	}
	return false
}
