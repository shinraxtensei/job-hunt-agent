//go:build integration

package collector_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/goleak"
	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunt-agent/internal/collector"
	"github.com/shinraxtensei/job-hunt-agent/internal/job"
	"github.com/shinraxtensei/job-hunt-agent/internal/store/storetest"
)

// fakeSource records every Fetch: which slug, when, and how many ran at once. Behaviour
// per slug: fail returns an error, block waits for the context to be cancelled.
type fakeSource struct {
	name, host string
	title      string // when set, the posting's title in a Lever-shaped payload
	delay      time.Duration
	fail       map[string]bool
	block      map[string]bool
	inFlight   *atomic.Int32 // shared across sources to measure total concurrency
	maxSeen    *atomic.Int32

	mu    sync.Mutex
	calls []call
}

type call struct {
	slug string
	at   time.Time
}

func newFake(name, host string, tracker *[2]atomic.Int32) *fakeSource {
	return &fakeSource{name: name, host: host, inFlight: &tracker[0], maxSeen: &tracker[1],
		fail: map[string]bool{}, block: map[string]bool{}}
}

func (f *fakeSource) Name() string { return f.name }
func (f *fakeSource) Host() string { return f.host }

// Fetch waits on the host limiter first, as every Source must before each request.
func (f *fakeSource) Fetch(ctx context.Context, slug string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	if err := limit.Wait(ctx); err != nil {
		return nil, job.Skipped{}, err
	}
	f.mu.Lock()
	f.calls = append(f.calls, call{slug, time.Now()})
	f.mu.Unlock()

	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		m := f.maxSeen.Load()
		if n <= m || f.maxSeen.CompareAndSwap(m, n) {
			break
		}
	}

	if f.block[slug] {
		<-ctx.Done()
		return nil, job.Skipped{}, fmt.Errorf("%s: fetch %s: %w", f.name, slug, ctx.Err())
	}
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return nil, job.Skipped{}, ctx.Err()
	}
	if f.fail[slug] {
		return nil, job.Skipped{}, fmt.Errorf("%s: fetch %s: unexpected status 500 Internal Server Error", f.name, slug)
	}
	id := fmt.Sprintf("%s-%d", slug, time.Now().UnixNano())
	payload := fmt.Sprintf(`{"id":%q}`, id)
	if f.title != "" {
		payload = fmt.Sprintf(`{"id":%q,"text":%q}`, id, f.title)
	}
	return []job.Raw{{
		Source: f.name, SourceID: id, URL: "https://example.test/" + id, Title: "Engineer",
		Payload: []byte(payload),
	}}, job.Skipped{}, nil
}

func (f *fakeSource) fetched() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

func (f *fakeSource) slugs() map[string]bool {
	out := map[string]bool{}
	for _, c := range f.fetched() {
		out[c.slug] = true
	}
	return out
}

// env is one run test's database: every pre-existing company deactivated, so Run sees
// only the companies this test adds.
type env struct {
	t    *testing.T
	pool *pgxpool.Pool
	n    int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	storetest.Migrate(t, storetest.OwnerURL)
	pool, err := pgxpool.New(t.Context(), storetest.AppURL)
	if err != nil {
		t.Fatalf("opening pool: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE companies SET active = false`); err != nil {
		t.Fatalf("resetting companies: %v", err)
	}
	return &env{t: t, pool: pool, n: time.Now().UnixNano()}
}

// add inserts a company and returns its slug (unique per test run).
func (e *env) add(boardType, name string, active, excluded bool) string {
	e.t.Helper()
	slug := fmt.Sprintf("%s-%d", strings.ToLower(name), e.n)
	if _, err := e.pool.Exec(e.t.Context(),
		`INSERT INTO companies (name, board_type, board_slug, track, active, excluded)
		 VALUES ($1, $2, $3, 'remote', $4, $5)`, name, boardType, slug, active, excluded); err != nil {
		e.t.Fatalf("adding company %s: %v", name, err)
	}
	return slug
}

func (e *env) postings(slug string) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.t.Context(),
		`SELECT count(*) FROM postings p JOIN companies c ON c.id = p.company_id WHERE c.board_slug = $1`,
		slug).Scan(&n); err != nil {
		e.t.Fatalf("counting postings: %v", err)
	}
	return n
}

