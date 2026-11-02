package lever_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/source/fetch"
	"github.com/shinraxtensei/job-hunter/internal/source/lever"
)

// These tests cover Lever's URL, parsing and golden files. Retries, Retry-After, the
// limiter and cancellation live in fetch.Client and are tested there.

func client(baseURL string) *lever.Client { return lever.New(baseURL) }

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
	if skipped.Total() != 0 {
		t.Errorf("skipped %d postings, want 0", skipped.Total())
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
	if skipped.Total() != 1 {
		t.Errorf("skipped = %d, want 1", skipped.Total())
	}
	if len(got) != 2 || got[0].SourceID != "2a55d6c9-c564-443a-be3e-a3851db5a93b" ||
		got[1].SourceID != "785989a6-b219-43b1-92d9-750a67054f24" {
		t.Errorf("got %d records, want the two well-formed ones in order", len(got))
	}
}

// A 404 is an error that names the company (the adapter's wrapping) and never carries
// the response body.
func TestFetchErrorNamesCompany(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "secret upstream detail", http.StatusNotFound)
	}))
	defer srv.Close()
	_, _, err := client(srv.URL).Fetch(t.Context(), "acme", noLimit())
	if err == nil || !strings.Contains(err.Error(), "lever") || !strings.Contains(err.Error(), "acme") ||
		!strings.Contains(err.Error(), "404") {
		t.Fatalf("error = %v, want one naming lever, acme and 404", err)
	}
	if strings.Contains(err.Error(), "secret upstream detail") {
		t.Errorf("error leaks the response body: %v", err)
	}
}

// AC6: a posting with a NUL character (\u0000) is skipped and counted; the rest stay.
func TestFetchSkipsPostingWithNUL(t *testing.T) {
	srv, _ := serveFile(t, "nul_posting.json")
	got, skipped, err := client(srv.URL).Fetch(t.Context(), "spotify", noLimit())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if skipped.Total() != 1 || len(got) != 2 {
		t.Errorf("got %d records, %d skipped; want 2 records, 1 skipped", len(got), skipped.Total())
	}
	for _, r := range got {
		if r.SourceID == "00000000-nul0-4000-8000-000000000000" {
			t.Error("the posting containing NUL was returned")
		}
	}
	if len(skipped.IDs) != 1 || skipped.IDs[0] != "00000000-nul0-4000-8000-000000000000" {
		t.Errorf("skipped IDs = %v, want the NUL posting's ID (ADR-0010: it is still on the board)", skipped.IDs)
	}
}

// M2-07: FetchOne asks Lever's API for one posting, by site and id, never the pasted link.
func TestFetchOne(t *testing.T) {
	elems := rawElements(t, "spotify_postings.json")
	var one struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(elems[0], &one); err != nil {
		t.Fatal(err)
	}
	var path, mode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, mode = r.URL.Path, r.URL.Query().Get("mode")
		switch r.URL.Path {
		case "/v0/postings/spotify/" + one.ID:
			_, _ = w.Write(elems[0])
		case "/v0/postings/spotify/00000000-0000-4000-8000-000000000000":
			_, _ = w.Write(elems[1]) // a different posting than asked for
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := client(srv.URL)

	raw, err := c.FetchOne(t.Context(), "spotify", one.ID, noLimit())
	if err != nil || raw.Source != "lever" || raw.SourceID != one.ID || string(raw.Payload) != string(elems[0]) {
		t.Fatalf("FetchOne = %+v, %v", raw, err)
	}
	if path != "/v0/postings/spotify/"+one.ID || mode != "json" {
		t.Errorf("requested %s?mode=%s", path, mode)
	}
	if _, err := c.FetchOne(t.Context(), "spotify", "11111111-1111-4111-8111-111111111111", noLimit()); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("missing posting: err = %v, want ErrNotFound", err)
	}
	if _, err := c.FetchOne(t.Context(), "spotify", "00000000-0000-4000-8000-000000000000", noLimit()); err == nil {
		t.Error("a response for another posting was accepted")
	}
}
