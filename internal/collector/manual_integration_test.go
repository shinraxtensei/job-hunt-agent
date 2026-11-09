//go:build integration

package collector_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shinraxtensei/job-hunt-agent/internal/collector"
	"github.com/shinraxtensei/job-hunt-agent/internal/source/lever"
)

// postingUUID is a Lever-style id unique to a test: Lever ids are global, so two tests
// sharing one would share the stored posting.
func postingUUID(n int64) string {
	return fmt.Sprintf("3f9d5a7e-1b2c-4d3e-8f4a-%012x", uint64(n)%(1<<48))
}

// leverOne is a Lever API that serves one posting at /v0/postings/<site>/<id> while up is
// true (404 otherwise) and records every path asked for.
type leverOne struct {
	id   string
	srv  *httptest.Server
	up   atomic.Bool
	mu   sync.Mutex
	hits []string
}

func newLeverOne(t *testing.T, site, manualID string) *leverOne {
	t.Helper()
	l := &leverOne{id: manualID}
	l.up.Store(true)
	l.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		l.hits = append(l.hits, r.URL.Path)
		l.mu.Unlock()
		if r.URL.Path != "/v0/postings/"+site+"/"+manualID || !l.up.Load() {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"id":%q,"text":"Senior Backend Engineer","hostedUrl":"https://jobs.lever.co/%s/%s","workplaceType":"remote","categories":{"location":"Remote - EMEA","team":"Platform"}}`,
			manualID, site, manualID)
	}))
	t.Cleanup(l.srv.Close)
	return l
}

func (l *leverOne) requests() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.hits...)
}

type postingRow struct {
	source, origin, url string
	closed, active      bool
	company             string
	roleFamily          string
}

func postingByID(t *testing.T, e *env, id int64) postingRow {
	t.Helper()
	var r postingRow
	if err := e.pool.QueryRow(t.Context(),
		`SELECT p.source, p.origin, p.url, p.closed_at IS NOT NULL, c.active, c.name, coalesce(j.role_family, '')
		 FROM postings p JOIN companies c ON c.id = p.company_id LEFT JOIN jobs j ON j.posting_id = p.id
		 WHERE p.id = $1`, id).Scan(&r.source, &r.origin, &r.url, &r.closed, &r.active, &r.company, &r.roleFamily); err != nil {
		t.Fatalf("reading posting %d: %v", id, err)
	}
	return r
}

// M2-07 AC1 + AC5: a board link is fetched through the adapter's own API (by board and
// id), stored as a manual posting of a new inactive company, normalised and classified;
// adding it again creates nothing and fetches nothing.
func TestAddBoardLink(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	site, manualID := fmt.Sprintf("acme%d", e.n), postingUUID(e.n)
	api := newLeverOne(t, site, manualID)
	reg := collector.Registry{"lever": lever.New(api.srv.URL)}
	link := "https://jobs.lever.co/" + site + "/" + manualID + "/apply?lever-origin=applied"

	res, err := collector.Add(t.Context(), e.pool, reg, 0, link, collector.Manual{}, quiet())
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.Existed || !res.Board || res.PostingID == 0 {
		t.Fatalf("result = %+v", res)
	}
	p := postingByID(t, e, res.PostingID)
	if p.source != "lever" || p.origin != "manual" || p.active || p.company != site || p.roleFamily != "backend" {
		t.Errorf("posting = %+v, want a manual lever posting of an inactive company, classified backend", p)
	}
	if got := api.requests(); len(got) != 1 || got[0] != "/v0/postings/"+site+"/"+manualID {
		t.Errorf("API requests = %v, want exactly the posting's API path", got)
	}

	again, err := collector.Add(t.Context(), e.pool, reg, 0, "https://JOBS.lever.co/"+site+"/"+manualID, collector.Manual{}, quiet())
	if err != nil || !again.Existed || again.PostingID != res.PostingID {
		t.Errorf("second add = %+v, %v; want existed with the same id", again, err)
	}
	if n := len(api.requests()); n != 1 {
		t.Errorf("the second add fetched again (%d requests)", n)
	}
}

// M2-07 AC2 + AC3: any other link is never fetched, not even one that points at a live
// server through a userinfo trick; it needs --title and --company, and duplicates are
// found by the normalised link.
func TestAddOtherLinkIsNeverFetched(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	manualID := postingUUID(e.n)
	api := newLeverOne(t, "acme", manualID)
	reg := collector.Registry{"lever": lever.New(api.srv.URL)}
	host := strings.TrimPrefix(api.srv.URL, "http://")
	m := collector.Manual{Title: "Développeur Backend Go", Company: fmt.Sprintf("Atlas %d", e.n), Location: "Casablanca"}

	if _, err := collector.Add(t.Context(), e.pool, reg, 0, "https://www.linkedin.com/jobs/view/4012345678/", collector.Manual{Title: "x"}, quiet()); err == nil ||
		!strings.Contains(err.Error(), "--company") {
		t.Errorf("missing --company: err = %v", err)
	}
	if _, err := collector.Add(t.Context(), e.pool, reg, 0, "http://jobs.lever.co/acme/"+manualID, m, quiet()); err == nil ||
		!strings.Contains(err.Error(), "use the https:// link") {
		t.Errorf("http board link: err = %v, want a refusal asking for https", err)
	}
	for _, hostile := range []string{
		"https://jobs.lever.co@" + host + "/acme/" + manualID,
		"http://" + host + "/v0/postings/acme/" + manualID,
		"https://jobs.lever.co.evil.example/acme/" + manualID,
	} {
		res, err := collector.Add(t.Context(), e.pool, reg, 0, hostile, m, quiet())
		if strings.Contains(hostile, "@") {
			if err == nil {
				t.Errorf("%s: a link with user info was accepted", hostile)
			}
			continue
		}
		if err != nil || res.Board {
			t.Errorf("%s: %+v, %v; want a manual (non-board) posting", hostile, res, err)
		}
	}
	if got := api.requests(); len(got) != 0 {
		t.Fatalf("a non-board link was fetched: %v", got)
	}

	res, err := collector.Add(t.Context(), e.pool, reg, 0, "https://www.linkedin.com/jobs/view/4012345678/?trk=public_jobs", m, quiet())
	if err != nil || res.Board || res.Existed {
		t.Fatalf("Add = %+v, %v", res, err)
	}
	p := postingByID(t, e, res.PostingID)
	if p.source != "manual" || p.origin != "manual" || p.url != "https://www.linkedin.com/jobs/view/4012345678" ||
		p.company != m.Company || p.active || p.roleFamily != "backend" {
		t.Errorf("posting = %+v", p)
	}
	again, err := collector.Add(t.Context(), e.pool, reg, 0, "HTTPS://www.linkedin.com/jobs/view/4012345678?utm_source=x", m, quiet())
	if err != nil || !again.Existed || again.PostingID != res.PostingID {
		t.Errorf("second add = %+v, %v; want existed", again, err)
	}
	if got := api.requests(); len(got) != 0 {
		t.Errorf("requests = %v, want none", got)
	}
}

// M2-07 AC4: collector run rechecks manual board postings not rechecked for two days,
// through the adapter's API: gone → closed, back → reopened, counted in collector_runs.
// Manual non-board postings are never rechecked.
func TestRecheckManualPostings(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	site, manualID := fmt.Sprintf("recheck%d", e.n), postingUUID(e.n)
	api := newLeverOne(t, site, manualID)
	reg := collector.Registry{"lever": lever.New(api.srv.URL)}
	res, err := collector.Add(t.Context(), e.pool, reg, 0, "https://jobs.lever.co/"+site+"/"+manualID, collector.Manual{}, quiet())
	if err != nil {
		t.Fatal(err)
	}
	other, err := collector.Add(t.Context(), e.pool, reg, 0, fmt.Sprintf("https://example.ma/jobs/%d", e.n),
		collector.Manual{Title: "Data Engineer", Company: fmt.Sprintf("Other %d", e.n)}, quiet())
	if err != nil {
		t.Fatal(err)
	}
	age := func(id int64) {
		if _, err := e.pool.Exec(t.Context(), `UPDATE postings SET last_rechecked_at = now() - interval '3 days' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
	run := func() collector.RunResult {
		r, err := collector.Run(t.Context(), e.pool, reg, collector.Config{Concurrency: 1}, quiet())
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	if r := run(); r.ManualRechecked != 0 {
		t.Errorf("rechecked %d postings added just now, want 0", r.ManualRechecked)
	}

	api.up.Store(false)
	age(res.PostingID)
	age(other.PostingID)
	before := len(api.requests())
	r := run()
	if r.ManualRechecked != 1 || r.Closed != 1 || !postingByID(t, e, res.PostingID).closed {
		t.Errorf("gone: run = %+v, closed = %v", r, postingByID(t, e, res.PostingID).closed)
	}
	if postingByID(t, e, other.PostingID).closed || len(api.requests()) != before+1 {
		t.Errorf("the non-board posting was rechecked, or more than one request was made (%d)", len(api.requests())-before)
	}
	var recorded int
	if err := e.pool.QueryRow(t.Context(), `SELECT manual_rechecked FROM collector_runs WHERE id = $1`, r.RunID).Scan(&recorded); err != nil || recorded != 1 {
		t.Errorf("collector_runs.manual_rechecked = %d, %v; want 1", recorded, err)
	}

	api.up.Store(true)
	age(res.PostingID)
	if r := run(); r.ManualRechecked != 1 || r.Reopened != 1 || postingByID(t, e, res.PostingID).closed {
		t.Errorf("back: run = %+v, closed = %v, requests %v", r, postingByID(t, e, res.PostingID).closed, api.requests())
	}
}