type runRow struct {
	outcome                               string
	ended                                 bool
	attempted, failed, skipped, new, seen int
}

func (e *env) run(id int64) runRow {
	e.t.Helper()
	var r runRow
	if err := e.pool.QueryRow(e.t.Context(),
		`SELECT coalesce(outcome, ''), ended_at IS NOT NULL, companies_attempted, companies_failed,
		        companies_skipped, postings_new, postings_seen
		 FROM collector_runs WHERE id = $1`, id).Scan(
		&r.outcome, &r.ended, &r.attempted, &r.failed, &r.skipped, &r.new, &r.seen); err != nil {
		e.t.Fatalf("reading run %d: %v", id, err)
	}
	return r
}

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

// AC1: companies across board types are fetched concurrently, never more than
// Concurrency at once.
func TestRunFetchesConcurrentlyWithinTheLimit(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	e := newEnv(t)
	defer e.pool.Close()

	var tracker [2]atomic.Int32
	lever := newFake("lever", "lever.test", &tracker)
	greenhouse := newFake("greenhouse", "greenhouse.test", &tracker)
	lever.delay, greenhouse.delay = 150*time.Millisecond, 150*time.Millisecond
	var slugs []string
	for i := range 5 {
		slugs = append(slugs, e.add("lever", fmt.Sprintf("L%d", i), true, false))
		slugs = append(slugs, e.add("greenhouse", fmt.Sprintf("G%d", i), true, false))
	}

	res, err := collector.Run(t.Context(), e.pool,
		collector.Registry{"lever": lever, "greenhouse": greenhouse},
		collector.Config{Concurrency: 3}, quiet())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := tracker[1].Load(); got != 3 {
		t.Errorf("max concurrent fetches = %d, want exactly 3 (the limit, and reached)", got)
	}
	if res.Outcome != "ok" || res.Attempted != 10 || res.New != 10 {
		t.Errorf("result = %+v, want ok, 10 attempted, 10 new", res)
	}
	for _, s := range slugs {
		if e.postings(s) != 1 {
			t.Errorf("company %s: %d postings stored, want 1", s, e.postings(s))
		}
	}
}

// AC2: requests to one host are spaced at least HostInterval apart; another host is
// limited separately, not queued behind it.
func TestRunRateLimitsEachHost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	e := newEnv(t)
	defer e.pool.Close()

	const interval = 200 * time.Millisecond
	var tracker [2]atomic.Int32
	lever := newFake("lever", "lever.test", &tracker)
	greenhouse := newFake("greenhouse", "greenhouse.test", &tracker)
	for i := range 4 {
		e.add("lever", fmt.Sprintf("RL%d", i), true, false)
	}
	for i := range 2 {
		e.add("greenhouse", fmt.Sprintf("RG%d", i), true, false)
	}

	start := time.Now()
	if _, err := collector.Run(t.Context(), e.pool,
		collector.Registry{"lever": lever, "greenhouse": greenhouse},
		collector.Config{Concurrency: 8, HostInterval: interval}, quiet()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, f := range []*fakeSource{lever, greenhouse} {
		calls := f.fetched()
		for i := 1; i < len(calls); i++ {
			// A few ms of slack for timer granularity; a missing limiter gives ~0.
			if gap := calls[i].at.Sub(calls[i-1].at); gap < interval-10*time.Millisecond {
				t.Errorf("%s: requests %d and %d were %v apart, want >= %v", f.host, i-1, i, gap, interval)
			}
		}
	}
	if gh := greenhouse.fetched(); len(gh) == 0 || gh[0].at.Sub(start) > interval {
		t.Errorf("greenhouse.test waited behind lever.test: first request at %v, want within %v",
			gh[0].at.Sub(start), interval)
	}
}

