//go:build integration

package collector_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/source/lever"
	"github.com/shinraxtensei/job-hunter/internal/store/storetest"
)

func TestMain(m *testing.M) { storetest.Main(m) }

const golden = "../source/lever/testdata/spotify_postings.json"

// The two posting IDs in the golden file.
var goldenIDs = []string{"2a55d6c9-c564-443a-be3e-a3851db5a93b", "785989a6-b219-43b1-92d9-750a67054f24"}

// board serves the golden Lever response with test-unique posting IDs (posting IDs are
// unique across the shared database) and returns the server, the company slug and the
// IDs and body it serves.
func board(t *testing.T) (srv *httptest.Server, company string, ids []string, body string) {
	t.Helper()
	b, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden file: %v", err)
	}
	n := time.Now().UnixNano()
	body = string(b)
	ids = make([]string, len(goldenIDs))
	for i, id := range goldenIDs {
		ids[i] = fmt.Sprintf("%s-%d", id, n)
		body = strings.ReplaceAll(body, id, ids[i])
	}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body)) // a failed write shows up as a fetch error
	}))
	t.Cleanup(srv.Close)
	return srv, fmt.Sprintf("co-%d", n), ids, body
}

func once(t *testing.T, baseURL, company string, logs *bytes.Buffer) (collector.Result, error) {
	t.Helper()
	storetest.Migrate(t, storetest.OwnerURL)
	conn := storetest.Connect(t, storetest.AppURL)
	log := slog.New(slog.NewJSONHandler(logs, nil))
	return collector.Once(t.Context(), conn, lever.New(baseURL), company, log)
}

// logLines decodes every log line, failing the test if any line is not JSON.
func logLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(logs.Bytes()))
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("log line is not JSON: %q", sc.Text())
		}
		lines = append(lines, m)
	}
	return lines
}

func summary(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	for _, l := range logLines(t, logs) {
		if l["msg"] == "collect finished" {
			return l
		}
	}
	t.Fatalf("no 'collect finished' log line in:\n%s", logs)
	return nil
}

// AC1: postings are inserted with the raw JSON stored, and the run logs new/existing.
func TestOnceInsertsPostingsWithRawJSON(t *testing.T) {
	srv, company, ids, body := board(t)
	var logs bytes.Buffer
	res, err := once(t, srv.URL, company, &logs)
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if res != (collector.Result{Fetched: 2, New: 2}) {
		t.Errorf("result = %+v, want 2 fetched, 2 new", res)
	}
	s := summary(t, &logs)
	if s["company"] != company || s["new"] != 2.0 || s["existing"] != 0.0 || s["fetched"] != 2.0 {
		t.Errorf("summary log = %v, want company=%s fetched=2 new=2 existing=0", s, company)
	}

	conn := storetest.Connect(t, storetest.AppURL)
	var elems []json.RawMessage
	if err := json.Unmarshal([]byte(body), &elems); err != nil {
		t.Fatalf("decoding golden: %v", err)
	}
	for i, id := range ids {
		var same bool
		err := conn.QueryRow(t.Context(),
			`SELECT p.raw_payload = $2::jsonb
			 FROM postings p JOIN companies c ON c.id = p.company_id
			 WHERE p.source = 'lever' AND p.source_posting_id = $1 AND c.board_slug = $3`,
			id, string(elems[i]), company).Scan(&same)
		if err != nil {
			t.Fatalf("reading posting %s: %v", id, err)
		}
		if !same {
			t.Errorf("posting %s: stored raw_payload differs from the JSON Lever sent", id)
		}
	}
}

