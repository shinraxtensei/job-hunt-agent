package ashby_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/source/ashby"
)

// These tests cover Ashby's URL, parsing and golden files. Retries, Retry-After, the
// limiter and cancellation live in fetch.Client and are tested there.

func noLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return b
}

// serve answers every request with status and body, and records the requests.
func serve(t *testing.T, status int, body []byte) (*httptest.Server, func() []*http.Request) {
	t.Helper()
	var mu sync.Mutex
	var reqs []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, r)
		mu.Unlock()
		if status != http.StatusOK {
			http.Error(w, "secret upstream detail", status)
			return
		}
		_, _ = w.Write(body) // a failed write shows up as a client error
	}))
	t.Cleanup(srv.Close)
	return srv, func() []*http.Request { mu.Lock(); defer mu.Unlock(); return append([]*http.Request(nil), reqs...) }
}

func fetch(t *testing.T, name string) ([]job.Raw, int, error) {
	t.Helper()
	srv, _ := serve(t, http.StatusOK, fixture(t, name))
	return ashby.New(srv.URL).Fetch(t.Context(), "linear", noLimit())
}

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

// AC2: a recorded Ashby response produces exactly the expected raw records, each job's
// JSON (description and compensation included) kept byte for byte.
func TestFetchGoldenFile(t *testing.T) {
	got, skipped, err := fetch(t, "linear_jobs.json")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped != 0 {
		t.Errorf("skipped %d, want 0", skipped)
	}
	jobs := jobsOf(t, "linear_jobs.json")
	want := []job.Raw{
		{Source: "ashby", SourceID: "d3bc1ced-3ce4-4086-a050-555055dbb1ff", Title: "Senior / Staff Fullstack Engineer",
			URL: "https://jobs.ashbyhq.com/linear/d3bc1ced-3ce4-4086-a050-555055dbb1ff", Payload: jobs[0]},
		{Source: "ashby", SourceID: "b7669c4b-eeca-421d-ba9a-d90203f6fcb2", Title: "Product Manager",
			URL: "https://jobs.ashbyhq.com/linear/b7669c4b-eeca-421d-ba9a-d90203f6fcb2", Payload: jobs[1]},
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

// The request names the board and asks for compensation.
func TestFetchRequest(t *testing.T) {
	srv, reqs := serve(t, http.StatusOK, fixture(t, "empty.json"))
	if _, _, err := ashby.New(srv.URL).Fetch(t.Context(), "linear", noLimit()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	r := reqs()[0]
	if r.URL.Path != "/posting-api/job-board/linear" || r.URL.Query().Get("includeCompensation") != "true" {
		t.Errorf("requested %s, want /posting-api/job-board/linear?includeCompensation=true", r.URL)
	}
}

func TestNameAndHost(t *testing.T) {
	c := ashby.New(ashby.BaseURL)
	if c.Name() != "ashby" || c.Host() != "api.ashbyhq.com" {
		t.Errorf("Name, Host = %q, %q; want ashby, api.ashbyhq.com", c.Name(), c.Host())
	}
}

// AC3: empty board → empty slice, not an error.
func TestFetchEmptyBoard(t *testing.T) {
	got, _, err := fetch(t, "empty.json")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got %#v, want an empty, non-nil slice", got)
	}
}

// AC3: malformed jobs (no id, no jobUrl) are skipped and counted; the rest returned.
func TestFetchSkipsMalformedPosting(t *testing.T) {
	got, skipped, err := fetch(t, "malformed_posting.json")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped != 2 || len(got) != 2 || got[0].Title != "Senior / Staff Fullstack Engineer" || got[1].Title != "Product Manager" {
		t.Errorf("got %d records (%d skipped), want the two well-formed ones and 2 skipped", len(got), skipped)
	}
}

// Unlisted jobs are meant only for people with the direct link: left out, and not
// counted as malformed.
func TestFetchLeavesOutUnlistedJobs(t *testing.T) {
	got, skipped, err := fetch(t, "unlisted_posting.json")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 2 || skipped != 0 {
		t.Errorf("got %d records, %d skipped; want 2 listed jobs and 0 skipped", len(got), skipped)
	}
	for _, r := range got {
		if strings.HasSuffix(r.SourceID, "unlst") {
			t.Error("the unlisted job was returned")
		}
	}
}

// Like Lever (M1-03 AC6): a job containing NUL is skipped and counted.
func TestFetchSkipsPostingWithNUL(t *testing.T) {
	got, skipped, err := fetch(t, "nul_posting.json")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped != 1 || len(got) != 2 {
		t.Errorf("got %d records, %d skipped; want 2 and 1", len(got), skipped)
	}
}

// A 200 without a jobs array means the API changed: an error, not an empty board.
func TestFetchRejectsResponseWithoutJobs(t *testing.T) {
	srv, _ := serve(t, http.StatusOK, []byte(`{"apiVersion":"1"}`))
	if _, _, err := ashby.New(srv.URL).Fetch(t.Context(), "acme", noLimit()); err == nil || !strings.Contains(err.Error(), "acme") {
		t.Errorf("error = %v, want one naming acme", err)
	}
}

// AC3: a 404 is an error naming the board (the adapter's wrapping), without the body.
func TestFetchErrorNamesCompany(t *testing.T) {
	srv, _ := serve(t, http.StatusNotFound, nil)
	_, _, err := ashby.New(srv.URL).Fetch(t.Context(), "acme", noLimit())
	if err == nil || !strings.Contains(err.Error(), "ashby") || !strings.Contains(err.Error(), "acme") ||
		!strings.Contains(err.Error(), "404") {
		t.Fatalf("error = %v, want one naming ashby, acme and 404", err)
	}
	if strings.Contains(err.Error(), "secret upstream detail") {
		t.Errorf("error leaks the response body: %v", err)
	}
}