// AC3: one failing company doesn't stop the rest; its error is recorded; the run is
// "partial".
func TestRunRecordsFailureAndEndsPartial(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	e := newEnv(t)
	defer e.pool.Close()

	var tracker [2]atomic.Int32
	lever := newFake("lever", "lever.test", &tracker)
	ok1, broken, ok2 := e.add("lever", "Ok1", true, false), e.add("lever", "Broken", true, false), e.add("lever", "Ok2", true, false)
	lever.fail[broken] = true

	res, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": lever},
		collector.Config{Concurrency: 2}, quiet())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Outcome != "partial" || res.Failed != 1 || res.Attempted != 3 {
		t.Errorf("result = %+v, want partial, 1 failed of 3", res)
	}
	if e.postings(ok1) != 1 || e.postings(ok2) != 1 || e.postings(broken) != 0 {
		t.Errorf("postings ok1=%d ok2=%d broken=%d, want 1, 1, 0", e.postings(ok1), e.postings(ok2), e.postings(broken))
	}
	if r := e.run(res.RunID); r != (runRow{"partial", true, 3, 1, 0, 2, 0}) {
		t.Errorf("collector_runs row = %+v, want partial, ended, 3 attempted, 1 failed, 2 new", r)
	}
	var msg string
	if err := e.pool.QueryRow(t.Context(),
		`SELECT r.error FROM collector_run_errors r JOIN companies c ON c.id = r.company_id
		 WHERE r.run_id = $1 AND c.board_slug = $2`, res.RunID, broken).Scan(&msg); err != nil {
		t.Fatalf("reading recorded error: %v", err)
	}
	if !strings.Contains(msg, broken) || !strings.Contains(msg, "500") {
		t.Errorf("recorded error %q should name the company and the failure", msg)
	}
}

// AC3 (edge): when every company fails, the run is "failed", not "partial".
func TestRunAllFailedIsFailed(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	e := newEnv(t)
	defer e.pool.Close()
	var tracker [2]atomic.Int32
	lever := newFake("lever", "lever.test", &tracker)
	lever.fail[e.add("lever", "Down1", true, false)] = true
	lever.fail[e.add("lever", "Down2", true, false)] = true

	res, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": lever},
		collector.Config{Concurrency: 2}, quiet())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Outcome != "failed" || res.Failed != 2 {
		t.Errorf("result = %+v, want failed, 2 failed", res)
	}
}

// AC4 (in process): cancelling the context stops in-flight fetches promptly, keeps the
// companies that finished, writes nothing for the rest, and records "cancelled".
func TestRunCancelStopsPromptlyAndLeavesNothingHalfWritten(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	e := newEnv(t)
	defer e.pool.Close()

	var tracker [2]atomic.Int32
	lever := newFake("lever", "lever.test", &tracker)
	done := e.add("lever", "Done", true, false)
	stuck := []string{e.add("lever", "Stuck1", true, false), e.add("lever", "Stuck2", true, false)}
	for _, s := range stuck {
		lever.block[s] = true
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		// Cancel once all three fetches have started (the stuck ones are now in flight).
		for len(lever.fetched()) < 3 {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond) // let the finished company commit
		cancel()
	}()
	start := time.Now()
	res, err := collector.Run(ctx, e.pool, collector.Registry{"lever": lever},
		collector.Config{Concurrency: 3}, quiet())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("Run took %v after cancellation, want prompt", d)
	}
	if res.Outcome != "cancelled" {
		t.Errorf("outcome = %q, want cancelled", res.Outcome)
	}
	if e.postings(done) != 1 {
		t.Errorf("finished company has %d postings, want 1 (kept)", e.postings(done))
	}
	for _, s := range stuck {
		if e.postings(s) != 0 {
			t.Errorf("cancelled company %s has %d postings, want 0", s, e.postings(s))
		}
	}
	if r := e.run(res.RunID); r.outcome != "cancelled" || !r.ended {
		t.Errorf("collector_runs row = %+v, want cancelled and ended", r)
	}
}

