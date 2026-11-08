//go:build integration

package collector_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/collector"
)

// M6-06 AC1-AC3: a company added by hand merges into its watch-list twin: postings, jobs
// and applications move in one transaction (history and documents stay attached), the
// hand-added company stays, inactive; --dry-run changes nothing; a watch-list company
// can't be merged away; clashing applications refuse the merge.
func TestMergeCompany(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	ctx := t.Context()
	name := fmt.Sprintf("Atlas %d", e.n)
	e.add("lever", name, true, false)
	var watch int64
	if err := e.pool.QueryRow(ctx, `SELECT id FROM companies WHERE name = $1 AND board_type = 'lever'`, name).Scan(&watch); err != nil {
		t.Fatal(err)
	}
	addManual := func(company, link string) (posting, companyID int64, titleKey string) {
		t.Helper()
		res, err := collector.Add(ctx, e.pool, collector.Registry{}, 0, link, collector.Manual{Title: "Backend Engineer", Company: company}, quiet())
		if err != nil {
			t.Fatal(err)
		}
		if err := e.pool.QueryRow(ctx, `SELECT company_id, title_key FROM jobs WHERE posting_id = $1`, res.PostingID).Scan(&companyID, &titleKey); err != nil {
			t.Fatal(err)
		}
		return res.PostingID, companyID, titleKey
	}
	posting, manual, key := addManual(name, fmt.Sprintf("https://www.linkedin.com/jobs/view/%d/", e.n%1e9))
	var app int64
	if err := e.pool.QueryRow(ctx, `WITH a AS (INSERT INTO applications (company_id, title_key, posting_id, status)
		VALUES ($1, $2, $3, 'applied') RETURNING id), ev AS (INSERT INTO application_events (application_id, status) SELECT id, 'applied' FROM a)
		SELECT id FROM a`, manual, key, posting).Scan(&app); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ from, into, want string }{
		{fmt.Sprint(watch), fmt.Sprint(manual), "watch list"},
		{fmt.Sprint(manual), fmt.Sprint(manual), "into itself"},
		{"no such company " + name, name, "no company"},
	} {
		if _, err := collector.MergeCompany(ctx, e.pool, c.from, c.into, false); !errors.Is(err, collector.ErrMerge) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s into %s: %v, want %q", c.from, c.into, err, c.want)
		}
	}

	plan, err := collector.MergeCompany(ctx, e.pool, name, name, true)
	if err != nil || plan.From.ID != manual || plan.Into.ID != watch || plan.Postings != 1 || plan.Jobs != 1 || plan.Applications != 1 {
		t.Fatalf("dry run: %+v, %v", plan, err)
	}
	where := func() (p, j, a int64) {
		t.Helper()
		if err := e.pool.QueryRow(ctx, `SELECT (SELECT company_id FROM postings WHERE id = $1), (SELECT company_id FROM jobs WHERE posting_id = $1),
			(SELECT company_id FROM applications WHERE id = $2)`, posting, app).Scan(&p, &j, &a); err != nil {
			t.Fatal(err)
		}
		return p, j, a
	}
	if p, j, a := where(); p != manual || j != manual || a != manual {
		t.Fatal("a dry run changed something")
	}

	if _, err := collector.MergeCompany(ctx, e.pool, name, name, false); err != nil {
		t.Fatal(err)
	}
	if p, j, a := where(); p != watch || j != watch || a != watch {
		t.Errorf("after merge: posting %d, job %d, application %d; want %d", p, j, a, watch)
	}
	var active bool
	var events int
	if err := e.pool.QueryRow(ctx, `SELECT (SELECT active FROM companies WHERE id = $1), (SELECT count(*) FROM application_events WHERE application_id = $2)`,
		manual, app).Scan(&active, &events); err != nil {
		t.Fatalf("the hand-added company must stay: %v", err)
	}
	if active || events != 1 {
		t.Errorf("merged-away company active=%v; history %d events", active, events)
	}
	var grouped int64
	if err := e.pool.QueryRow(ctx, `SELECT company_id FROM job_groups WHERE canonical_posting_id = $1`, posting).Scan(&grouped); err != nil || grouped != watch {
		t.Errorf("the group moved with its jobs: %d, %v", grouped, err)
	}

	// a second hand-added copy with an application for the same role clashes
	name2 := name + " SA"
	posting2, manual2, key2 := addManual(name2, fmt.Sprintf("https://www.linkedin.com/jobs/view/%d/", e.n%1e9+1))
	if key2 != key {
		t.Fatalf("title keys differ: %q %q", key, key2)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO applications (company_id, title_key, posting_id, status) VALUES ($1, $2, $3, 'shortlisted')`,
		manual2, key2, posting2); err != nil {
		t.Fatal(err)
	}
	if _, err := collector.MergeCompany(ctx, e.pool, name2, fmt.Sprint(watch), false); !errors.Is(err, collector.ErrMerge) || !strings.Contains(err.Error(), "both companies") {
		t.Errorf("clash: %v", err)
	}
	var still int64
	if err := e.pool.QueryRow(ctx, `SELECT company_id FROM postings WHERE id = $1`, posting2).Scan(&still); err != nil || still != manual2 {
		t.Errorf("a refused merge moved the posting: %d, %v", still, err)
	}
}
