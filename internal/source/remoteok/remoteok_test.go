package remoteok

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
	return New(srv.URL + "/api"), &n
}

func noLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

// M7-03 AC1: a recorded response (2026-11-09, the legal notice and 4 jobs) gives the jobs
// tagged dev or engineer, each with its company and its Remote OK URL; the legal notice
// and the untagged job are not jobs.
func TestFetchGoldenFile(t *testing.T) {
	c, n := serve(t, http.StatusOK, fixture(t, "api.json"))
	got, skipped, err := c.Fetch(t.Context(), Slug, noLimit())
	if err != nil || skipped.Total() != 0 || *n != 1 {
		t.Fatalf("Fetch: %v, skipped %d, %d requests", err, skipped.Total(), *n)
	}
	if len(got) != 3 {
		t.Fatalf("%d records, want 3", len(got))
	}
	r := got[1]
	if r.Source != "remoteok" || r.SourceID != "1137688" || r.Company != "Lemon.io" || r.Title != "Senior Vue Developer" ||
		!strings.HasPrefix(r.URL, "https://remoteok.com/remote-jobs/") {
		t.Errorf("record: %+v", r)
	}
	if !strings.Contains(string(r.Payload), `"position"`) {
		t.Errorf("payload is not the job object: %.80s", r.Payload)
	}
}

func TestEmptyBoard(t *testing.T) {
	c, _ := serve(t, http.StatusOK, []byte(`[{"legal":"..."}]`))
	got, _, err := c.Fetch(t.Context(), Slug, noLimit())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty board: %v, %v", got, err)
	}
}

// A job without an id, position, company or a Remote OK URL is skipped, never the fetch.
func TestMalformedJobSkipped(t *testing.T) {
	body := `[{"legal":"x"},` +
		`{"id":"1","position":"Backend Engineer","company":"Acme","tags":["dev"],"url":"https://remoteOK.com/remote-jobs/a-1"},` +
		`{"id":"2","position":"No company","tags":["dev"],"url":"https://remoteOK.com/remote-jobs/b-2"},` +
		`{"id":"3","position":"Elsewhere","company":"Evil","tags":["dev"],"url":"https://evil.test/x"},` +
		`{"position":"No id","company":"Acme","tags":["engineer"]},` +
		`{"id":"5","position":"Bad tags","company":"Acme","tags":"dev","url":"https://remoteok.com/remote-jobs/e-5"}]`
	c, _ := serve(t, http.StatusOK, []byte(body))
	got, skipped, err := c.Fetch(t.Context(), Slug, noLimit())
	if err != nil || len(got) != 1 || got[0].SourceID != "1" || skipped.Total() != 4 {
		t.Fatalf("got %+v, skipped %d, %v", got, skipped.Total(), err)
	}
}

// Names come HTML-escaped or mis-decoded; the company row gets the repaired name.
func TestNamesRepaired(t *testing.T) {
	body := `[{"legal":"x"},{"id":"1","position":"Ops &amp; Infra Engineer","company":"Sotheby\u00e2\u0080\u0099s","tags":["dev"],"url":"https://remoteOK.com/remote-jobs/a-1"}]`
	c, _ := serve(t, http.StatusOK, []byte(body))
	got, _, err := c.Fetch(t.Context(), Slug, noLimit())
	if err != nil || len(got) != 1 || got[0].Company != "Sotheby\u2019s" || got[0].Title != "Ops & Infra Engineer" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestNotAJobList(t *testing.T) {
	for _, body := range []string{`<html>blocked</html>`, `{"jobs": []}`, `[]`, `"x"`} {
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
	if err == nil || !strings.Contains(err.Error(), "remoteok: fetch") || strings.Contains(err.Error(), "upstream detail") {
		t.Fatalf("err = %v", err)
	}
}

func TestCancelledContextStops(t *testing.T) {
	c, n := serve(t, http.StatusOK, fixture(t, "api.json"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := c.Fetch(ctx, Slug, noLimit()); err == nil || *n != 0 {
		t.Errorf("err %v, %d requests", err, *n)
	}
}

func TestBoardWide(t *testing.T) {
	c := New(URL)
	if !c.BoardWide() || c.Host() != "remoteok.com" || c.MinInterval().Hours() < 6 {
		t.Errorf("BoardWide %v, Host %q, MinInterval %v", c.BoardWide(), c.Host(), c.MinInterval())
	}
}

// Parsers never panic on hostile input.
func FuzzParse(f *testing.F) {
	if b, err := os.ReadFile(filepath.Join("testdata", "api.json")); err == nil {
		f.Add(b)
	}
	f.Add([]byte(`[{"legal":"x"},{"id":"1","position":"x","company":"y","tags":["dev"],"url":"https://remoteok.com/remote-jobs/x"}]`))
	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _, _ = parse(b)
	})
}
