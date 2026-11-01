//go:build integration

package collector_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/store/storetest"
)

type jobRow struct {
	title, workplace string
	version          int
	hashMatches      bool
	normalisedAt     time.Time
}

// jobOf returns the jobs row of a lever posting, and whether it exists.
func jobOf(t *testing.T, conn *pgx.Conn, sourceID string) (jobRow, bool) {
	t.Helper()
	var r jobRow
	err := conn.QueryRow(t.Context(),
		`SELECT j.title, j.workplace, j.normaliser_version, j.posting_hash = p.content_hash, j.normalised_at
		 FROM jobs j JOIN postings p ON p.id = j.posting_id
		 WHERE p.source = 'lever' AND p.source_posting_id = $1`, sourceID).
		Scan(&r.title, &r.workplace, &r.version, &r.hashMatches, &r.normalisedAt)
	if err == pgx.ErrNoRows { //nolint:errorlint // pgx returns the sentinel itself
		return r, false
	}
	if err != nil {
		t.Fatalf("reading job of %s: %v", sourceID, err)
	}
	return r, true
}

// M2-01 AC4: a new posting gets its job record in the same transaction; an unchanged
// one is not rewritten; a changed one is replaced.
func TestCollectWritesJobRecords(t *testing.T) {
	_, company, ids, body := board(t)
	var mu sync.Mutex
	current := body
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write([]byte(current)) // a failed write shows up as a fetch error
	}))
	t.Cleanup(srv.Close)

	var logs bytes.Buffer
	if _, err := once(t, srv.URL, company, &logs); err != nil {
		t.Fatalf("first Once: %v", err)
	}
	conn := storetest.Connect(t, storetest.AppURL)
	first := map[string]jobRow{}
	for i, want := range []struct{ title, workplace string }{
		{"Account Executive - Commercial, Spotify Technology", "hybrid"},
		{"AV Engineer - Platform", "onsite"},
	} {
		r, ok := jobOf(t, conn, ids[i])
		if !ok {
			t.Fatalf("new posting %s has no job record", ids[i])
		}
		if r.title != want.title || r.workplace != want.workplace || r.version != job.NormaliserVersion || !r.hashMatches {
			t.Errorf("job %s = %+v, want %q/%s, version %d, made from the current payload", ids[i], r, want.title, want.workplace, job.NormaliserVersion)
		}
		first[ids[i]] = r
	}

	mu.Lock()
	current = strings.Replace(body, `"text": "AV Engineer - Platform"`, `"text": "AV Engineer - Platform (Senior)"`, 1)
	mu.Unlock()
	if _, err := once(t, srv.URL, company, &logs); err != nil {
		t.Fatalf("second Once: %v", err)
	}
	unchanged, _ := jobOf(t, conn, ids[0])
	if !unchanged.normalisedAt.Equal(first[ids[0]].normalisedAt) {
		t.Errorf("unchanged posting's job was rewritten (normalised_at %v → %v)", first[ids[0]].normalisedAt, unchanged.normalisedAt)
	}
	changed, _ := jobOf(t, conn, ids[1])
	if changed.title != "AV Engineer - Platform (Senior)" || !changed.hashMatches || !changed.normalisedAt.After(first[ids[1]].normalisedAt) {
		t.Errorf("changed posting's job = %+v, want the new title, made from the new payload, rewritten", changed)
	}
}

// M2-01 AC4: the job record is in the posting's transaction, so a failure midway leaves
// no job rows either.
func TestCollectRollbackLeavesNoJobRecords(t *testing.T) {
	n := time.Now().UnixNano()
	ids := []string{fmt.Sprintf("jok-%d", n), fmt.Sprintf("jbad-%d", n)}
	src := rawsSource{
		{Source: "lever", SourceID: ids[0], URL: "https://example.test/ok", Title: "Ok", Payload: []byte(`{"text":"Ok"}`)},
		{Source: "lever", SourceID: ids[1], URL: "https://example.test/bad", Title: "Bad", Payload: []byte(`{not json`)},
	}
	storetest.Migrate(t, storetest.OwnerURL)
	conn := storetest.Connect(t, storetest.AppURL)
	if _, err := collector.Once(t.Context(), conn, src, fmt.Sprintf("jrollback-%d", n), slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("Once succeeded with an unstorable payload")
	}
	var jobs int
	if err := conn.QueryRow(t.Context(),
		`SELECT count(*) FROM jobs j JOIN postings p ON p.id = j.posting_id WHERE p.source_posting_id = ANY($1)`, ids).Scan(&jobs); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	if jobs != 0 {
		t.Errorf("%d job rows survived a rolled-back transaction, want 0", jobs)
	}
}

