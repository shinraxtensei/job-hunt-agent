package greenhouse_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
	"github.com/shinraxtensei/job-hunt-agent/internal/source/fetch"
	"github.com/shinraxtensei/job-hunt-agent/internal/source/greenhouse"
)

// These tests cover Greenhouse's URL, parsing and golden files. Retries, Retry-After,
// the limiter and cancellation live in fetch.Client and are tested there.

func client(baseURL string) *greenhouse.Client { return greenhouse.New(baseURL) }

// noLimit is a limiter that never waits.
func noLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return b
}

// server answers each request with the next status from statuses (the last repeats);
// 200 serves body. It records each request.
func server(t *testing.T, body []byte, statuses []int, header http.Header) (*httptest.Server, func() []*http.Request, func() []time.Time) {
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
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body) // a failed write shows up as a client error
	}))
	t.Cleanup(srv.Close)
	return srv,
		func() []*http.Request { mu.Lock(); defer mu.Unlock(); return append([]*http.Request(nil), reqs...) },
		func() []time.Time { mu.Lock(); defer mu.Unlock(); return append([]time.Time(nil), times...) }
}

func ok(t *testing.T, name string) (*httptest.Server, func() []*http.Request) {
	t.Helper()
	srv, reqs, _ := server(t, fixture(t, name), []int{200}, nil)
	return srv, reqs
}

