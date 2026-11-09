package fetch_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/source/fetch"
)

// These tests cover what every adapter inherits through fetch.Client.Get: the
// source-adapter skill's politeness and retry rules. Adapter tests cover only their URL,
// parsing and golden files.

const okBody = `{"ok":true}`

// client is a fetch client with a 1ms retry backoff, so retry tests stay fast.
func client() *fetch.Client {
	c := fetch.New()
	c.Backoff = time.Millisecond
	return c
}

// noLimit is a limiter that never waits.
func noLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

// scripted answers request n with statuses[n] (the last repeats); 200 serves okBody.
// It records each request and when it arrived.
func scripted(t *testing.T, statuses []int, header http.Header) (*httptest.Server, func() []*http.Request, func() []time.Time) {
	t.Helper()
	var mu sync.Mutex
	var reqs []*http.Request
	var times []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, r)
		times = append(times, time.Now())
		code := statuses[min(len(reqs), len(statuses))-1]
		mu.Unlock()
		if code != http.StatusOK {
			for k, v := range header {
				w.Header()[k] = v
			}
			http.Error(w, "secret upstream detail", code)
			return
		}
		_, _ = w.Write([]byte(okBody)) // a failed write shows up as a client error
	}))
	t.Cleanup(srv.Close)
	return srv,
		func() []*http.Request { mu.Lock(); defer mu.Unlock(); return append([]*http.Request(nil), reqs...) },
		func() []time.Time { mu.Lock(); defer mu.Unlock(); return append([]time.Time(nil), times...) }
}

func TestGetReturnsBodyAndIdentifiesUs(t *testing.T) {
	srv, reqs, _ := scripted(t, []int{200}, nil)
	body, err := client().Get(t.Context(), srv.URL, noLimit())
	if err != nil || string(body) != okBody {
		t.Fatalf("Get = %q, %v; want %q", body, err, okBody)
	}
	r := reqs()[0]
	if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "https://github.com/shinraxtensei/job-hunt-agent") {
		t.Errorf("User-Agent %q has no contact URL", ua)
	}
	if r.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept = %q, want application/json", r.Header.Get("Accept"))
	}
}

// M1-10: no cap on the whole transfer; instead fast connect/TLS/header timeouts, a stall
// timeout for the body, and a generous per-attempt ceiling.
func TestNewTimeouts(t *testing.T) {
	c := fetch.New()
	if c.HTTP.Timeout != 0 {
		t.Errorf("http.Client.Timeout = %v, want 0: a total cap fails slow-but-progressing bodies", c.HTTP.Timeout)
	}
	tr := transport(t, c)
	for name, d := range map[string]time.Duration{
		"TLS handshake": tr.TLSHandshakeTimeout, "response header": tr.ResponseHeaderTimeout,
	} {
		if d <= 0 || d > 30*time.Second {
			t.Errorf("%s timeout = %v, want set and <= 30s (fail fast on a dead server)", name, d)
		}
	}
	if c.Stall <= 0 || c.Stall > time.Minute {
		t.Errorf("Stall = %v, want set and <= 1m", c.Stall)
	}
	if c.Ceiling < 5*time.Minute || c.Ceiling > 15*time.Minute {
		t.Errorf("Ceiling = %v, want generous (5-15m): the largest Lever board needs ~290s at 11.5 KB/s", c.Ceiling)
	}
}

// transport returns the client's *http.Transport, failing the test if it has none.
func transport(t *testing.T, c *fetch.Client) *http.Transport {
	t.Helper()
	tr, ok := c.HTTP.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport is %T, want *http.Transport", c.HTTP.Transport)
	}
	return tr
}

