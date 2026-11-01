package lever_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/source/lever"
)

// client is a Lever client with a 1ms retry backoff, so retry tests stay fast.
func client(baseURL string) *lever.Client {
	c := lever.New(baseURL)
	c.Backoff = time.Millisecond
	return c
}

// noLimit is a limiter that never waits.
func noLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

// serveFile serves testdata/<name> as the postings list for the "spotify" site and
// records the last request so tests can inspect it.
func serveFile(t *testing.T, name string) (*httptest.Server, *http.Request) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	var last http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = *r
		if r.URL.Path != "/v0/postings/spotify" || r.URL.Query().Get("mode") != "json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body) // a failed write shows up as a test failure on the client side
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

// rawElements returns each element of the JSON array in testdata/<name>, byte for byte.
func rawElements(t *testing.T, name string) []json.RawMessage {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(body, &elems); err != nil {
		t.Fatalf("decoding %s: %v", name, err)
	}
	return elems
}

func equalRaw(t *testing.T, got, want []job.Raw) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Source != w.Source || g.SourceID != w.SourceID || g.URL != w.URL || g.Title != w.Title {
			t.Errorf("record %d metadata = %+v, want %+v", i,
				[]string{g.Source, g.SourceID, g.URL, g.Title}, []string{w.Source, w.SourceID, w.URL, w.Title})
		}
		if string(g.Payload) != string(w.Payload) {
			t.Errorf("record %d payload is not the posting exactly as received", i)
		}
	}
}

// AC3: a recorded Lever response produces exactly the expected raw records, with each
// posting's JSON kept byte for byte.
func TestFetchGoldenFile(t *testing.T) {
	srv, _ := serveFile(t, "spotify_postings.json")
	got, skipped, err := client(srv.URL).Fetch(t.Context(), "spotify", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped != 0 {
		t.Errorf("skipped %d postings, want 0", skipped)
	}
	elems := rawElements(t, "spotify_postings.json")
	equalRaw(t, got, []job.Raw{
		{
			Source:   "lever",
			SourceID: "2a55d6c9-c564-443a-be3e-a3851db5a93b",
			URL:      "https://jobs.lever.co/spotify/2a55d6c9-c564-443a-be3e-a3851db5a93b",
			Title:    "Account Executive - Commercial, Spotify Technology",
			Payload:  elems[0],
		},
		{
			Source:   "lever",
			SourceID: "785989a6-b219-43b1-92d9-750a67054f24",
			URL:      "https://jobs.lever.co/spotify/785989a6-b219-43b1-92d9-750a67054f24",
			Title:    "AV Engineer - Platform",
			Payload:  elems[1],
		},
	})
}

func TestFetchEmptyBoard(t *testing.T) {
	srv, _ := serveFile(t, "empty.json")
	got, _, err := client(srv.URL).Fetch(t.Context(), "spotify", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got %#v, want an empty, non-nil slice", got)
	}
}

func TestFetchSkipsMalformedPosting(t *testing.T) {
	srv, _ := serveFile(t, "malformed_posting.json")
	got, skipped, err := client(srv.URL).Fetch(t.Context(), "spotify", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if len(got) != 2 || got[0].SourceID != "2a55d6c9-c564-443a-be3e-a3851db5a93b" ||
		got[1].SourceID != "785989a6-b219-43b1-92d9-750a67054f24" {
		t.Errorf("got %d records, want the two well-formed ones in order", len(got))
	}
}

// AC4 (adapter half): a non-200 status is an error that names the company and never
// carries the response body.
func TestFetchHTTPErrorNamesCompany(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "secret upstream detail", status)
		}))
		_, _, err := client(srv.URL).Fetch(t.Context(), "acme", noLimit())
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: Fetch returned no error", status)
		}
		if !strings.Contains(err.Error(), "acme") || !strings.Contains(err.Error(), http.StatusText(status)) {
			t.Errorf("status %d: error %q should name the company and the status", status, err)
		}
		if strings.Contains(err.Error(), "secret upstream detail") {
			t.Errorf("status %d: error leaks the response body: %q", status, err)
		}
	}
}

