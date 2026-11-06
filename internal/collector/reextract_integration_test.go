//go:build integration

package collector_test

import (
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/queue"
)

// extracted stores an extraction of the posting's current content with this prompt
// version, as the Python worker does.
func (e *env) extracted(sourceID string, version int) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.t.Context(), `
		INSERT INTO extractions (posting_id, content_hash, prompt_version, model, data)
		SELECT id, content_hash, $2, 'claude-haiku-5-5', '{}' FROM postings WHERE source_posting_id = $1`,
		sourceID, version); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.t.Context(), sql, args...); err != nil {
		e.t.Fatal(err)
	}
}

// M3-04 AC7: --reextract re-queues exactly the shown postings whose extraction used an
// older prompt version, never hidden jobs; --dry-run only counts and estimates.
func TestReextract(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Reextract", true, false)
	old, current, none, hidden := slug+"-old", slug+"-cur", slug+"-none", slug+"-hid"
	e.collect(collectorRegistry(rawsSource{
		leverPosting(old, "Backend Engineer", "Remote", "remote"),
		leverPosting(current, "Backend Engineer II", "Remote", "remote"),
		leverPosting(none, "Backend Engineer III", "Remote", "remote"),
		leverPosting(hidden, "Account Executive", "Remote", "remote"),
	}))
	p := backendOnly(t)
	e.reconcile(p)
	e.exec(`UPDATE work_items SET state = 'done' WHERE posting_id IN (SELECT id FROM postings WHERE source_posting_id LIKE $1)`, slug+"%")
	e.extracted(old, 1)
	e.extracted(current, 1)
	e.extracted(current, 2)
	e.extracted(hidden, 1)
	e.exec(`INSERT INTO agent_prompts (agent, version) VALUES ('extract', 2)
		ON CONFLICT (agent) DO UPDATE SET version = 2`)
	e.exec(`INSERT INTO model_calls (agent, prompt_file, prompt_version, model, cost_usd, latency_ms, outcome)
		VALUES ('extract', 'extract/v1.md', 1, 'claude-haiku-5-5', 0.002, 1, 'ok')`)

	dry, err := collector.Reextract(t.Context(), e.pool, p, true)
	if err != nil {
		t.Fatal(err)
	}
	if dry.N != 1 || dry.Version != 2 || !dry.CostKnown || dry.EstimatedUSD <= 0 {
		t.Errorf("dry run = %+v, want 1 posting at v2 with a cost estimate", dry)
	}
	if got := e.items(old)[old]; got.state != "done" {
		t.Errorf("dry run changed %s to %+v", old, got)
	}

	res, err := collector.Reextract(t.Context(), e.pool, p, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.N != 1 {
		t.Errorf("re-queued %d, want 1", res.N)
	}
	got := e.items(old, current, none, hidden)
	if got[old] != (item{"pending", 0}) {
		t.Errorf("older-version item = %+v, want pending, 0 attempts", got[old])
	}
	if got[current].state != "done" || got[none].state != "done" {
		t.Errorf("current/none items changed: %+v %+v", got[current], got[none])
	}
	if _, ok := got[hidden]; ok {
		t.Error("a hidden job got a work item")
	}
}

// M3-04 AC7: before any worker ran, there is no current version to compare against.
func TestReextractNeedsTheWorkersVersion(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	e.exec(`DELETE FROM agent_prompts`)
	if _, err := collector.Reextract(t.Context(), e.pool, backendOnly(t), true); err == nil {
		t.Error("want an error when no worker has recorded its prompt version")
	}
}

// M3-04 AC8: retry --failed puts failed items of that kind back to pending with 0
// attempts and no error; nothing else changes.
func TestRetryFailed(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Retry", true, false)
	a, b := slug+"-a", slug+"-b"
	e.collect(collectorRegistry(rawsSource{
		leverPosting(a, "Backend Engineer", "Remote", "remote"),
		leverPosting(b, "Backend Engineer II", "Remote", "remote"),
	}))
	e.reconcile(backendOnly(t))
	e.exec(`UPDATE work_items SET state = 'done' WHERE state IN ('pending', 'failed')`)
	e.exec(`UPDATE work_items SET state = 'failed', attempts = 5, last_error = 'InvalidOutput: x'
		WHERE posting_id = (SELECT id FROM postings WHERE source_posting_id = $1)`, a)

	n, err := queue.RetryFailed(t.Context(), e.pool, "extract")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("retried %d, want 1", n)
	}
	got := e.items(a, b)
	if got[a] != (item{"pending", 0}) || got[b].state != "done" {
		t.Errorf("items = %+v", got)
	}
	var lastErr *string
	if err := e.pool.QueryRow(t.Context(), `SELECT last_error FROM work_items w JOIN postings p ON p.id = w.posting_id
		WHERE p.source_posting_id = $1`, a).Scan(&lastErr); err != nil || lastErr != nil {
		t.Errorf("last_error = %v (%v), want NULL", lastErr, err)
	}
}

// M3-06 (ADR-0021): the worker's own reason items retry by kind too, and nothing else moves.
func TestRetryFailedReason(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "RetryReason", true, false)
	a := slug + "-a"
	e.collect(collectorRegistry(rawsSource{leverPosting(a, "Backend Engineer", "Remote", "remote")}))
	e.reconcile(backendOnly(t))
	e.exec(`UPDATE work_items SET state = 'failed', last_error = 'InvalidOutput: x'
		WHERE posting_id = (SELECT id FROM postings WHERE source_posting_id = $1)`, a)
	e.exec(`INSERT INTO work_items (posting_id, kind, content_hash, state, last_error)
		SELECT id, 'reason', content_hash, 'failed', 'InvalidOutput: x' FROM postings WHERE source_posting_id = $1`, a)

	n, err := queue.RetryFailed(t.Context(), e.pool, "reason")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("retried %d, want 1", n)
	}
	var extractState, reasonState string
	if err := e.pool.QueryRow(t.Context(), `SELECT
		max(state) FILTER (WHERE kind = 'extract'), max(state) FILTER (WHERE kind = 'reason')
		FROM work_items w JOIN postings p ON p.id = w.posting_id WHERE p.source_posting_id = $1`, a).
		Scan(&extractState, &reasonState); err != nil {
		t.Fatal(err)
	}
	if extractState != "failed" || reasonState != "pending" {
		t.Errorf("extract %s, reason %s; want failed, pending", extractState, reasonState)
	}
}
