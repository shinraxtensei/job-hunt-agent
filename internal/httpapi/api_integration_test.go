//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/httpapi"
	"github.com/shinraxtensei/job-hunter/internal/store/storetest"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestMain(m *testing.M) { storetest.Main(m) }

// now is the API's clock in these tests: "new" means first seen after 2026-11-01T12:00Z.
var now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

var (
	seedOnce sync.Once
	pool     *pgxpool.Pool
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// world seeds a small fixed dataset (explicit ids, fixed timestamps) once per package.
func world(t *testing.T) *pgxpool.Pool {
	t.Helper()
	seedOnce.Do(func() {
		storetest.Migrate(t, storetest.OwnerURL)
		var err error
		if pool, err = pgxpool.New(t.Context(), storetest.AppURL); err != nil {
			t.Fatalf("pool: %v", err)
		}
		exec := func(sql string, args ...any) {
			if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
				t.Fatalf("seeding: %v\n%s", err, sql)
			}
		}
		exec(`INSERT INTO companies (id, name, board_type, board_slug, track, active) OVERRIDING SYSTEM VALUE VALUES
			(901, 'Acme', 'lever', 'acme', 'remote', true),
			(902, 'Brick', 'lever', 'brick', 'relocation', true),
			(903, 'Gone', 'lever', 'gone', 'morocco', false)`)
		for _, p := range []struct {
			id                                   int64
			company                              int64
			title, location, workplace, desc, at string
		}{
			{9001, 901, "Senior Backend Engineer", "Remote – Morocco", "remote", "You have 5+ years of experience with Go.", "2026-10-30T10:00:00Z"},
			{9002, 901, "Senior Backend Engineer", "Remote - EMEA", "remote", "", "2026-10-31T10:00:00Z"},
			{9003, 901, "Account Executive", "Paris, France", "onsite", "", "2026-11-01T09:00:00Z"},
			{9004, 901, "Staff Data Engineer", "Remote - US", "remote", "8+ years of experience", "2026-11-02T08:00:00Z"},
			{9005, 901, "Frontend Engineer", "Casablanca", "hybrid", "", "2026-11-02T09:00:00Z"},
			{9006, 902, "Mobile Engineer (Flutter)", "Amsterdam", "onsite", "", "2026-11-01T12:30:00Z"},
			{9007, 903, "Backend Engineer", "Remote", "remote", "", "2026-11-01T10:00:00Z"},
		} {
			payload, _ := json.Marshal(map[string]any{
				"text": p.title, "hostedUrl": fmt.Sprintf("https://jobs.lever.co/x/%d", p.id),
				"workplaceType": p.workplace, "descriptionPlain": p.desc, "createdAt": 1761900000000,
				"categories": map[string]any{"location": p.location, "team": "Engineering"},
			})
			exec(`INSERT INTO postings (id, company_id, source, source_posting_id, url, title, raw_payload,
					first_seen_at, last_checked_at, last_changed_at, content_hash)
				OVERRIDING SYSTEM VALUE
				VALUES ($1, $2, 'lever', $7, $3, $4, $5, $6, $6, $6, sha256(convert_to($5::jsonb::text, 'UTF8')))`,
				p.id, p.company, fmt.Sprintf("https://jobs.lever.co/x/%d", p.id), p.title, string(payload), p.at, strconv.FormatInt(p.id, 10))
		}
		exec(`UPDATE postings SET closed_at = '2026-11-02T10:00:00Z' WHERE id = 9005`)
		if _, err := collector.NormaliseStored(t.Context(), pool, 100, quiet()); err != nil {
			t.Fatalf("normalising: %v", err)
		}
	})
	if pool == nil {
		t.Fatal("seeding failed earlier")
	}
	return pool
}

func server(t *testing.T) http.Handler {
	t.Helper()
	return httpapi.New(world(t), "../../data/presets", "test-sha", func() time.Time { return now }, quiet())
}

// AC1, AC2, AC2b-d, AC4, AC6: each endpoint's JSON is pinned by a golden file, so the
// contract can't change by accident (go test -tags integration ./internal/httpapi -update).
func TestGolden(t *testing.T) {
	h := server(t)
	for _, c := range []struct {
		name, url string
		status    int
	}{
		{"leads_default", "/api/leads", 200},
		{"leads_none", "/api/leads?preset=none", 200},
		{"leads_override", "/api/leads?role_family=sales&workplace=onsite", 200},
		{"leads_new", "/api/leads?preset=none&new=true", 200},
		{"leads_since_company", "/api/leads?preset=none&since=2026-11-01&company=acme", 200},
		{"leads_first_page", "/api/leads?preset=none&limit=2", 200},
		{"hidden_default", "/api/leads/hidden", 200},
		{"members", "/api/leads/9001/members", 200},
		{"members_of_any_member", "/api/leads/9002/members", 200},
		{"members_unknown", "/api/leads/424242/members", 404},
		{"members_bad_id", "/api/leads/abc/members", 400},
		{"facets_default", "/api/facets", 200},
		{"facets_none", "/api/facets?preset=none", 200},
		{"facets_backend", "/api/facets?preset=none&role_family=backend", 200},
		{"facets_remote", "/api/facets?preset=none&workplace=remote&company=Acme", 200},
		{"presets", "/api/presets", 200},
		{"companies", "/api/companies", 200},
		{"healthz", "/healthz", 200},
		{"bad_seniority", "/api/leads?seniority=ancient", 400},
		{"bad_scope", "/api/leads?remote_scope=mars", 400},
		{"unknown_param", "/api/leads?colour=blue", 400},
		{"unknown_preset", "/api/leads?preset=nope", 400},
		{"bad_cursor", "/api/leads?cursor=zzz", 400},
		{"bad_limit", "/api/leads?limit=500", 400},
		{"not_found", "/api/nothing", 404},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.url, nil))
			if rec.Code != c.status {
				t.Errorf("status %d, want %d: %s", rec.Code, c.status, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type %q", ct)
			}
			if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Error("no nosniff header")
			}
			var got bytes.Buffer
			if err := json.Indent(&got, rec.Body.Bytes(), "", "  "); err != nil {
				t.Fatalf("not JSON: %v\n%s", err, rec.Body)
			}
			got.WriteByte('\n')
			path := filepath.Join("testdata", "golden", c.name+".json")
			if *update {
				if err := os.WriteFile(path, got.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading golden (run with -update): %v", err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("%s differs from %s:\n%s", c.url, path, got.String())
			}
		})
	}
}