// paced serves a body of chunks bytes, one byte-chunk of size n every gap, flushing each
// one, so tests control exactly how fast the body arrives. stopAfter > 0 stops sending
// (but keeps the connection open) after that many chunks.
func paced(t *testing.T, chunks, n int, gap time.Duration, stopAfter int) (*httptest.Server, func() []time.Time) {
	t.Helper()
	var mu sync.Mutex
	var times []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher) // httptest's ResponseWriter always flushes
		chunk := []byte(strings.Repeat("x", n))
		for i := 0; chunks == 0 || i < chunks; i++ {
			if stopAfter > 0 && i == stopAfter {
				<-r.Context().Done() // stall: connection open, no more bytes
				return
			}
			if _, err := w.Write(chunk); err != nil {
				return // client went away
			}
			fl.Flush()
			select {
			case <-time.After(gap):
			case <-r.Context().Done():
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []time.Time { mu.Lock(); defer mu.Unlock(); return append([]time.Time(nil), times...) }
}

// M1-10 AC1: a body that arrives steadily but slowly completes, even though it takes
// longer than a total-transfer cap of the old kind would allow.
func TestGetCompletesSlowSteadyBody(t *testing.T) {
	srv, _ := paced(t, 30, 1024, 50*time.Millisecond, 0) // 30 KiB over ~1.5s
	c := client()
	c.Stall, c.Ceiling = 300*time.Millisecond, 10*time.Second
	body, err := c.Get(t.Context(), srv.URL, noLimit())
	if err != nil {
		t.Fatalf("Get: %v (a steady body must complete)", err)
	}
	if len(body) != 30*1024 {
		t.Errorf("got %d bytes, want %d", len(body), 30*1024)
	}

	old := client()
	old.HTTP.Timeout = time.Second // what a total cap does to the same body
	if _, err := old.Get(t.Context(), srv.URL, noLimit()); err == nil {
		t.Error("sanity: a 1s total cap should have failed this 1.5s body")
	}
}

// M1-10 AC2 + AC5: a body that stops arriving fails after the stall timeout, is retried
// (3 attempts in all), and every attempt waits on the limiter.
func TestGetFailsWhenBodyStalls(t *testing.T) {
	srv, times := paced(t, 100, 1024, 10*time.Millisecond, 5) // 5 KiB, then silence
	c := client()
	c.Stall, c.Ceiling = 200*time.Millisecond, 10*time.Second
	const every = 300 * time.Millisecond
	start := time.Now()
	_, err := c.Get(t.Context(), srv.URL, rate.NewLimiter(rate.Every(every), 1))
	if err == nil || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("error = %v, want one saying the body stalled", err)
	}
	ts := times()
	if len(ts) != 3 {
		t.Errorf("made %d attempts, want 3 (a stall is retried)", len(ts))
	}
	for i := 1; i < len(ts); i++ {
		if gap := ts[i].Sub(ts[i-1]); gap < every-10*time.Millisecond {
			t.Errorf("attempts %d and %d were %v apart, want >= %v (limiter)", i-1, i, gap, every)
		}
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v, want each attempt to end at the 200ms stall timeout", d)
	}
}

// M1-10 AC3: a server that never sends headers fails each attempt at the header timeout,
// long before the stall timeout or the ceiling.
func TestGetFailsFastWhenServerNeverAnswers(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		n.Add(1)
		<-r.Context().Done() // never answer
	}))
	defer srv.Close()
	c := client()
	c.Stall, c.Ceiling = 5*time.Second, time.Minute
	transport(t, c).ResponseHeaderTimeout = 100 * time.Millisecond
	start := time.Now()
	if _, err := c.Get(t.Context(), srv.URL, noLimit()); err == nil {
		t.Fatal("Get returned no error for a server that never answers")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v for %d attempts, want each to fail at the 100ms header timeout", d, n.Load())
	}
	if n.Load() != 3 {
		t.Errorf("made %d attempts, want 3", n.Load())
	}
}

