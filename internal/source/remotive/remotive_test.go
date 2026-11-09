package remotive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func serve(t *testing.T, status int, body []byte) (*Client, *int) {
	t.Helper()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n++
		if status != http.StatusOK {
			http.Error(w, "upstream detail", status)
			return
		}
		_, _ = w.Write(body) // a failed write shows up as a client error
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL + "/api/remote-jobs"), &n
}

func noLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

// M7-03 AC1: a recorded response (2026-11-09, trimmed to 4 jobs) gives the software jobs
// only, each with its company, its Remotive URL and the job object as stored.
func TestFetchGoldenFile(t *testing.T) {
	c, n := serve(t, http.StatusOK, fixture(t, "software_dev.json"))
	got, skipped, err := c.Fetch(t.Context(), Slug, noLimit())
	if err != nil || skipped.Total() != 0 || *n != 1 {
		t.Fatalf("Fetch: %v, skipped %d, %d requests", err, skipped.Total(), *n)
	}
	if len(got) != 3 {
		t.Fatalf("%d records, want 3 (the Writing job dropped)", len(got))
	}
	r := got[0]
	if r.Source != "remotive" || r.SourceID != "2091165" || r.Company != "Fluence International, Inc." ||
		!strings.HasPrefix(r.URL, "https://remotive.com/remote-jobs/") || r.Title == "" {
		t.Errorf("record: %+v", r)
	}
	if !strings.Contains(string(r.Payload), `"candidate_required_location"`) {
		t.Errorf("payload is not the job object: %.80s", r.Payload)
	}
}

func TestEmptyBoard(t *testing.T) {
	c, _ := serve(t, http.StatusOK, []byte(`{"job-count":0,"jobs":[]}`))
	got, _, err := c.Fetch(t.Context(), Slug, noLimit())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty board: %v, %v", got, err)
	}
}

// A job without an id, title, company or Remotive URL is skipped, never the whole fetch.
func TestMalformedJobSkipped(t *testing.T) {
	body := `{"jobs":[` +
		`{"id":1,"title":"Backend Engineer","company_name":"Acme","category":"Software Development","url":"https://remotive.com/remote-jobs/software-dev/a-1"},` +
		`{"id":2,"title":"No company","category":"Software Development","url":"https://remotive.com/remote-jobs/software-dev/b-2"},` +
		`{"id":3,"title":"Elsewhere","company_name":"Evil","category":"Software Development","url":"https://evil.test/x"},` +
		`{"title":"No id","company_name":"Acme","category":"Software Development"},` +
		`"not an object"]}`
	c, _ := serve(t, http.StatusOK, []byte(body))
	got, skipped, err := c.Fetch(t.Context(), Slug, noLimit())
	if err != nil || len(got) != 1 || got[0].SourceID != "1" || skipped.Total() != 4 {
		t.Fatalf("got %+v, skipped %d, %v", got, skipped.Total(), err)
	}
}

func TestNotAJobList(t *testing.T) {
	for _, body := range []string{`<html>blocked</html>`, `{"job-count": 3}`, `[]`, `{"jobs": {}}`} {
		c, _ := serve(t, http.StatusOK, []byte(body))
		if _, _, err := c.Fetch(t.Context(), Slug, noLimit()); err == nil {
			t.Errorf("%.30s: want an error", body)
		}
	}
}

func TestOnlyItsOwnSlug(t *testing.T) {
	c, n := serve(t, http.StatusOK, nil)
	if _, _, err := c.Fetch(t.Context(), "other", noLimit()); err == nil || *n != 0 {
		t.Errorf("err %v, %d requests", err, *n)
	}
}

func TestHTTPErrorNamesTheBoard(t *testing.T) {
	c, _ := serve(t, http.StatusForbidden, nil)
	_, _, err := c.Fetch(t.Context(), Slug, noLimit())
	if err == nil || !strings.Contains(err.Error(), "remotive: fetch") || strings.Contains(err.Error(), "upstream detail") {
		t.Fatalf("err = %v", err)
	}
}

func TestCancelledContextStops(t *testing.T) {
	c, n := serve(t, http.StatusOK, fixture(t, "software_dev.json"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := c.Fetch(ctx, Slug, noLimit()); err == nil || *n != 0 {
		t.Errorf("err %v, %d requests", err, *n)
	}
}

// ADR-0029: a board-wide source fetched at most 4 times a day.
func TestBoardWideAndAtMostFourADay(t *testing.T) {
	c := New(URL)
	if !c.BoardWide() || c.MinInterval().Hours() < 6 || c.Host() != "remotive.com" {
		t.Errorf("BoardWide %v, MinInterval %v, Host %q", c.BoardWide(), c.MinInterval(), c.Host())
	}
}

// Parsers never panic on hostile input.
func FuzzParse(f *testing.F) {
	if b, err := os.ReadFile(filepath.Join("testdata", "software_dev.json")); err == nil {
		f.Add(b)
	}
	f.Add([]byte(`{"jobs":[{"id":"1","title":"x","company_name":"y","category":"Software Development","url":"https://remotive.com/remote-jobs/x"}]}`))
	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _, _ = parse(b)
	})
}
