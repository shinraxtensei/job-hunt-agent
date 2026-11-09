// Package fetch is the HTTP side every job-board adapter shares: one polite GET with
// retries, and the check for JSON Postgres can't store. Adapters build the URL and
// parse the body; the source-adapter skill's politeness rules live here once.
package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const (
	userAgent = "job-hunter/0.1 (+https://github.com/shinraxtensei/job-hunt-agent)"

	// maxBody caps how much of a response we read: the payload is untrusted. The
	// largest real board seen (palantir on Lever, 301 postings) is ~6 MiB.
	maxBody = 32 << 20

	// maxAttempts bounds retries of one Get (source-adapter skill: at most 3).
	maxAttempts = 3

	// maxRetryAfter is the longest Retry-After we sit out; longer means give up on this
	// board for this run rather than stall the whole run.
	maxRetryAfter = time.Minute
)

// Client makes polite GETs to one API host.
type Client struct {
	HTTP *http.Client
	// Backoff is the first retry delay; each retry doubles it, plus up to 50% jitter.
	Backoff time.Duration
	// Stall is the longest gap allowed between bytes of a response body; a body that
	// stops arriving fails then and is retried. A slow but steady body is fine (M1-10).
	Stall time.Duration
	// Ceiling bounds one attempt from request to last byte, so a pathological trickle
	// still ends. Reaching it is not retried.
	Ceiling time.Duration
}

// New returns a client that fails fast on a dead server (10s to connect, 10s for TLS,
// 20s for response headers) but lets a slow body finish: no total timeout, a 30s stall
// timeout and a 10-minute ceiling per attempt. The largest Lever board (~3.3 MB) needed
// ~290s at the 11.5 KB/s Lever sent on 2026-11-01. First retry delay: 1s.
func New() *Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 20 * time.Second
	return &Client{
		HTTP:    &http.Client{Transport: tr}, // no Timeout: it would cap the whole body
		Backoff: time.Second,
		Stall:   30 * time.Second,
		Ceiling: 10 * time.Minute,
	}
}

// Get returns the body of a 200 response to u.
//
// Every request, retries included, first waits on limit: the host's limiter, shared by
// every company on that host. It retries 429, 5xx, timeouts and network errors, at most
// maxAttempts times in all, with exponential backoff and jitter, honouring Retry-After
// (up to maxRetryAfter). Errors carry the status, never the response body. Callers add
// the board and company.
func (c *Client) Get(ctx context.Context, u string, limit *rate.Limiter) ([]byte, error) {
	var lastErr error
	for attempt := 1; ; attempt++ {
		if err := limit.Wait(ctx); err != nil {
			return nil, err
		}
		body, retryAfter, err := c.get(ctx, u)
		if err == nil {
			return body, nil
		}
		lastErr = err
		var perm permanent
		if errors.As(err, &perm) || ctx.Err() != nil || attempt == maxAttempts {
			return nil, lastErr
		}
		if retryAfter > maxRetryAfter {
			return nil, fmt.Errorf("%w (Retry-After %v is longer than %v; not waiting)", err, retryAfter, maxRetryAfter)
		}
		select {
		case <-time.After(max(retryAfter, c.backoff(attempt))):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// ErrNotFound is a 404: the board says the thing asked for doesn't exist (M2-07: a
// rechecked posting that is gone).
var ErrNotFound = errors.New("not found")

// permanent marks an error that retrying won't fix (4xx other than 429, an oversized
// body).
type permanent struct{ error }

func (p permanent) Unwrap() error { return p.error }

// get makes one request. It returns the body on 200; otherwise an error, and for 429
// or 5xx the server's Retry-After, if any. The attempt ends at the ceiling, or when the
// body stops arriving for Stall.
func (c *Client) get(ctx context.Context, u string) ([]byte, time.Duration, error) {
	attempt, cancel := context.WithTimeout(ctx, c.Ceiling)
	defer cancel()
	var stalled atomic.Bool

	req, err := http.NewRequestWithContext(attempt, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, permanent{err}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, c.classify(ctx, attempt, &stalled, err) // dial/TLS/header timeouts: retryable
	}
	defer func() { _ = resp.Body.Close() }() // read-only body; a close error loses nothing

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		// The body is untrusted and may be large; only the status goes in the error.
		return nil, retryAfter(resp.Header.Get("Retry-After")), fmt.Errorf("unexpected status %s", resp.Status)
	case resp.StatusCode == http.StatusNotFound:
		return nil, 0, permanent{fmt.Errorf("unexpected status %s: %w", resp.Status, ErrNotFound)}
	default:
		return nil, 0, permanent{fmt.Errorf("unexpected status %s", resp.Status)}
	}
	// The stall timer restarts on every read that returns bytes; if it fires, it cancels
	// the attempt, which aborts the read.
	timer := time.AfterFunc(c.Stall, func() { stalled.Store(true); cancel() })
	defer timer.Stop()
	body, err := io.ReadAll(&progress{r: io.LimitReader(resp.Body, maxBody+1), timer: timer, stall: c.Stall})
	if err != nil {
		return nil, 0, fmt.Errorf("reading body: %w", c.classify(ctx, attempt, &stalled, err))
	}
	if len(body) > maxBody {
		return nil, 0, permanent{fmt.Errorf("response larger than %d bytes", maxBody)}
	}
	return body, 0, nil
}

// classify names why an attempt ended early: the caller cancelled (returned as is), the
// body stalled (retryable), or the attempt reached the ceiling (permanent).
func (c *Client) classify(parent, attempt context.Context, stalled *atomic.Bool, err error) error {
	switch {
	case parent.Err() != nil:
		return err
	case stalled.Load():
		return fmt.Errorf("stalled: no data for %v", c.Stall)
	case errors.Is(attempt.Err(), context.DeadlineExceeded):
		return permanent{fmt.Errorf("reached the %v ceiling for one request", c.Ceiling)}
	default:
		return err
	}
}

// progress restarts the stall timer whenever a read returns bytes.
type progress struct {
	r     io.Reader
	timer *time.Timer
	stall time.Duration
}

func (p *progress) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.timer.Reset(p.stall)
	}
	return n, err
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

// HasNUL reports whether JSON text contains a \u0000 escape that decodes to NUL, which
// Postgres refuses in text and jsonb: the backslash before "u0000" must itself be
// unescaped (an odd run of backslashes). Valid JSON can't hold a raw NUL byte, so the
// escape is the only form to look for.
func HasNUL(b []byte) bool {
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
