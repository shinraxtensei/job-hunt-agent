package fetch_test

import (
	"context"
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
	if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "https://github.com/shinraxtensei/job-hunter") {
		t.Errorf("User-Agent %q has no contact URL", ua)
	}
	if r.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept = %q, want application/json", r.Header.Get("Accept"))
	}
}

func TestNewHasBoundedTimeout(t *testing.T) {
	if d := fetch.New().HTTP.Timeout; d <= 0 || d > time.Minute {
		t.Errorf("timeout = %v, want a bounded timeout", d)
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

func TestGetRetriesTimeout(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			select { // first request: slower than the client timeout
			case <-time.After(time.Second):
			case <-r.Context().Done():
			}
			return
		}
		_, _ = w.Write([]byte(okBody)) // a failed write shows up as a client error
	}))
	defer srv.Close()
	c := client()
	c.HTTP.Timeout = 100 * time.Millisecond
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
