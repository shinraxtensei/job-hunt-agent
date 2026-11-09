//go:build integration

package collector_test

import (
	"slices"
	"testing"

	"github.com/shinraxtensei/job-hunt-agent/internal/collector"
	"github.com/shinraxtensei/job-hunt-agent/internal/preset"
)

// AC5 (ADR-0011): a preset is evaluated on duplicate groups: facets from the canonical
// job, location from every member, so "Berlin, Remote" shows if Remote qualifies.
func TestGroupsForPresets(t *testing.T) {
	e := newEnv(t)
	defer e.pool.Close()
	slug := e.add("lever", "Groups", true, false)
	src := rawsSource{
		leverPosting(slug+"-berlin", "Backend Engineer - Berlin", "Berlin", "onsite"),
		leverPosting(slug+"-remote", "Backend Engineer - Remote", "Remote - EMEA", "remote"),
	}
	if _, err := collector.Run(t.Context(), e.pool, collector.Registry{"lever": src}, collector.Config{Concurrency: 1}, quiet()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	gs, err := collector.Groups(t.Context(), e.pool)
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	var g *collector.GroupRow
	for i := range gs {
		if gs[i].Company == "Groups" {
			g = &gs[i]
		}
	}
	if g == nil || g.RoleFamily != "backend" || g.Track != "remote" || g.Closed || len(g.Members) != 2 {
		t.Fatalf("group = %+v", g)
	}
	var scopes []string
	for _, m := range g.Members {
		scopes = append(scopes, m.RemoteScopes...)
	}
	if !slices.Equal(scopes, []string{"emea"}) {
		t.Errorf("members' scopes = %v, want [emea]", scopes)
	}
	p, err := preset.Load("../../data/presets/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Evaluate(g.Group); got != nil {
		t.Errorf("Berlin + Remote EMEA hidden by %v", got)
	}
}
