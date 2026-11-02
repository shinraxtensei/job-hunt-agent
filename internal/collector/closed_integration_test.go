//go:build integration

package collector_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/store/storetest"
)

func (e *env) closed(sourceID string) bool {
	e.t.Helper()
	var closed bool
	if err := e.pool.QueryRow(e.t.Context(),
		`SELECT closed_at IS NOT NULL FROM postings WHERE source_posting_id = $1`, sourceID).Scan(&closed); err != nil {
		e.t.Fatalf("reading posting %s: %v", sourceID, err)
	}
	return closed
}

func (e *env) runCounts(id int64) (closed, reopened int) {
	e.t.Helper()
	if err := e.pool.QueryRow(e.t.Context(),
		`SELECT postings_closed, postings_reopened FROM collector_runs WHERE id = $1`, id).Scan(&closed, &reopened); err != nil {
		e.t.Fatalf("reading run %d: %v", id, err)
	}
	return closed, reopened
}

// postings returns n lever postings named <prefix>-0 .. <prefix>-(n-1).
func postings(prefix string, n int) rawsSource {
	var s rawsSource
	for i := range n {
		id := fmt.Sprintf("%s-%d", prefix, i)
		s = append(s, leverPosting(id, "Engineer "+id, "Berlin", "hybrid"))
	}
	return s
}

func (e *env) collect(reg collector.Registry) collector.RunResult {
	e.t.Helper()
	res, err := collector.Run(e.t.Context(), e.pool, reg, collector.Config{Concurrency: 2}, quiet())
	if err != nil {
		e.t.Fatalf("Run: %v", err)
	}
	return res
}

// M2-04 AC1: after a successful fetch, stored postings missing from the board are closed,
// in the company's transaction, and counted in collector_runs.postings_closed.
func TestRunClosesMissingPostings(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Closes", true, false)
	all := postings(slug, 3)
	e.collect(collector.Registry{"lever": all})

	res := e.collect(collector.Registry{"lever": all[:2]})
	if res.Closed != 1 || res.Outcome != "ok" {
		t.Errorf("result = %+v, want 1 closed, ok", res)
	}
	if e.closed(slug+"-0") || e.closed(slug+"-1") || !e.closed(slug+"-2") {
		t.Errorf("closed: -0 %v, -1 %v, -2 %v; want only -2 (missing from the board)",
			e.closed(slug+"-0"), e.closed(slug+"-1"), e.closed(slug+"-2"))
	}
	if c, _ := e.runCounts(res.RunID); c != 1 {
		t.Errorf("collector_runs.postings_closed = %d, want 1", c)
	}
}

// M2-04 AC1: closing is in the company's transaction: if storing fails, nothing closes.
func TestRunClosingRollsBackWithTheCompany(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "CloseRollback", true, false)
	all := postings(slug, 2)
	e.collect(collector.Registry{"lever": all})

	bad := job.Raw{Source: "lever", SourceID: slug + "-bad", URL: "https://example.test/bad", Title: "Bad", Payload: []byte(`{not json`)}
	res := e.collect(collector.Registry{"lever": rawsSource{all[0], bad}})
	if res.Failed != 1 {
		t.Fatalf("result = %+v, want the company to fail", res)
	}
	if e.closed(slug + "-1") {
		t.Error("a posting was closed although the company's transaction failed")
	}
}

