package collector

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"

	"github.com/shinraxtensei/job-hunter/internal/preset"
	"github.com/shinraxtensei/job-hunter/internal/store"
)

// GroupRow is one duplicate group ready for preset evaluation, with its canonical title
// and company for display.
type GroupRow struct {
	Company, Title string
	preset.Group
}

// Groups reads every group of an active company with its members' places (ADR-0011).
func Groups(ctx context.Context, db DB) ([]GroupRow, error) {
	q := store.New(db)
	gs, err := q.ListGroupsForPresets(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing groups: %w", err)
	}
	places, err := q.ListJobPlaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing job places: %w", err)
	}
	type key struct {
		company int64
		title   string
	}
	members := map[key][]preset.Place{}
	for _, p := range places {
		k := key{p.CompanyID, p.TitleKey}
		members[k] = append(members[k], preset.Place{Workplace: p.Workplace, Locations: p.Locations,
			RemoteScopes: p.RemoteScopes, Countries: p.Countries})
	}
	out := make([]GroupRow, 0, len(gs))
	for _, g := range gs {
		out = append(out, GroupRow{Company: g.Company, Title: g.Title, Group: preset.Group{
			RoleFamily: g.RoleFamily, Seniority: g.Seniority, ExperienceMinYears: int(g.ExperienceMinYears.Int32),
			Track: g.Track, Closed: g.Closed, Members: members[key{g.CompanyID, g.TitleKey}],
		}})
	}
	return out, nil
}

// Preview writes how many groups p shows and hides, by filter (with how many each filter
// hides on its own), and up to sample random shown and hidden groups with their reasons.
// Titles are untrusted: they are printed quoted.
func Preview(w io.Writer, p preset.Preset, groups []GroupRow, sample int, rng *rand.Rand) {
	type counts struct{ all, only int }
	by := map[string]*counts{}
	var shown, hidden []string
	for _, g := range groups {
		reasons := p.Evaluate(g.Group)
		if reasons == nil {
			shown = append(shown, fmt.Sprintf("%q (%s) %s/%s", g.Title, g.Company, g.RoleFamily, g.Seniority))
			continue
		}
		hidden = append(hidden, fmt.Sprintf("%q (%s): %s", g.Title, g.Company, strings.Join(reasons, ", ")))
		for _, r := range reasons {
			f, _, _ := strings.Cut(r, "=")
			if by[f] == nil {
				by[f] = &counts{}
			}
			by[f].all++
			if len(reasons) == 1 {
				by[f].only++
			}
		}
	}
	fmt.Fprintf(w, "preset %s: %d groups: %d shown, %d hidden\n\nhidden by:\n", p.Name, len(groups), len(shown), len(hidden))
	for _, f := range []string{"role_family", "seniority", "experience_min_years", "location", "closed"} {
		if c := by[f]; c != nil {
			fmt.Fprintf(w, "  %-22s %d (only this: %d)\n", f, c.all, c.only)
		}
	}
	for _, s := range []struct {
		name  string
		lines []string
	}{{"shown", shown}, {"hidden", hidden}} {
		rng.Shuffle(len(s.lines), func(i, j int) { s.lines[i], s.lines[j] = s.lines[j], s.lines[i] })
		fmt.Fprintf(w, "\n%d random %s:\n", min(sample, len(s.lines)), s.name)
		for _, l := range s.lines[:min(sample, len(s.lines))] {
			fmt.Fprintf(w, "  %s\n", l)
		}
	}
}