// AC3: paging returns every group exactly once even when new jobs (new groups, and a
// new member of an existing group) arrive between pages.
func TestCursorPaginationIsStable(t *testing.T) {
	db := world(t)
	h := server(t)
	if _, err := db.Exec(t.Context(), `INSERT INTO companies (id, name, board_type, board_slug, track, active)
		OVERRIDING SYSTEM VALUE VALUES (950, 'PageCo', 'lever', 'pageco', 'remote', true)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background() // t.Context is already cancelled in cleanup
		// Leave the shared world as the golden files expect it, whatever order tests run in.
		for _, q := range []string{`DELETE FROM jobs WHERE company_id = 950`, `DELETE FROM postings WHERE company_id = 950`,
			`DELETE FROM companies WHERE id = 950`} {
			if _, err := db.Exec(ctx, q); err != nil {
				t.Errorf("cleaning up: %v", err)
			}
		}
	})
	n := 0
	add := func(title, at string) {
		n++
		payload := fmt.Sprintf(`{"text":%q,"workplaceType":"remote","categories":{"location":"Remote"}}`, title)
		if _, err := db.Exec(t.Context(), `INSERT INTO postings (company_id, source, source_posting_id, url, title, raw_payload,
				first_seen_at, last_checked_at, last_changed_at, content_hash)
			VALUES (950, 'lever', $1, 'https://example.test', $2, $3, $4, $4, $4, sha256(convert_to($3::jsonb::text, 'UTF8')))`,
			fmt.Sprintf("page-%d-%d", n, time.Now().UnixNano()), title, payload, at); err != nil {
			t.Fatal(err)
		}
		if _, err := collector.NormaliseStored(t.Context(), db, 100, quiet()); err != nil {
			t.Fatal(err)
		}
	}
	// Seven groups; two pairs share a first-seen time, so the tie-break is exercised.
	for i, at := range []string{"2026-10-01T00:00:00Z", "2026-10-02T00:00:00Z", "2026-10-02T00:00:00Z",
		"2026-10-03T00:00:00Z", "2026-10-04T00:00:00Z", "2026-10-04T00:00:00Z", "2026-10-05T00:00:00Z"} {
		add(fmt.Sprintf("Role %c", 'A'+i), at)
	}

	seen := map[string]int{}
	cursor := ""
	for page := 0; ; page++ {
		url := "/api/leads?preset=none&company=PageCo&limit=3"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		var body struct {
			Leads []struct {
				Title string `json:"title"`
			} `json:"leads"`
			NextCursor string `json:"next_cursor"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 {
			t.Fatalf("page %d: %d %s", page, rec.Code, rec.Body)
		}
		for _, l := range body.Leads {
			seen[l.Title]++
		}
		if page == 0 {
			add("Role New 1", time.Now().UTC().Format(time.RFC3339)) // new groups, newest of all
			add("Role New 2", time.Now().UTC().Format(time.RFC3339))
			add("Role B", time.Now().UTC().Format(time.RFC3339)) // a new member of an existing group
		}
		if body.NextCursor == "" {
			break
		}
		cursor = body.NextCursor
	}
	for i := range 7 {
		if title := fmt.Sprintf("Role %c", 'A'+i); seen[title] != 1 {
			t.Errorf("%s seen %d times, want 1 (all: %v)", title, seen[title], seen)
		}
	}
	for title, c := range seen {
		if c > 1 {
			t.Errorf("%s repeated", title)
		}
	}
}

// AC2c: each facet is counted with its own filter removed and every other filter applied,
// so a value's count is how many leads choosing it would add. With role_family=backend,
// sales still has its real count; the other facets count only the backend lead.
func TestFacetCountsDropTheirOwnFilter(t *testing.T) {
	rec := httptest.NewRecorder()
	server(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/facets?preset=none&role_family=backend", nil))
	var body struct {
		Total  int                       `json:"total"`
		Facets map[string]map[string]int `json:"facets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || body.Facets["role_family"]["sales"] != 1 || body.Facets["role_family"]["backend"] != 1 ||
		body.Facets["role_family"]["data"] != 1 {
		t.Errorf("total %d, role_family %v; want 1 shown and sales, backend, data each 1", body.Total, body.Facets["role_family"])
	}
	if body.Facets["seniority"]["staff_plus"] != 0 || body.Facets["seniority"]["senior"] != 1 {
		t.Errorf("seniority %v: other facets must still apply role_family=backend", body.Facets["seniority"])
	}
}