// M2-04 AC2: a failed, skipped or cancelled fetch closes nothing.
func TestRunNeverClosesWithoutASuccessfulFetch(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	failSlug := e.add("lever", "FailFetch", true, false)
	skipSlug := e.add("greenhouse", "SkipFetch", true, false)
	e.collect(collector.Registry{"lever": postings(failSlug, 2), "greenhouse": greenhouseRaws(postings(skipSlug, 2))})

	t.Run("failed", func(t *testing.T) {
		var tracker [2]atomic.Int32
		failing := newFake("lever", "lever.test", &tracker)
		failing.fail[failSlug] = true
		e.collect(collector.Registry{"lever": failing, "greenhouse": greenhouseRaws(postings(skipSlug, 2))})
		if e.closed(failSlug+"-0") || e.closed(failSlug+"-1") {
			t.Error("a failed fetch closed postings")
		}
	})
	t.Run("skipped", func(t *testing.T) {
		e.collect(collector.Registry{"lever": postings(failSlug, 2)}) // no greenhouse adapter
		if e.closed(skipSlug+"-0") || e.closed(skipSlug+"-1") {
			t.Error("a skipped board type closed postings")
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		var tracker [2]atomic.Int32
		stuck := newFake("lever", "lever.test", &tracker)
		stuck.block[failSlug] = true
		ctx, cancel := context.WithCancel(t.Context())
		go func() { time.Sleep(200 * time.Millisecond); cancel() }()
		if _, err := collector.Run(ctx, e.pool, collector.Registry{"lever": stuck}, collector.Config{Concurrency: 1}, quiet()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if e.closed(failSlug+"-0") || e.closed(failSlug+"-1") {
			t.Error("a cancelled fetch closed postings")
		}
	})
}

// greenhouseRaws relabels lever test postings as greenhouse ones, for a second board type.
type greenhouseRaws rawsSource

func (greenhouseRaws) Name() string { return "greenhouse" }
func (greenhouseRaws) Host() string { return "greenhouse.test" }
func (g greenhouseRaws) Fetch(ctx context.Context, slug string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	raws, n, err := rawsSource(g).Fetch(ctx, slug, limit)
	for i := range raws {
		raws[i].Source = "greenhouse"
	}
	return raws, n, err
}

// M2-04 AC3: a closed posting that comes back is reopened, counted and logged by ID.
func TestRunReopensPosting(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Reopens", true, false)
	all := postings(slug, 2)
	e.collect(collector.Registry{"lever": all})
	e.collect(collector.Registry{"lever": all[:1]})
	if !e.closed(slug + "-1") {
		t.Fatal("setup: -1 was not closed")
	}

	var logs bytes.Buffer
	res, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": all}, collector.Config{Concurrency: 1},
		slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if e.closed(slug+"-1") || res.Reopened != 1 || res.Closed != 0 {
		t.Errorf("after reappearing: closed=%v, result %+v; want open, 1 reopened, 0 closed", e.closed(slug+"-1"), res)
	}
	if _, r := e.runCounts(res.RunID); r != 1 {
		t.Errorf("collector_runs.postings_reopened = %d, want 1", r)
	}
	if !strings.Contains(logs.String(), `"msg":"posting reopened"`) || !strings.Contains(logs.String(), slug+"-1") {
		t.Errorf("no 'posting reopened' log naming %s-1:\n%s", slug, logs.String())
	}
}

// M2-04 AC4: the empty-board guard, at its boundary, and how it meets the ratio guard. An
// empty board closes a company's open postings only when that is fewer than
// CloseRatioGuardMin (9 here); exactly 10 is refused by the ratio guard (10 of 10 > 50%);
// more than EmptyBoardGuard (11) is refused by the empty-board guard itself.
func TestEmptyBoardGuard(t *testing.T) {
	if collector.EmptyBoardGuard != 10 {
		t.Fatalf("EmptyBoardGuard = %d, the story says 10", collector.EmptyBoardGuard)
	}
	for open, wantErr := range map[int]string{
		9:  "",
		10: "refusing to close 10 of 10 postings",
		11: "empty board, refusing to close 11 postings",
	} {
		t.Run(fmt.Sprintf("%d open", open), func(t *testing.T) {
			e := newEnv(t)
			defer e.pool.Close()
			slug := e.add("lever", fmt.Sprintf("Guard%d", open), true, false)
			e.collect(collector.Registry{"lever": postings(slug, open)})
			res := e.collect(collector.Registry{"lever": rawsSource{}}) // empty board
			e.checkGuard(res, slug, open, open, wantErr)
		})
	}
}

// M2-04 (ratio guard): a fetch that would close more than half of a company's open
// postings, and at least CloseRatioGuardMin of them, closes nothing. Both thresholds are
// tested on both sides.
func TestCloseRatioGuard(t *testing.T) {
	if collector.CloseRatioGuardMin != 10 {
		t.Fatalf("CloseRatioGuardMin = %d, want 10", collector.CloseRatioGuardMin)
	}
	for _, c := range []struct {
		name          string
		open, missing int
		wantErr       string
	}{
		{"exactly half closes", 20, 10, ""},
		{"over half refused", 20, 11, "refusing to close 11 of 20 postings"},
		{"over half but under 10 closes", 12, 9, ""},
		{"over half and 10 refused", 15, 10, "refusing to close 10 of 15 postings"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			defer e.pool.Close()
			slug := e.add("lever", fmt.Sprintf("Ratio%d-%d", c.open, c.missing), true, false)
			all := postings(slug, c.open)
			e.collect(collector.Registry{"lever": all})
			res := e.collect(collector.Registry{"lever": all[c.missing:]}) // the first `missing` are gone
			e.checkGuard(res, slug, c.open, c.missing, c.wantErr)
		})
	}
}

// checkGuard asserts that either the missing postings (the first `missing` of `open`) were
// all closed, or, when wantErr is set, nothing was closed and the company failed with it.
func (e *env) checkGuard(res collector.RunResult, slug string, open, missing int, wantErr string) {
	e.t.Helper()
	closedN := 0
	for i := range open {
		if e.closed(fmt.Sprintf("%s-%d", slug, i)) {
			closedN++
		}
	}
	if wantErr == "" {
		if closedN != missing || res.Closed != missing || res.Failed != 0 {
			e.t.Errorf("%d missing of %d: %d closed, result %+v; want %d closed, no failure", missing, open, closedN, res, missing)
		}
		return
	}
	if closedN != 0 || res.Closed != 0 || res.Failed != 1 {
		e.t.Errorf("%d missing of %d: %d closed, result %+v; want nothing closed and the company failed", missing, open, closedN, res)
	}
	var msg string
	if err := e.pool.QueryRow(e.t.Context(), `SELECT error FROM collector_run_errors WHERE run_id = $1`, res.RunID).Scan(&msg); err != nil {
		e.t.Fatalf("reading run error: %v", err)
	}
	if !strings.Contains(msg, wantErr) {
		e.t.Errorf("recorded error %q, want it to contain %q", msg, wantErr)
	}
}

// ADR-0010: a stored posting that the adapter now skips (here: a NUL in its text, from the
// Lever NUL golden file) is still on the board, so it is not closed.
func TestSkippedPostingWithReadableIDIsNotClosed(t *testing.T) {
	b, err := os.ReadFile("../source/lever/testdata/nul_posting.json")
	if err != nil {
		t.Fatal(err)
	}
	n := time.Now().UnixNano()
	body := string(b)
	nulID := fmt.Sprintf("nul-%d", n)
	for i, id := range append(goldenIDs, "00000000-nul0-4000-8000-000000000000") {
		repl := fmt.Sprintf("%s-%d", id, n)
		if i == len(goldenIDs) {
			repl = nulID
		}
		body = strings.ReplaceAll(body, id, repl)
	}
	clean := strings.ReplaceAll(body, `\u0000`, "")
	if clean == body {
		t.Fatal("fixture has no NUL escape; update the test")
	}
	current := clean
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write([]byte(current)) // a failed write shows up as a fetch error
	}))
	t.Cleanup(srv.Close)
	company := fmt.Sprintf("nulclose-%d", n)

	var logs bytes.Buffer
	if res, err := once(t, srv.URL, company, &logs); err != nil || res.New != 3 {
		t.Fatalf("first Once (clean board) = %+v, %v; want 3 new", res, err)
	}
	mu.Lock()
	current = body // now the third posting carries a NUL: the adapter skips it, by ID
	mu.Unlock()
	res, err := once(t, srv.URL, company, &logs)
	if err != nil {
		t.Fatalf("second Once: %v", err)
	}
	if res.Skipped != 1 || res.Closed != 0 {
		t.Errorf("result = %+v, want 1 skipped and 0 closed", res)
	}
	conn := storetest.Connect(t, storetest.AppURL)
	var closed bool
	if err := conn.QueryRow(t.Context(), `SELECT closed_at IS NOT NULL FROM postings WHERE source_posting_id = $1`, nulID).Scan(&closed); err != nil {
		t.Fatalf("reading NUL posting: %v", err)
	}
	if closed {
		t.Error("a posting skipped with a readable ID was closed")
	}
}