// AC5: a board type with no adapter is skipped, with a warning naming the company.
func TestRunSkipsBoardsWithoutAdapter(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	e := newEnv(t)
	defer e.pool.Close()
	var tracker [2]atomic.Int32
	lever := newFake("lever", "lever.test", &tracker)
	e.add("lever", "Fine", true, false)
	e.add("workable", "NoAdapter", true, false)

	var logs bytes.Buffer
	res, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": lever},
		collector.Config{Concurrency: 2}, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Skipped != 1 || res.Attempted != 1 || res.Outcome != "ok" {
		t.Errorf("result = %+v, want 1 skipped, 1 attempted, ok", res)
	}
	var warned bool
	for _, l := range strings.Split(logs.String(), "\n") {
		if strings.Contains(l, `"level":"WARN"`) && strings.Contains(l, "NoAdapter") && strings.Contains(l, "workable") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no warning naming NoAdapter and its board type:\n%s", logs.String())
	}
	if r := e.run(res.RunID); r.skipped != 1 {
		t.Errorf("collector_runs.companies_skipped = %d, want 1", r.skipped)
	}
}

// Excluded or inactive companies are never fetched.
func TestRunNeverFetchesExcludedOrInactive(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	e := newEnv(t)
	defer e.pool.Close()
	var tracker [2]atomic.Int32
	lever := newFake("lever", "lever.test", &tracker)
	want := e.add("lever", "Watched", true, false)
	excluded := e.add("lever", "Excluded", true, true)
	inactive := e.add("lever", "Inactive", false, false)
	excludedInactive := e.add("lever", "Both", false, true)

	if _, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": lever},
		collector.Config{Concurrency: 2}, quiet()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := lever.slugs()
	if !got[want] {
		t.Errorf("active company %s was not fetched", want)
	}
	for _, s := range []string{excluded, inactive, excludedInactive} {
		if got[s] {
			t.Errorf("company %s was fetched, but it is excluded or inactive", s)
		}
	}
}

// A run with nothing to collect still records itself.
func TestRunWithNoCompaniesIsOK(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	e := newEnv(t)
	defer e.pool.Close()
	res, err := collector.Run(t.Context(), e.pool, collector.Registry{}, collector.Config{Concurrency: 1}, quiet())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Outcome != "ok" || res.Attempted != 0 {
		t.Errorf("result = %+v, want ok with nothing attempted", res)
	}
	if r := e.run(res.RunID); !r.ended || r.outcome != "ok" {
		t.Errorf("collector_runs row = %+v, want ok and ended", r)
	}
}

// M1-07: collector_runs records postings whose payload changed, next to those seen
// unchanged.
func TestRunRecordsChangedPostings(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Changes", true, false)
	a, b := slug+"-a", slug+"-b"
	src := rawsSource{
		{Source: "lever", SourceID: a, URL: "https://example.test/a", Title: "A", Payload: []byte(`{"id":"a","v":1}`)},
		{Source: "lever", SourceID: b, URL: "https://example.test/b", Title: "B", Payload: []byte(`{"id":"b","v":1}`)},
	}
	if _, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": src}, collector.Config{Concurrency: 1}, quiet()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	src[1].Payload = []byte(`{"id":"b","v":2}`)
	res, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": src}, collector.Config{Concurrency: 1}, quiet())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if res.New != 0 || res.Changed != 1 || res.Seen != 1 {
		t.Errorf("result = %+v, want 0 new, 1 changed, 1 seen", res)
	}
	var changed, seen int
	if err := e.pool.QueryRow(t.Context(),
		`SELECT postings_changed, postings_seen FROM collector_runs WHERE id = $1`, res.RunID).Scan(&changed, &seen); err != nil {
		t.Fatalf("reading run: %v", err)
	}
	if changed != 1 || seen != 1 {
		t.Errorf("collector_runs postings_changed=%d postings_seen=%d, want 1 and 1", changed, seen)
	}
}