func TestFetchSendsUserAgentWithContactURL(t *testing.T) {
	srv, last := serveFile(t, "empty.json")
	if _, _, err := client(srv.URL).Fetch(t.Context(), "spotify", noLimit()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if ua := last.Header.Get("User-Agent"); !strings.Contains(ua, "https://github.com/shinraxtensei/job-hunter") {
		t.Errorf("User-Agent %q has no contact URL", ua)
	}
}

func TestFetchStopsOnCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // hang until the client gives up
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	if _, _, err := client(srv.URL).Fetch(ctx, "spotify", noLimit()); err == nil {
		t.Fatal("Fetch with a cancelled context returned no error")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Fetch took %v after cancellation, want prompt return", d)
	}
}

func TestClientHasTimeout(t *testing.T) {
	if got := lever.New("http://unused").HTTP.Timeout; got <= 0 || got > time.Minute {
		t.Errorf("client timeout = %v, want a bounded timeout", got)
	}
}

// scripted serves one response per request from statuses (the last repeats) and
// records when each request arrived. 200 serves the golden file.
func scripted(t *testing.T, statuses []int, header http.Header) (*httptest.Server, func() []time.Time) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "spotify_postings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var times []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		code := statuses[min(len(times), len(statuses))-1]
		mu.Unlock()
		if code != http.StatusOK {
			for k, v := range header {
				w.Header()[k] = v
			}
			http.Error(w, "nope", code)
			return
		}
		_, _ = w.Write(body) // a failed write shows up as a client error
	}))
	t.Cleanup(srv.Close)
	return srv, func() []time.Time {
		mu.Lock()
		defer mu.Unlock()
		return append([]time.Time(nil), times...)
	}
}

// Skill: retry 5xx (idempotent GET), at most 3 attempts.
func TestFetchRetriesServerErrorsThenSucceeds(t *testing.T) {
	srv, times := scripted(t, []int{500, 503, 200}, nil)
	got, _, err := client(srv.URL).Fetch(t.Context(), "spotify", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 2 || len(times()) != 3 {
		t.Errorf("got %d records after %d requests, want 2 after 3", len(got), len(times()))
	}
}

func TestFetchGivesUpAfterThreeAttempts(t *testing.T) {
	srv, times := scripted(t, []int{500}, nil)
	_, _, err := client(srv.URL).Fetch(t.Context(), "acme", noLimit())
	if err == nil || !strings.Contains(err.Error(), "acme") || !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %v, want one naming acme and 500", err)
	}
	if n := len(times()); n != 3 {
		t.Errorf("made %d requests, want exactly 3", n)
	}
}

func TestFetchDoesNotRetryClientErrors(t *testing.T) {
	srv, times := scripted(t, []int{404}, nil)
	if _, _, err := client(srv.URL).Fetch(t.Context(), "acme", noLimit()); err == nil {
		t.Fatal("Fetch returned no error for a 404")
	}
	if n := len(times()); n != 1 {
		t.Errorf("made %d requests for a 404, want 1 (not retryable)", n)
	}
}