// AC2: a second run inserts no duplicates and bumps last_checked_at only.
func TestOnceTwiceUpdatesLastCheckedWithoutDuplicates(t *testing.T) {
	srv, company, ids, _ := board(t)
	var logs bytes.Buffer
	if _, err := once(t, srv.URL, company, &logs); err != nil {
		t.Fatalf("first Once: %v", err)
	}
	conn := storetest.Connect(t, storetest.AppURL)
	type stamps struct{ first, checked time.Time }
	read := func() map[string]stamps {
		rows, err := conn.Query(t.Context(),
			`SELECT source_posting_id, first_seen_at, last_checked_at FROM postings
			 WHERE source = 'lever' AND source_posting_id = ANY($1)`, ids)
		if err != nil {
			t.Fatalf("reading postings: %v", err)
		}
		defer rows.Close()
		out := map[string]stamps{}
		for rows.Next() {
			var id string
			var s stamps
			if err := rows.Scan(&id, &s.first, &s.checked); err != nil {
				t.Fatalf("scanning: %v", err)
			}
			out[id] = s
		}
		return out
	}
	before := read()

	logs.Reset()
	res, err := once(t, srv.URL, company, &logs)
	if err != nil {
		t.Fatalf("second Once: %v", err)
	}
	if res != (collector.Result{Fetched: 2, Existing: 2}) {
		t.Errorf("second result = %+v, want 2 fetched, 2 existing", res)
	}
	if s := summary(t, &logs); s["new"] != 0.0 || s["existing"] != 2.0 {
		t.Errorf("second summary log = %v, want new=0 existing=2", s)
	}
	after := read()
	if len(after) != 2 {
		t.Fatalf("got %d rows for the two postings, want 2 (no duplicates)", len(after))
	}
	var companies int
	if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM companies WHERE board_type = 'lever' AND board_slug = $1`, company).Scan(&companies); err != nil {
		t.Fatalf("counting companies: %v", err)
	}
	if companies != 1 {
		t.Errorf("got %d company rows, want 1", companies)
	}
	for id, b := range before {
		a := after[id]
		if !a.checked.After(b.checked) {
			t.Errorf("posting %s: last_checked_at %v not after %v", id, a.checked, b.checked)
		}
		if !a.first.Equal(b.first) {
			t.Errorf("posting %s: first_seen_at changed from %v to %v", id, b.first, a.first)
		}
	}
}

// AC4: a 500 is an error naming the company, and nothing is written.
func TestOnceUpstream500WritesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	company := fmt.Sprintf("down-%d", time.Now().UnixNano())

	var logs bytes.Buffer
	_, err := once(t, srv.URL, company, &logs)
	if err == nil || !strings.Contains(err.Error(), company) {
		t.Fatalf("Once error = %v, want one naming %q", err, company)
	}
	conn := storetest.Connect(t, storetest.AppURL)
	var companies, postings int
	if err := conn.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM companies WHERE board_slug = $1),
		        (SELECT count(*) FROM postings p JOIN companies c ON c.id = p.company_id WHERE c.board_slug = $1)`,
		company).Scan(&companies, &postings); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if companies != 0 || postings != 0 {
		t.Errorf("after a 500: %d company rows and %d postings for %s, want none", companies, postings, company)
	}
}

// AC5: logs are JSON with counts and IDs only; no posting text, not even titles.
func TestOnceLogsCountsAndIDsNeverPostingText(t *testing.T) {
	srv, company, ids, _ := board(t)
	var logs bytes.Buffer
	if _, err := once(t, srv.URL, company, &logs); err != nil {
		t.Fatalf("Once: %v", err)
	}
	out := logs.String()
	for _, text := range []string{
		"Spotify Technology is our business-to-business", // descriptionPlain
		"Account Executive - Commercial",                 // a title
		"AV Engineer",                                    // the other title
		"The United States base range",                   // additionalPlain
		"<div>",                                          // any HTML body
	} {
		if strings.Contains(out, text) {
			t.Errorf("logs contain posting text %q:\n%s", text, out)
		}
	}
	logged := map[string]bool{}
	for _, l := range logLines(t, &logs) {
		if id, ok := l["posting_id"].(string); ok {
			logged[id] = true
		}
	}
	for _, id := range ids {
		if !logged[id] {
			t.Errorf("new posting %s is not logged by ID", id)
		}
	}
}

// rawsSource returns fixed records for any board, waiting on the limiter like a real
// adapter.
type rawsSource []job.Raw

func (rawsSource) Name() string { return "lever" }
func (rawsSource) Host() string { return "raws.test" }
func (s rawsSource) Fetch(ctx context.Context, _ string, limit *rate.Limiter) ([]job.Raw, int, error) {
	if err := limit.Wait(ctx); err != nil {
		return nil, 0, err
	}
	return s, 0, nil
}