// ADR-0008: a posting that can't be normalised is still stored, gets no job record,
// and is counted in collector_runs.jobs_failed.
func TestRunCountsJobsFailed(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "JobsFailed", true, false)
	src := rawsSource{
		{Source: "lever", SourceID: slug + "-ok", URL: "https://example.test/ok", Title: "Ok", Payload: []byte(`{"text":"Ok","workplaceType":"remote"}`)},
		{Source: "lever", SourceID: slug + "-arr", URL: "https://example.test/arr", Title: "Array", Payload: []byte(`[1, 2]`)},
	}
	res, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": src}, collector.Config{Concurrency: 1}, quiet())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.JobsFailed != 1 || res.New != 2 || res.Outcome != "ok" {
		t.Errorf("result = %+v, want 2 new, 1 job failed, ok", res)
	}
	var failed, jobs int
	if err := e.pool.QueryRow(t.Context(),
		`SELECT (SELECT jobs_failed FROM collector_runs WHERE id = $1),
		        (SELECT count(*) FROM jobs j JOIN postings p ON p.id = j.posting_id WHERE p.source_posting_id LIKE $2 || '%')`,
		res.RunID, slug).Scan(&failed, &jobs); err != nil {
		t.Fatalf("reading run: %v", err)
	}
	if failed != 1 || jobs != 1 {
		t.Errorf("collector_runs.jobs_failed=%d, job rows=%d; want 1 and 1", failed, jobs)
	}
}

// M2-01 AC5: `collector normalise` (re)writes every posting whose record is missing,
// made by an older normaliser version, or made from an older payload, in batches; it
// leaves current ones alone and counts failures.
func TestNormaliseStoredBackfills(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Backfill", true, false)
	src := rawsSource{
		{Source: "lever", SourceID: slug + "-a", URL: "https://example.test/a", Title: "A", Payload: []byte(`{"text":"A","workplaceType":"remote"}`)},
		{Source: "lever", SourceID: slug + "-b", URL: "https://example.test/b", Title: "B", Payload: []byte(`{"text":"B"}`)},
		{Source: "lever", SourceID: slug + "-c", URL: "https://example.test/c", Title: "C", Payload: []byte(`{"text":"C"}`)},
		{Source: "lever", SourceID: slug + "-d", URL: "https://example.test/d", Title: "D", Payload: []byte(`{"text":"D"}`)},
		{Source: "lever", SourceID: slug + "-bad", URL: "https://example.test/x", Title: "X", Payload: []byte(`"just a string"`)},
	}
	if _, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": src}, collector.Config{Concurrency: 1}, quiet()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// a: delete its record (missing). b: an older version. c: made from an older payload.
	// d: current, must not be rewritten.
	for stmt, id := range map[string]string{
		`DELETE FROM jobs WHERE posting_id = (SELECT id FROM postings WHERE source_posting_id = $1)`:                                        slug + "-a",
		`UPDATE jobs SET normaliser_version = 0, title = 'stale' WHERE posting_id = (SELECT id FROM postings WHERE source_posting_id = $1)`: slug + "-b",
		`UPDATE jobs SET posting_hash = '\x00', title = 'stale' WHERE posting_id = (SELECT id FROM postings WHERE source_posting_id = $1)`:  slug + "-c",
	} {
		if _, err := e.pool.Exec(t.Context(), stmt, id); err != nil {
			t.Fatalf("making %s stale: %v", id, err)
		}
	}
	var dBefore time.Time
	if err := e.pool.QueryRow(t.Context(), `SELECT normalised_at FROM jobs WHERE posting_id = (SELECT id FROM postings WHERE source_posting_id = $1)`, slug+"-d").Scan(&dBefore); err != nil {
		t.Fatalf("reading d: %v", err)
	}

	var logs bytes.Buffer
	res, err := collector.NormaliseStored(t.Context(), e.pool, 2, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatalf("NormaliseStored: %v", err)
	}
	if res.Written < 3 || res.Failed < 1 || res.Batches < 2 {
		t.Errorf("result = %+v, want >= 3 written, >= 1 failed, >= 2 batches of 2", res)
	}
	rows, err := e.pool.Query(t.Context(),
		`SELECT p.source_posting_id, j.title, j.normaliser_version, j.posting_hash = p.content_hash, j.normalised_at
		 FROM postings p JOIN jobs j ON j.posting_id = p.id WHERE p.source_posting_id LIKE $1 || '%'`, slug)
	if err != nil {
		t.Fatalf("reading jobs: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var id, title string
		var version int
		var current bool
		var at time.Time
		if err := rows.Scan(&id, &title, &version, &current, &at); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		got[id] = true
		if title == "stale" || version != job.NormaliserVersion || !current {
			t.Errorf("%s still stale after normalise: title=%q version=%d current=%v", id, title, version, current)
		}
		if id == slug+"-d" && !at.Equal(dBefore) {
			t.Errorf("current record d was rewritten")
		}
	}
	for _, s := range []string{"-a", "-b", "-c", "-d"} {
		if !got[slug+s] {
			t.Errorf("posting %s has no job record after normalise", slug+s)
		}
	}
	if got[slug+"-bad"] {
		t.Error("an unnormalisable posting got a job record")
	}
	if !strings.Contains(logs.String(), `"msg":"normalise finished"`) || !strings.Contains(logs.String(), `"failed"`) {
		t.Errorf("no summary with a failed count:\n%s", logs.String())
	}
}
