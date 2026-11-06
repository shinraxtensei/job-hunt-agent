//go:build integration

package collector_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/preset"
	"github.com/shinraxtensei/job-hunter/internal/queue"
)

// backendOnly shows backend jobs and hides the rest (a hidden job must never be queued).
func backendOnly(t *testing.T) preset.Preset {
	t.Helper()
	p, err := preset.Parse([]byte("name: test\nrole_family: [backend]\n"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type item struct {
	state    string
	attempts int
}

// items returns the work items of the postings with these source ids, by source id.
func (e *env) items(sourceIDs ...string) map[string]item {
	e.t.Helper()
	rows, err := e.pool.Query(e.t.Context(), `
		SELECT p.source_posting_id, w.state, w.attempts FROM work_items w JOIN postings p ON p.id = w.posting_id
		WHERE w.kind = 'extract' AND p.source_posting_id = ANY($1)`, sourceIDs)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]item{}
	for rows.Next() {
		var id string
		var it item
		if err := rows.Scan(&id, &it.state, &it.attempts); err != nil {
			e.t.Fatal(err)
		}
		out[id] = it
	}
	return out
}

func (e *env) reconcile(p preset.Preset) int {
	e.t.Helper()
	n, err := collector.Reconcile(e.t.Context(), e.pool, p)
	if err != nil {
		e.t.Fatalf("Reconcile: %v", err)
	}
	return n
}

func (e *env) countItems() int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.t.Context(), `SELECT count(*) FROM work_items`).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// M3-02 AC1: shown groups get one extract item for their canonical posting; hidden
// groups none; a second run enqueues nothing more.
func TestReconcileEnqueuesShownGroupsOnce(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Reconcile", true, false)
	be, sales := slug+"-be", slug+"-sales"
	e.collect(collectorRegistry(rawsSource{
		leverPosting(be, "Backend Engineer", "Remote", "remote"),
		leverPosting(sales, "Account Executive", "Remote", "remote"),
	}))
	p := backendOnly(t)
	e.reconcile(p)
	got := e.items(be, sales)
	if got[be] != (item{"pending", 0}) {
		t.Errorf("backend item = %+v, want pending, 0 attempts", got[be])
	}
	if _, ok := got[sales]; ok {
		t.Error("a job the preset hides was enqueued")
	}
	if n := e.reconcile(p); n != 0 {
		t.Errorf("second reconcile enqueued %d, want 0", n)
	}
}

// M3-02 AC1: a changed posting (new content_hash) is queued again, even if its old
// content was done; an unchanged done one is not.
func TestReconcileRequeuesChangedPostings(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Requeue", true, false)
	a, b := slug+"-a", slug+"-b"
	e.collect(collectorRegistry(rawsSource{
		leverPosting(a, "Backend Engineer A", "Remote", "remote"),
		leverPosting(b, "Backend Engineer B", "Remote", "remote"),
	}))
	p := backendOnly(t)
	e.reconcile(p)
	if _, err := e.pool.Exec(t.Context(), `
		UPDATE work_items SET state = 'done', attempts = 1 FROM postings p
		WHERE p.id = work_items.posting_id AND p.source_posting_id = ANY($1)`, []string{a, b}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(t.Context(),
		`UPDATE postings SET content_hash = sha256('changed'::bytea) WHERE source_posting_id = $1`, a); err != nil {
		t.Fatal(err)
	}
	if n := e.reconcile(p); n != 1 {
		t.Errorf("reconcile enqueued %d, want 1 (the changed posting)", n)
	}
	got := e.items(a, b)
	if got[a] != (item{"pending", 0}) || got[b] != (item{"done", 1}) {
		t.Errorf("items = %+v, want changed pending/0 and unchanged done/1", got)
	}
}

// M3-02 AC1: reconciles running at the same time enqueue each group once between them.
func TestReconcileConcurrent(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Concurrent", true, false)
	e.collect(collectorRegistry(postingsTitled(slug, "Backend Engineer", 20)))
	p := backendOnly(t)
	before := e.countItems()
	var wg sync.WaitGroup
	counts := make([]int, 4)
	errs := make([]error, 4)
	for i := range counts {
		wg.Go(func() { counts[i], errs[i] = collector.Reconcile(t.Context(), e.pool, p) })
	}
	wg.Wait()
	sum := 0
	for i := range counts {
		if errs[i] != nil {
			t.Fatalf("Reconcile %d: %v", i, errs[i])
		}
		sum += counts[i]
	}
	if added := e.countItems() - before; sum != added || added < 20 {
		t.Errorf("reconciles reported %d enqueued, table grew by %d (want equal, >= 20)", sum, added)
	}
}

// M3-02 AC5: counts by state.
func TestQueueCounts(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Counts", true, false)
	e.collect(collectorRegistry(postingsTitled(slug, "Backend Engineer", 3)))
	e.reconcile(backendOnly(t))
	c, err := queue.Count(t.Context(), e.pool)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM work_items WHERE state = 'pending'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if c.Pending != n || c.Pending < 3 {
		t.Errorf("counts = %+v, want pending %d", c, n)
	}
}

func collectorRegistry(s rawsSource) collector.Registry { return collector.Registry{"lever": s} }

// postingsTitled returns n remote lever postings <prefix>-t-<i> titled "<title> <i>".
func postingsTitled(prefix, title string, n int) rawsSource {
	var s rawsSource
	for i := range n {
		id := prefix + "-t-" + strconv.Itoa(i)
		s = append(s, leverPosting(id, title+" "+strconv.Itoa(i), "Remote", "remote"))
	}
	return s
}

// M3-09 review (ADR-0023): reconcile stores the shown group's matched location on its
// canonical job, for the scorer. The group's first posting (Austin, on-site) fails the
// location rule; its remote EMEA posting matches, so that is the matched location.
func TestReconcileStoresMatchedLocation(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Matched", true, false)
	austin, emea := slug+"-austin", slug+"-emea"
	e.collect(collectorRegistry(rawsSource{leverPosting(austin, "Backend Engineer", "Austin, TX", "onsite")}))
	e.collect(collectorRegistry(rawsSource{
		leverPosting(austin, "Backend Engineer", "Austin, TX", "onsite"),
		leverPosting(emea, "Backend Engineer", "Remote - EMEA", "remote"),
	}))
	p, err := preset.Parse([]byte("name: t\nrole_family: [backend]\nlocation:\n  - {workplace: [remote], remote_scopes: [emea]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	e.reconcile(p)
	var got string
	if err := e.pool.QueryRow(t.Context(), `SELECT j.matched_location FROM jobs j JOIN postings po ON po.id = j.posting_id
		WHERE po.source_posting_id = $1`, austin).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "Remote - EMEA" {
		t.Errorf("matched_location = %q, want %q (the location that passed, not the first)", got, "Remote - EMEA")
	}
}