// M1-10 AC4: a body that keeps trickling (never stalling) ends at the ceiling, and that
// is not retried: a second attempt would trickle the same way.
func TestGetCeilingEndsPathologicalTrickle(t *testing.T) {
	srv, times := paced(t, 0, 1, 20*time.Millisecond, 0) // 1 byte every 20ms, forever
	c := client()
	c.Stall, c.Ceiling = 500*time.Millisecond, 300*time.Millisecond
	start := time.Now()
	_, err := c.Get(t.Context(), srv.URL, noLimit())
	if err == nil || !strings.Contains(err.Error(), "ceiling") {
		t.Fatalf("error = %v, want one saying the ceiling was reached", err)
	}
	if d := time.Since(start); d < 250*time.Millisecond || d > 1500*time.Millisecond {
		t.Errorf("took %v, want about the 300ms ceiling", d)
	}
	if n := len(times()); n != 1 {
		t.Errorf("made %d attempts, want 1 (hitting the ceiling is not retried)", n)
	}
}

// Retry 5xx (idempotent GET), at most 3 attempts in all.
func TestGetRetriesServerErrorsThenSucceeds(t *testing.T) {
	srv, reqs, _ := scripted(t, []int{500, 503, 200}, nil)
	if _, err := client().Get(t.Context(), srv.URL, noLimit()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n := len(reqs()); n != 3 {
		t.Errorf("made %d requests, want 3", n)
	}
}

func TestGetGivesUpAfterThreeAttempts(t *testing.T) {
	srv, reqs, _ := scripted(t, []int{500}, nil)
	_, err := client().Get(t.Context(), srv.URL, noLimit())
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %v, want one naming 500", err)
	}
	if strings.Contains(err.Error(), "secret upstream detail") {
		t.Errorf("error leaks the response body: %v", err)
	}
	if n := len(reqs()); n != 3 {
		t.Errorf("made %d requests, want exactly 3", n)
	}
}

// 4xx other than 429 is permanent: one request, no retry.
func TestGetDoesNotRetryClientErrors(t *testing.T) {
	srv, reqs, _ := scripted(t, []int{404}, nil)
	_, err := client().Get(t.Context(), srv.URL, noLimit())
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("error = %v, want one naming 404", err)
	}
	if n := len(reqs()); n != 1 {
		t.Errorf("made %d requests for a 404, want 1", n)
	}
}

// A 429 with Retry-After is retried after that delay.
func TestGetHonoursRetryAfter(t *testing.T) {
	srv, _, times := scripted(t, []int{429, 200}, http.Header{"Retry-After": {"1"}})
	if _, err := client().Get(t.Context(), srv.URL, noLimit()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	ts := times()
	if len(ts) != 2 || ts[1].Sub(ts[0]) < time.Second {
		t.Errorf("requests at %v; want 2, the retry >= 1s after a Retry-After: 1", ts)
	}
}

func TestGetHonoursRetryAfterDate(t *testing.T) {
	when := time.Now().Add(1500 * time.Millisecond).UTC().Format(http.TimeFormat)
	srv, _, times := scripted(t, []int{503, 200}, http.Header{"Retry-After": {when}})
	if _, err := client().Get(t.Context(), srv.URL, noLimit()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	// HTTP dates have 1s resolution, so the wait is somewhere in 0.5-1.5s.
	if ts := times(); len(ts) != 2 || ts[1].Sub(ts[0]) < 400*time.Millisecond {
		t.Errorf("retried too soon after a Retry-After date: %v", ts)
	}
}

// A Retry-After beyond what a run can afford fails fast instead of stalling the run.
func TestGetRetryAfterTooLongFailsFast(t *testing.T) {
	srv, reqs, _ := scripted(t, []int{429}, http.Header{"Retry-After": {"3600"}})
	start := time.Now()
	_, err := client().Get(t.Context(), srv.URL, noLimit())
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("error = %v, want one naming 429", err)
	}
	if d := time.Since(start); d > time.Second || len(reqs()) != 1 {
		t.Errorf("took %v and %d requests, want an immediate give-up after 1", d, len(reqs()))
	}
}

// A server that misses the header timeout once and then answers is retried.
func TestGetRetriesAfterHeaderTimeout(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			select { // first request: never sends headers within the timeout
			case <-time.After(time.Second):
			case <-r.Context().Done():
			}
			return
		}
		_, _ = w.Write([]byte(okBody)) // a failed write shows up as a client error
	}))
	defer srv.Close()
	c := client()
	transport(t, c).ResponseHeaderTimeout = 100 * time.Millisecond
	if body, err := c.Get(t.Context(), srv.URL, noLimit()); err != nil || string(body) != okBody {
		t.Fatalf("Get = %q, %v; want the body on the second attempt", body, err)
	}
	if n.Load() != 2 {
		t.Errorf("made %d requests, want 2", n.Load())
	}
}

