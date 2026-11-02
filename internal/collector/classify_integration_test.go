//go:build integration

package collector_test

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/job"
)

type facetRow struct {
	family, seniority string
	experience        *int
	scopes, countries []string
	version           int
}

func facetsOf(t *testing.T, e *env, sourceID string) facetRow {
	t.Helper()
	var f facetRow
	if err := e.pool.QueryRow(t.Context(),
		`SELECT j.role_family, j.seniority, j.experience_min_years, j.remote_scopes, j.countries, j.classifier_version
		 FROM jobs j JOIN postings p ON p.id = j.posting_id WHERE p.source_posting_id = $1`, sourceID).
		Scan(&f.family, &f.seniority, &f.experience, &f.scopes, &f.countries, &f.version); err != nil {
		t.Fatalf("reading facets of %s: %v", sourceID, err)
	}
	return f
}

// M2-03 AC1 + AC6: a collector run writes the facets with the job record; `collector
// classify` rewrites only rows made by an older classifier version.
func TestClassifyFacetsAndBackfill(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Facets", true, false)
	backend := job.Raw{Source: "lever", SourceID: slug + "-a", URL: "https://example.test/a", Title: "Senior Backend Engineer",
		Payload: []byte(`{"text":"Senior Backend Engineer","workplaceType":"remote","categories":{"location":"Remote – Morocco"},
			"descriptionPlain":"We were founded 12 years ago. You have 5+ years of experience with Go."}`)}
	sales := job.Raw{Source: "lever", SourceID: slug + "-b", URL: "https://example.test/b", Title: "Account Executive",
		Payload: []byte(`{"text":"Account Executive","workplaceType":"onsite","categories":{"location":"Paris, France"}}`)}
	if _, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": rawsSource{backend, sales}},
		collector.Config{Concurrency: 1}, quiet()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	a := facetsOf(t, e, slug+"-a")
	if a.family != "backend" || a.seniority != "senior" || a.experience == nil || *a.experience != 5 ||
		!slices.Equal(a.scopes, []string{"country:MA"}) || !slices.Equal(a.countries, []string{"MA"}) ||
		a.version != job.ClassifierVersion {
		t.Errorf("facets after run = %+v (experience %v)", a, a.experience)
	}
	if b := facetsOf(t, e, slug+"-b"); b.family != "sales" || b.experience != nil || len(b.scopes) != 0 ||
		!slices.Equal(b.countries, []string{"FR"}) {
		t.Errorf("sales facets = %+v", b)
	}

	// a: made by an older classifier. b: current (a sentinel value shows it isn't rewritten).
	for stmt, id := range map[string]string{
		`UPDATE jobs SET classifier_version = 0, role_family = 'other' WHERE posting_id = (SELECT id FROM postings WHERE source_posting_id = $1)`: slug + "-a",
		`UPDATE jobs SET role_family = 'legal' WHERE posting_id = (SELECT id FROM postings WHERE source_posting_id = $1)`:                         slug + "-b",
	} {
		if _, err := e.pool.Exec(t.Context(), stmt, id); err != nil {
			t.Fatalf("staging %s: %v", id, err)
		}
	}
	var logs bytes.Buffer
	res, err := collector.ClassifyStored(t.Context(), e.pool, 1, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatalf("ClassifyStored: %v", err)
	}
	if res.Written < 1 || res.Failed != 0 {
		t.Errorf("result = %+v, want >= 1 written, 0 failed", res)
	}
	if a := facetsOf(t, e, slug+"-a"); a.family != "backend" || a.version != job.ClassifierVersion {
		t.Errorf("stale row not reclassified: %+v", a)
	}
	if b := facetsOf(t, e, slug+"-b"); b.family != "legal" {
		t.Errorf("current row was reclassified: %+v", b)
	}
	if !strings.Contains(logs.String(), `"msg":"classify finished"`) || strings.Contains(logs.String(), "Go.") {
		t.Errorf("want a summary and no description text in logs:\n%s", logs.String())
	}
}

// M2-09: --report-unknown lists the location strings that resolve to no country, city or
// region, most common first, counted over jobs.
func TestUnresolvedLocations(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Unknowns", true, false)
	src := rawsSource{
		leverPosting(slug+"-a", "A", "Béziers-sur-Test", "onsite"),
		leverPosting(slug+"-b", "B", "Béziers-sur-Test", "onsite"),
		leverPosting(slug+"-c", "C", "Nowhere Valley", "remote"),
		leverPosting(slug+"-d", "D", "Paris, France", "onsite"),
	}
	if _, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": src}, collector.Config{Concurrency: 1}, quiet()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := collector.UnresolvedLocations(t.Context(), e.pool, 1000)
	if err != nil {
		t.Fatalf("UnresolvedLocations: %v", err)
	}
	counts := map[string]int{}
	for _, l := range got {
		counts[l.Location] = l.Jobs
	}
	if counts["Béziers-sur-Test"] != 2 || counts["Nowhere Valley"] != 1 {
		t.Errorf("counts = %v, want Béziers-sur-Test 2 and Nowhere Valley 1", counts)
	}
	if _, ok := counts["Paris, France"]; ok {
		t.Error("a resolved location was reported")
	}
	for i := 1; i < len(got); i++ {
		if got[i].Jobs > got[i-1].Jobs {
			t.Fatalf("not sorted by count: %v", got)
		}
	}
}
