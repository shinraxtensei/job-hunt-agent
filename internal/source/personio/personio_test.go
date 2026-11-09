package personio

import (
	"encoding/json"
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

func serve(t *testing.T, status int, body []byte) (*Client, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if status != http.StatusOK {
			http.Error(w, "upstream detail", status)
			return
		}
		_, _ = w.Write(body) // a failed write shows up as a client error
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL + "/%s/xml"), &paths
}

func noLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 1) }

// M7-02 AC1: a recorded feed (Inyad, 2026-11-09, trimmed) gives one raw record per
// position: its fields as a JSON object, the public job URL, the title.
func TestFetchGoldenFile(t *testing.T) {
	c, paths := serve(t, http.StatusOK, fixture(t, "inyad.xml"))
	got, skipped, err := c.Fetch(t.Context(), "inyad", noLimit())
	if err != nil || skipped.Total() != 0 {
		t.Fatalf("Fetch: %v, skipped %d", err, skipped.Total())
	}
	if (*paths)[0] != "/inyad/xml" {
		t.Errorf("requested %v", *paths)
	}
	if len(got) != 3 {
		t.Fatalf("%d records, want 3", len(got))
	}
	r := got[2]
	if r.Source != "personio" || r.SourceID != "578189" || r.Title != "Android Software Engineer" ||
		r.URL != "https://inyad.jobs.personio.de/job/578189" {
		t.Errorf("record: %+v", r)
	}
	var p map[string]any
	if err := json.Unmarshal(r.Payload, &p); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"office": "Casablanca", "seniority": "experienced", "yearsOfExperience": "2-5",
		"createdAt": "2022-02-01T16:33:49+00:00", "url": r.URL} {
		if p[k] != want {
			t.Errorf("%s = %v, want %q", k, p[k], want)
		}
	}
	if d, _ := p["jobDescriptions"].([]any); len(d) != 2 {
		t.Errorf("descriptions: %v", p["jobDescriptions"])
	}
}

func TestEmptyFeed(t *testing.T) {
	c, _ := serve(t, http.StatusOK, []byte(`<?xml version="1.0"?><workzag-jobs></workzag-jobs>`))
	got, _, err := c.Fetch(t.Context(), "acme", noLimit())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty feed: %v, %v", got, err)
	}
}

func TestMalformedPositionSkipped(t *testing.T) {
	body := `<workzag-jobs><position><name>No id</name></position><position><id>1</id><name>Backend Engineer</name><office>Rabat</office></position><position><id>2</id></position></workzag-jobs>`
	c, _ := serve(t, http.StatusOK, []byte(body))
	got, skipped, err := c.Fetch(t.Context(), "acme", noLimit())
	if err != nil || len(got) != 1 || got[0].SourceID != "1" || skipped.Total() != 2 {
		t.Fatalf("got %+v, skipped %d, %v", got, skipped.Total(), err)
	}
}

func TestNotAFeed(t *testing.T) {
	for _, body := range []string{`<html><body>Login</body></html>`, `{"jobs": []}`, `<workzag-jobs><position><id>1`} {
		c, _ := serve(t, http.StatusOK, []byte(body))
		if _, _, err := c.Fetch(t.Context(), "acme", noLimit()); err == nil {
			t.Errorf("%.30s: want an error", body)
		}
	}
}

// The slug becomes a host name: anything else is refused before a request is made.
func TestBadSlugRefused(t *testing.T) {
	c, paths := serve(t, http.StatusOK, nil)
	for _, slug := range []string{"", "evil.com/", "a b", "../x", "UPPER", strings.Repeat("a", 70)} {
		if _, _, err := c.Fetch(t.Context(), slug, noLimit()); err == nil {
			t.Errorf("%q accepted", slug)
		}
	}
	if len(*paths) != 0 {
		t.Errorf("requests made: %v", *paths)
	}
}

func TestHTTPErrorNamesTheCompany(t *testing.T) {
	c, _ := serve(t, http.StatusNotFound, nil)
	_, _, err := c.Fetch(t.Context(), "acme", noLimit())
	if err == nil || !strings.Contains(err.Error(), "personio: fetch acme") || strings.Contains(err.Error(), "upstream detail") {
		t.Fatalf("err = %v", err)
	}
}

func TestHostSharedAcrossCompanies(t *testing.T) {
	if h := New(URL).Host(); h != "jobs.personio.de" {
		t.Errorf("Host = %q", h)
	}
}

// Parsers never panic on hostile input.
func FuzzParse(f *testing.F) {
	if b, err := os.ReadFile(filepath.Join("testdata", "inyad.xml")); err == nil {
		f.Add(b)
	}
	f.Add([]byte(`<workzag-jobs><position><id>1</id><name>x</name></position></workzag-jobs>`))
	f.Add([]byte(`<!DOCTYPE x [<!ENTITY e SYSTEM "file:///etc/passwd">]><workzag-jobs>&e;</workzag-jobs>`))
	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _, _ = parse(b, "acme")
	})
}