// Every attempt, retries included, waits on the host's limiter. With a 1ms backoff,
// only the limiter can space requests 200ms apart.
func TestGetWaitsOnLimiterForEveryAttempt(t *testing.T) {
	srv, _, times := scripted(t, []int{500, 500, 200}, nil)
	const every = 200 * time.Millisecond
	if _, err := client().Get(t.Context(), srv.URL, rate.NewLimiter(rate.Every(every), 1)); err != nil {
		t.Fatalf("Get: %v", err)
	}
	ts := times()
	for i := 1; i < len(ts); i++ {
		if gap := ts[i].Sub(ts[i-1]); gap < every-10*time.Millisecond {
			t.Errorf("attempts %d and %d were %v apart, want >= %v (limiter)", i-1, i, gap, every)
		}
	}
}

func TestGetStopsOnCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // hang until the client gives up
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(t.Context())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	start := time.Now()
	if _, err := client().Get(ctx, srv.URL, noLimit()); err == nil {
		t.Fatal("Get returned no error after cancellation")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Get took %v after cancellation, want prompt", d)
	}
}

func TestGetStopsDuringBackoffOnCancel(t *testing.T) {
	srv, _, _ := scripted(t, []int{500}, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := fetch.New().Get(ctx, srv.URL, noLimit()); err == nil { // default backoff: seconds
		t.Fatal("Get returned no error after cancellation")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Get took %v, want to stop when the context ends, not finish its backoff", d)
	}
}

func TestHasNUL(t *testing.T) {
	for in, want := range map[string]bool{
		`{"a":"x\u0000y"}`:   true,  // a real NUL
		`{"a":"\\u0000"}`:    false, // escaped backslash, then the text u0000
		`{"a":"\\\u0000"}`:   true,  // escaped backslash, then a real NUL
		`{"a":"\u00000"}`:    true,  // NUL followed by "0"
		`{"a":"\u0001"}`:     false, // another control character: Postgres accepts it
		`{"a":"u0000"}`:      false, // no backslash at all
		`{"a":"plain text"}`: false,
	} {
		if got := fetch.HasNUL([]byte(in)); got != want {
			t.Errorf("HasNUL(%s) = %v, want %v", in, got, want)
		}
	}
}

// M2-07: a 404 is ErrNotFound, so a recheck can tell "gone" from "failed".
func TestGet404IsNotFound(t *testing.T) {
	for code, want := range map[int]bool{404: true, 410: false, 403: false} {
		srv, _, _ := scripted(t, []int{code}, nil)
		_, err := client().Get(t.Context(), srv.URL, noLimit())
		if got := errors.Is(err, fetch.ErrNotFound); got != want {
			t.Errorf("%d: errors.Is(err, ErrNotFound) = %v, want %v (err %v)", code, got, want, err)
		}
	}
}

// M8-04: a response over the 32 MiB cap is refused, not read into memory, and not retried
// (the payload is untrusted; a board can't make the collector hold an unbounded body).
func TestGetRefusesOversizedBody(t *testing.T) {
	var hits atomic.Int32
	chunk := strings.Repeat("x", 1<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		for range 33 {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return // the client stopped reading
			}
		}
	}))
	t.Cleanup(srv.Close)
	_, err := client().Get(t.Context(), srv.URL, noLimit())
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v, want the size cap", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d requests, want 1 (a size refusal is permanent)", n)
	}
}