// M2-04 AC6: job_groups prefers the open member seen first as canonical; a group is closed
// only when all its members are.
func TestJobGroupsPreferOpenMembers(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "OpenCanonical", true, false)
	berlin := leverPosting(slug+"-berlin", "Platform Engineer - Berlin", "Berlin", "hybrid")
	remote := leverPosting(slug+"-remote", "Platform Engineer (Remote)", "Remote", "remote")
	e.collect(collector.Registry{"lever": rawsSource{berlin}})
	e.collect(collector.Registry{"lever": rawsSource{berlin, remote}})

	closedGroup := func() (bool, int64) {
		t.Helper()
		var closed bool
		var canonical int64
		if err := e.pool.QueryRow(t.Context(),
			`SELECT g.closed, g.canonical_posting_id FROM job_groups g JOIN companies c ON c.id = g.company_id
			 WHERE c.board_slug = $1 AND g.title_key = 'platform engineer'`, slug).Scan(&closed, &canonical); err != nil {
			t.Fatalf("reading group: %v", err)
		}
		return closed, canonical
	}
	if closed, canonical := closedGroup(); closed || canonical != postingID(t, e, slug+"-berlin") {
		t.Fatalf("both open: closed=%v canonical=%d; want open, Berlin (seen first)", closed, canonical)
	}

	e.collect(collector.Registry{"lever": rawsSource{remote}}) // Berlin taken down
	if closed, canonical := closedGroup(); closed || canonical != postingID(t, e, slug+"-remote") {
		t.Errorf("Berlin closed: closed=%v canonical=%d; want open, Remote (the open member)", closed, canonical)
	}

	e.collect(collector.Registry{"lever": rawsSource{}}) // both gone (2 open ≤ guard)
	if closed, _ := closedGroup(); !closed {
		t.Error("all members closed, but the group is not")
	}
}
