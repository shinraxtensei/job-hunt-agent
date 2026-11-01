package lever_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/source/lever"
)

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
	got, skipped, err := lever.New(srv.URL).Fetch(t.Context(), "spotify")
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
	got, _, err := lever.New(srv.URL).Fetch(t.Context(), "spotify")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got %#v, want an empty, non-nil slice", got)
	}
}

func TestFetchSkipsMalformedPosting(t *testing.T) {
	srv, _ := serveFile(t, "malformed_posting.json")
	got, skipped, err := lever.New(srv.URL).Fetch(t.Context(), "spotify")
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
		_, _, err := lever.New(srv.URL).Fetch(t.Context(), "acme")
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
	if _, _, err := lever.New(srv.URL).Fetch(t.Context(), "spotify"); err != nil {
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
	if _, _, err := lever.New(srv.URL).Fetch(ctx, "spotify"); err == nil {
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