// M0-04 AC4: a failure partway through storing rolls back everything, including the
// postings and the company row written before it. The second record's payload is not
// JSON, so Postgres rejects its insert after the first one succeeded. (Until M1-03 this
// used a NUL in a title; the adapter now skips those, see TestOnceSkipsNULPosting.)
func TestOnceRollsBackOnFailureMidway(t *testing.T) {
	n := time.Now().UnixNano()
	company := fmt.Sprintf("midway-%d", n)
	ids := []string{fmt.Sprintf("ok-%d", n), fmt.Sprintf("bad-%d", n)}
	src := rawsSource{
		{Source: "lever", SourceID: ids[0], URL: "https://example.test/ok", Title: "Ok", Payload: []byte(`{"id":"ok"}`)},
		{Source: "lever", SourceID: ids[1], URL: "https://example.test/bad", Title: "Bad", Payload: []byte(`{not json`)},
	}
	storetest.Migrate(t, storetest.OwnerURL)
	conn := storetest.Connect(t, storetest.AppURL)

	var logs bytes.Buffer
	_, err := collector.Once(t.Context(), conn, src, company, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err == nil || !strings.Contains(err.Error(), company) {
		t.Fatalf("Once error = %v, want one naming %q", err, company)
	}
	var companies, postings int
	if err := conn.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM companies WHERE board_slug = $1),
		        (SELECT count(*) FROM postings WHERE source = 'lever' AND source_posting_id = ANY($2))`,
		company, ids).Scan(&companies, &postings); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if companies != 0 || postings != 0 {
		t.Errorf("after a failure midway: %d company rows and %d postings, want none", companies, postings)
	}
	if strings.Contains(logs.String(), "new posting") {
		t.Errorf("logged new postings that were rolled back:\n%s", logs.String())
	}
}

// M1-03 AC6: a posting containing NUL is skipped and counted, and the rest of the
// company's postings are stored.
func TestOnceSkipsNULPosting(t *testing.T) {
	b, err := os.ReadFile("../source/lever/testdata/nul_posting.json")
	if err != nil {
		t.Fatal(err)
	}
	n := time.Now().UnixNano()
	body := string(b)
	ids := make([]string, len(goldenIDs))
	for i, id := range goldenIDs {
		ids[i] = fmt.Sprintf("%s-%d", id, n)
		body = strings.ReplaceAll(body, id, ids[i])
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body)) // a failed write shows up as a fetch error
	}))
	t.Cleanup(srv.Close)
	company := fmt.Sprintf("nul-%d", n)

	var logs bytes.Buffer
	res, err := once(t, srv.URL, company, &logs)
	if err != nil {
		t.Fatalf("Once: %v (one NUL posting must not fail the board)", err)
	}
	if res.Skipped != 1 || res.New != 2 {
		t.Errorf("result = %+v, want 2 new, 1 skipped", res)
	}
	if s := summary(t, &logs); s["skipped"] != 1.0 {
		t.Errorf("summary log = %v, want skipped=1", s)
	}
	conn := storetest.Connect(t, storetest.AppURL)
	var stored int
	if err := conn.QueryRow(t.Context(),
		`SELECT count(*) FROM postings WHERE source = 'lever' AND source_posting_id = ANY($1)`, ids).Scan(&stored); err != nil {
		t.Fatalf("counting postings: %v", err)
	}
	if stored != 2 {
		t.Errorf("stored %d of the company's well-formed postings, want 2", stored)
	}
}

// M1-01 AC5: concurrent runs for a company not stored yet create exactly one company
// row and none of them fails (companyID is a single upsert on the board).
func TestOnceConcurrentRunsCreateOneCompany(t *testing.T) {
	srv, company, _, _ := board(t)
	storetest.Migrate(t, storetest.OwnerURL)
	const runs = 6
	conns := make([]*pgx.Conn, runs)
	for i := range conns {
		conns[i] = storetest.Connect(t, storetest.AppURL)
	}
	errs := make(chan error, runs)
	start := make(chan struct{})
	for _, c := range conns {
		go func() {
			<-start
			_, err := collector.Once(t.Context(), c, lever.New(srv.URL), company, slog.New(slog.DiscardHandler))
			errs <- err
		}()
	}
	close(start)
	for range runs {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Once: %v", err)
		}
	}
	var companies int
	if err := conns[0].QueryRow(t.Context(),
		`SELECT count(*) FROM companies WHERE board_type = 'lever' AND board_slug = $1`, company).Scan(&companies); err != nil {
		t.Fatalf("counting companies: %v", err)
	}
	if companies != 1 {
		t.Errorf("got %d company rows after %d concurrent runs, want 1", companies, runs)
	}
}