// AC3: a 429 with Retry-After is retried after that delay.
func TestFetchHonoursRetryAfter(t *testing.T) {
	srv, times := scripted(t, []int{429, 200}, http.Header{"Retry-After": {"1"}})
	got, _, err := client(srv.URL).Fetch(t.Context(), "spotify", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	ts := times()
	if len(ts) != 2 || len(got) != 2 {
		t.Fatalf("got %d records after %d requests, want 2 after 2", len(got), len(ts))
	}
	if gap := ts[1].Sub(ts[0]); gap < time.Second {
		t.Errorf("retried %v after a 429 with Retry-After: 1, want >= 1s", gap)
	}
}

func TestFetchHonoursRetryAfterDate(t *testing.T) {
	when := time.Now().Add(1500 * time.Millisecond).UTC().Format(http.TimeFormat)
	srv, times := scripted(t, []int{503, 200}, http.Header{"Retry-After": {when}})
	if _, _, err := client(srv.URL).Fetch(t.Context(), "spotify", noLimit()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// HTTP dates have 1s resolution, so the wait is somewhere in 0.5-1.5s.
	if ts := times(); len(ts) != 2 || ts[1].Sub(ts[0]) < 400*time.Millisecond {
		t.Errorf("retried too soon after a Retry-After date: %v", ts)
	}
}

// A Retry-After beyond what a run can afford fails fast instead of stalling the run.
func TestFetchRetryAfterTooLongFailsFast(t *testing.T) {
	srv, times := scripted(t, []int{429}, http.Header{"Retry-After": {"3600"}})
	start := time.Now()
	_, _, err := client(srv.URL).Fetch(t.Context(), "acme", noLimit())
	if err == nil || !strings.Contains(err.Error(), "acme") || !strings.Contains(err.Error(), "429") {
		t.Errorf("error = %v, want one naming acme and 429", err)
	}
	if d := time.Since(start); d > time.Second || len(times()) != 1 {
		t.Errorf("took %v and %d requests, want an immediate give-up after 1", d, len(times()))
	}
}

func TestFetchRetriesTimeout(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "spotify_postings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			select { // first request: slower than the client timeout
			case <-time.After(time.Second):
			case <-r.Context().Done():
			}
			return
		}
		_, _ = w.Write(body) // a failed write shows up as a client error
	}))
	defer srv.Close()
	c := client(srv.URL)
	c.HTTP.Timeout = 100 * time.Millisecond
	got, _, err := c.Fetch(t.Context(), "spotify", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 2 || n.Load() != 2 {
		t.Errorf("got %d records after %d requests, want 2 after 2", len(got), n.Load())
	}
}

// Every attempt, retries included, waits on the host's limiter (robots.txt:
// Crawl-delay: 1). With a 1ms backoff, only the limiter can space requests 200ms apart.
func TestFetchWaitsOnLimiterForEveryAttempt(t *testing.T) {
	srv, times := scripted(t, []int{500, 500, 200}, nil)
	const every = 200 * time.Millisecond
	if _, _, err := client(srv.URL).Fetch(t.Context(), "spotify", rate.NewLimiter(rate.Every(every), 1)); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	ts := times()
	for i := 1; i < len(ts); i++ {
		if gap := ts[i].Sub(ts[i-1]); gap < every-10*time.Millisecond {
			t.Errorf("attempts %d and %d were %v apart, want >= %v (limiter)", i-1, i, gap, every)
		}
	}
}

func TestFetchStopsDuringBackoffOnCancel(t *testing.T) {
	srv, _ := scripted(t, []int{500}, nil)
	c := lever.New(srv.URL) // default backoff: seconds
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := c.Fetch(ctx, "spotify", noLimit()); err == nil {
		t.Fatal("Fetch returned no error after cancellation")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Fetch took %v, want to stop when the context ends, not finish its backoff", d)
	}
}

// AC6: a posting with a NUL character (\u0000) is skipped and counted; the rest stay.
func TestFetchSkipsPostingWithNUL(t *testing.T) {
	srv, _ := serveFile(t, "nul_posting.json")
	got, skipped, err := client(srv.URL).Fetch(t.Context(), "spotify", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped != 1 || len(got) != 2 {
		t.Errorf("got %d records, %d skipped; want 2 records, 1 skipped", len(got), skipped)
	}
	for _, r := range got {
		if r.SourceID == "00000000-nul0-4000-8000-000000000000" {
			t.Error("the posting containing NUL was returned")
		}
	}
}