// jobsOf returns each element of the fixture's "jobs" array, byte for byte.
func jobsOf(t *testing.T, name string) []json.RawMessage {
	t.Helper()
	var doc struct {
		Jobs []json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(fixture(t, name), &doc); err != nil {
		t.Fatalf("decoding %s: %v", name, err)
	}
	return doc.Jobs
}

// AC2: a recorded Greenhouse response produces exactly the expected raw records, each
// job's JSON kept byte for byte, its numeric id as the source ID.
func TestFetchGoldenFile(t *testing.T) {
	srv, _ := ok(t, "cockroachlabs_jobs.json")
	got, skipped, err := client(srv.URL).Fetch(t.Context(), "cockroachlabs", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped.Total() != 0 {
		t.Errorf("skipped %d, want 0", skipped.Total())
	}
	jobs := jobsOf(t, "cockroachlabs_jobs.json")
	want := []job.Raw{
		{Source: "greenhouse", SourceID: "8079608", Title: "Deal Desk Manager, AMER",
			URL: "https://www.cockroachlabs.com/careers/job/?gh_jid=8079608", Payload: jobs[0]},
		{Source: "greenhouse", SourceID: "7422578", Title: "Member of Technical Staff (Senior - Specialized Indexing)",
			URL: "https://www.cockroachlabs.com/careers/job/?gh_jid=7422578", Payload: jobs[1]},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Source != w.Source || g.SourceID != w.SourceID || g.URL != w.URL || g.Title != w.Title {
			t.Errorf("record %d = %q %q %q %q, want %q %q %q %q", i,
				g.Source, g.SourceID, g.URL, g.Title, w.Source, w.SourceID, w.URL, w.Title)
		}
		if string(g.Payload) != string(w.Payload) {
			t.Errorf("record %d payload is not the job exactly as received", i)
		}
	}
}

// The request asks for the full board with descriptions, and identifies us.
func TestFetchRequest(t *testing.T) {
	srv, reqs := ok(t, "empty.json")
	if _, _, err := client(srv.URL).Fetch(t.Context(), "cockroachlabs", noLimit()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	r := reqs()[0]
	if r.URL.Path != "/v1/boards/cockroachlabs/jobs" || r.URL.Query().Get("content") != "true" {
		t.Errorf("requested %s, want /v1/boards/cockroachlabs/jobs?content=true", r.URL)
	}
	if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "https://github.com/shinraxtensei/job-hunt-agent") {
		t.Errorf("User-Agent %q has no contact URL", ua)
	}
}

func TestNameAndHost(t *testing.T) {
	c := greenhouse.New(greenhouse.BaseURL)
	if c.Name() != "greenhouse" || c.Host() != "boards-api.greenhouse.io" {
		t.Errorf("Name, Host = %q, %q; want greenhouse, boards-api.greenhouse.io", c.Name(), c.Host())
	}
}

// AC3: empty board → empty slice, not an error.
func TestFetchEmptyBoard(t *testing.T) {
	srv, _ := ok(t, "empty.json")
	got, _, err := client(srv.URL).Fetch(t.Context(), "cockroachlabs", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got %#v, want an empty, non-nil slice", got)
	}
}

// AC3: malformed jobs (no id, no URL, non-numeric id) are skipped; the rest returned.
func TestFetchSkipsMalformedPosting(t *testing.T) {
	srv, _ := ok(t, "malformed_posting.json")
	got, skipped, err := client(srv.URL).Fetch(t.Context(), "cockroachlabs", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped.Total() != 2 || len(got) != 2 || got[0].SourceID != "8079608" || got[1].SourceID != "7422578" {
		t.Errorf("got %d records (%d skipped), want the two well-formed ones and 2 skipped", len(got), skipped.Total())
	}
}

// Like Lever (M1-03 AC6): a job containing NUL is skipped and counted.
func TestFetchSkipsPostingWithNUL(t *testing.T) {
	srv, _ := ok(t, "nul_posting.json")
	got, skipped, err := client(srv.URL).Fetch(t.Context(), "cockroachlabs", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped.Total() != 1 || len(got) != 2 {
		t.Errorf("got %d records, %d skipped; want 2 and 1", len(got), skipped.Total())
	}
	if len(skipped.IDs) != 1 || skipped.IDs[0] != "1" {
		t.Errorf("skipped IDs = %v, want the NUL posting's ID (ADR-0010: it is still on the board)", skipped.IDs)
	}
}

// A 200 without a jobs array means the API changed: an error, not an empty board.
func TestFetchRejectsResponseWithoutJobs(t *testing.T) {
	srv, _, _ := server(t, []byte(`{"error":"Board not found"}`), []int{200}, nil)
	if _, _, err := client(srv.URL).Fetch(t.Context(), "acme", noLimit()); err == nil || !strings.Contains(err.Error(), "acme") {
		t.Errorf("error = %v, want one naming acme", err)
	}
}

// AC3: a 404 is an error naming the board (the adapter's wrapping), without the body.
func TestFetchErrorNamesCompany(t *testing.T) {
	srv, _, _ := server(t, nil, []int{http.StatusNotFound}, nil)
	_, _, err := client(srv.URL).Fetch(t.Context(), "acme", noLimit())
	if err == nil || !strings.Contains(err.Error(), "greenhouse") || !strings.Contains(err.Error(), "acme") ||
		!strings.Contains(err.Error(), "404") {
		t.Fatalf("error = %v, want one naming greenhouse, acme and 404", err)
	}
	if strings.Contains(err.Error(), "secret upstream detail") {
		t.Errorf("error leaks the response body: %v", err)
	}
}

// M2-07: FetchOne asks Greenhouse's API for one job, by board and id.
func TestFetchOne(t *testing.T) {
	jobs := jobsOf(t, "cockroachlabs_jobs.json")
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if r.URL.Path == "/v1/boards/cockroachlabs/jobs/8079608" {
			_, _ = w.Write(jobs[0])
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	raw, err := client(srv.URL).FetchOne(t.Context(), "cockroachlabs", "8079608", noLimit())
	if err != nil || raw.Source != "greenhouse" || raw.SourceID != "8079608" || string(raw.Payload) != string(jobs[0]) {
		t.Fatalf("FetchOne = %+v, %v", raw, err)
	}
	if path != "/v1/boards/cockroachlabs/jobs/8079608" {
		t.Errorf("requested %s", path)
	}
	if _, err := client(srv.URL).FetchOne(t.Context(), "cockroachlabs", "1", noLimit()); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("missing job: err = %v, want ErrNotFound", err)
	}
}
