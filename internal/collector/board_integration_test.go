//go:build integration

package collector_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunt-agent/internal/collector"
	"github.com/shinraxtensei/job-hunt-agent/internal/job"
)

// boardSource is a board-wide source (ADR-0029): one fetch, many companies. It returns
// `jobs` (company, title, id) as Remotive-shaped payloads.
type boardSource struct {
	jobs    [][3]string
	fetches atomic.Int32
}

func (*boardSource) Name() string               { return "remotive" }
func (*boardSource) Host() string               { return "remotive.test" }
func (*boardSource) BoardWide() bool            { return true }
func (*boardSource) MinInterval() time.Duration { return 6 * time.Hour }
func (b *boardSource) Fetch(ctx context.Context, _ string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	if err := limit.Wait(ctx); err != nil {
		return nil, job.Skipped{}, err
	}
	b.fetches.Add(1)
	var out []job.Raw
	for _, j := range b.jobs {
		url := "https://remotive.test/remote-jobs/" + j[2]
		out = append(out, job.Raw{
			Source: "remotive", SourceID: j[2], URL: url, Title: j[1], Company: j[0],
			Payload: fmt.Appendf(nil, `{"id":%q,"url":%q,"title":%q,"company_name":%q,`+
				`"category":"Software Development","candidate_required_location":"Worldwide",`+
				`"publication_date":"2026-11-08T10:00:00","description":"<p>Go</p>"}`, j[2], url, j[1], j[0]),
		})
	}
	return out, job.Skipped{}, nil
}

type posting struct {
	company, origin string
	closed          bool
}

func (e *env) bySource(source string) map[string]posting {
	e.t.Helper()
	rows, err := e.pool.Query(e.t.Context(),
		`SELECT p.source_posting_id, c.name, p.origin, p.closed_at IS NOT NULL
		 FROM postings p JOIN companies c ON c.id = p.company_id WHERE p.source = $1`, source)
	if err != nil {
		e.t.Fatalf("reading postings: %v", err)
	}
	defer rows.Close()
	out := map[string]posting{}
	for rows.Next() {
		var id string
		var p posting
		if err := rows.Scan(&id, &p.company, &p.origin, &p.closed); err != nil {
			e.t.Fatalf("scanning: %v", err)
		}
		out[id] = p
	}
	return out
}

func (e *env) runBoard(b *boardSource) collector.RunResult {
	e.t.Helper()
	res, err := collector.Run(e.t.Context(), e.pool, collector.Registry{"remotive": b},
		collector.Config{Concurrency: 2}, quiet())
	if err != nil {
		e.t.Fatalf("Run: %v", err)
	}
	return res
}

// AC1: one fetch stores each posting under its own company: a watch-list company of that
// name (any case) when there is one, else a board-less one; origin is 'board'.
func TestBoardPostingsGoToTheirCompanies(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	e.add("remotive", "Remotive", true, false)
	watched := fmt.Sprintf("Watched %d", e.n)
	e.add("lever", watched, true, false)
	other := fmt.Sprintf("Board Only %d", e.n)
	id := func(s string) string { return fmt.Sprintf("%s-%d", s, e.n) }
	b := &boardSource{jobs: [][3]string{
		{"  " + strings.ToUpper(watched) + " ", "Backend Engineer", id("a")},
		{other, "Go Developer", id("b")},
	}}
	res := e.runBoard(b)
	if res.Outcome != "ok" || res.New != 2 {
		t.Fatalf("run: %+v", res)
	}
	got := e.bySource("remotive")
	if p := got[id("a")]; p.company != watched || p.origin != "board" {
		t.Errorf("posting a: %+v, want company %q, origin board", p, watched)
	}
	if p := got[id("b")]; p.company != other || p.origin != "board" {
		t.Errorf("posting b: %+v, want board-less company %q", p, other)
	}
}

// AC1: Remotive is fetched at most once per 6 hours (4 a day), however often runs go; a
// skipped board is not a failure.
func TestBoardMinIntervalSkipsEarlyRuns(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	e.add("remotive", "Remotive", true, false)
	b := &boardSource{}
	e.runBoard(b)
	res := e.runBoard(b)
	if n := b.fetches.Load(); n != 1 {
		t.Errorf("fetched %d times, want 1", n)
	}
	if res.Outcome != "ok" || res.Failed != 0 {
		t.Errorf("second run: %+v, want ok with nothing failed", res)
	}
	if _, err := e.pool.Exec(t.Context(),
		`UPDATE companies SET fetched_at = now() - interval '7 hours' WHERE board_type = 'remotive'`); err != nil {
		t.Fatal(err)
	}
	e.runBoard(b)
	if n := b.fetches.Load(); n != 2 {
		t.Errorf("after the interval: fetched %d times, want 2", n)
	}
}

// ADR-0010 for a board: what the board no longer lists closes, whatever its company.
func TestBoardClosesMissingAcrossCompanies(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	e.add("remotive", "Remotive", true, false)
	id := func(s string) string { return fmt.Sprintf("%s-%d", s, e.n) }
	b := &boardSource{jobs: [][3]string{
		{"One Co", "Backend Engineer", id("1")},
		{"Two Co", "Backend Engineer", id("2")},
	}}
	e.runBoard(b)
	b.jobs = b.jobs[:1]
	if _, err := e.pool.Exec(t.Context(), `UPDATE companies SET fetched_at = NULL`); err != nil {
		t.Fatal(err)
	}
	res := e.runBoard(b)
	got := e.bySource("remotive")
	if got[id("1")].closed || !got[id("2")].closed || res.Closed < 1 {
		t.Errorf("postings %+v (closed %d), want 2 closed and 1 open", got, res.Closed)
	}
}

// AC3: the same job on a company's own feed and on a board is one group, and the
// company's posting is canonical, even when the board saw it first.
func TestBoardPostingGroupsWithTheCompanyFeed(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	e.add("remotive", "Remotive", true, false)
	name := fmt.Sprintf("Grouped %d", e.n)
	slug := e.add("lever", name, true, false)
	b := &boardSource{jobs: [][3]string{{name, "Senior Backend Engineer", fmt.Sprintf("g-%d", e.n)}}}
	e.runBoard(b)

	feed := newFake("lever", "lever.test", &[2]atomic.Int32{})
	feed.title = "Senior Backend Engineer"
	if _, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": feed},
		collector.Config{Concurrency: 1}, quiet()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var members int
	var canonicalSource string
	if err := e.pool.QueryRow(t.Context(),
		`SELECT g.members, p.source FROM job_groups g
		 JOIN companies c ON c.id = g.company_id JOIN postings p ON p.id = g.canonical_posting_id
		 WHERE c.board_slug = $1`, slug).Scan(&members, &canonicalSource); err != nil {
		t.Fatalf("reading group: %v", err)
	}
	if members != 2 || canonicalSource != "lever" {
		t.Errorf("group: %d members, canonical from %s; want 2, lever", members, canonicalSource)
	}
}
