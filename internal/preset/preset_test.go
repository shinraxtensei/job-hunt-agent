package preset_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/preset"
)

// AC3: every invalid entry is reported with its line before anything runs.
func TestParseReportsEveryProblemWithLine(t *testing.T) {
	_, err := preset.Parse([]byte(`name: broken
role_family: [backend, wizard]
seniority: [senior, ancient]
experience_max_years: lots
closed: maybe
colour: blue
location:
  - {workplace: [remote], remote_scopes: [mars, "country:mars"]}
  - {workplace: [floating], countries: [EU_EEA, Germany], track: [nomad]}
  - {}
`))
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{
		`line 2: unknown role_family "wizard"`,
		`line 3: unknown seniority "ancient"`,
		`line 4: experience_max_years`,
		`line 5: closed must be hide, show or only`,
		`line 6: unknown key "colour"`,
		`line 8: unknown remote_scope "mars"`,
		`line 8: unknown remote_scope "country:mars"`,
		`line 9: unknown workplace "floating"`,
		`line 9: country "Germany" is not an ISO alpha-2 code, EU_EEA, LATAM or AMERICAS`,
		`line 9: unknown track "nomad"`,
		`line 10: empty location rule`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

func TestParseNeedsAName(t *testing.T) {
	if _, err := preset.Parse([]byte("role_family: [backend]\n")); err == nil || !strings.Contains(err.Error(), "name") {
		t.Errorf("err = %v, want a missing-name error", err)
	}
}

// An omitted key is no filter: a preset with only a name shows everything, closed included.
func TestEmptyPresetShowsEverything(t *testing.T) {
	p, err := preset.Parse([]byte("name: all\n"))
	if err != nil {
		t.Fatal(err)
	}
	g := preset.Group{RoleFamily: "legal", Seniority: "staff_plus", ExperienceMinYears: 15, Closed: true,
		Members: []preset.Place{{Workplace: "onsite", Locations: []string{"Tokyo"}}}}
	if got := p.Evaluate(g); got != nil {
		t.Errorf("hidden by %v", got)
	}
}

func load(t *testing.T) preset.Preset {
	t.Helper()
	p, err := preset.Load("../../data/presets/default.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return p
}

// AC4: the owner's defaults, each one a filter.
func TestDefaultPresetValues(t *testing.T) {
	p := load(t)
	if p.Name != "default" ||
		!slices.Equal(p.RoleFamily, []string{"backend", "fullstack", "frontend", "mobile", "platform", "data", "ai_ml", "unknown"}) ||
		!slices.Equal(p.Seniority, []string{"junior", "mid", "senior", "unknown"}) ||
		p.ExperienceMaxYears == nil || *p.ExperienceMaxYears != 5 || p.Closed != "hide" || len(p.Location) != 4 {
		t.Errorf("default preset = %+v", p)
	}
	if !slices.Contains(p.Location[2].Countries, "DE") || !slices.Contains(p.Location[2].Countries, "AE") ||
		!slices.Contains(p.Location[0].RemoteScopes, "country:MA") {
		t.Errorf("EU_EEA alias or country:MA missing: %+v", p.Location)
	}
}

func remote(scopes ...string) preset.Place {
	return preset.Place{Workplace: "remote", RemoteScopes: scopes}
}

// AC4 + AC5: shown, or hidden with every filter that hid it.
func TestEvaluateDefault(t *testing.T) {
	p := load(t)
	backend := func(m ...preset.Place) preset.Group {
		return preset.Group{RoleFamily: "backend", Seniority: "senior", Track: "remote", Members: m}
	}
	kraken := remote("country:RO", "country:BR", "country:PE", "country:SE", "country:CO", "country:AR",
		"country:CY", "country:ZA", "country:CZ", "country:HU", "country:MX", "country:CL", "country:PL", "country:BG")
	for _, c := range []struct {
		name string
		g    preset.Group
		want []string
	}{
		{"remote Morocco", backend(remote("country:MA")), nil},
		{"remote EMEA", backend(remote("emea")), nil},
		{"remote, scope unknown", backend(remote("unknown")), nil},
		{"any scope accepted: US or EMEA", backend(remote("us_only", "emea")), nil},
		{"US only", backend(remote("us_only")), []string{"location"}},
		{"multi-country Ashby, none accepted", backend(kraken), []string{"location"}},
		{"Casablanca onsite", backend(preset.Place{Workplace: "onsite", Locations: []string{"Casablanca, Morocco"}}), nil},
		{"Rabat hybrid", backend(preset.Place{Workplace: "hybrid", Locations: []string{"Rabat"}}), nil},
		{"Paris onsite, remote track", backend(preset.Place{Workplace: "onsite", Locations: []string{"Paris, France"}, Countries: []string{"FR"}}), []string{"location"}},
		{"workplace unknown", backend(preset.Place{Workplace: "unknown", Locations: []string{"Tokyo"}}), nil},
		{"group: Berlin onsite + remote EMEA", backend(preset.Place{Workplace: "onsite", Locations: []string{"Berlin"}}, remote("emea")), nil},
		{"unknown everything", preset.Group{RoleFamily: "unknown", Seniority: "unknown", Members: []preset.Place{remote("unknown")}}, nil},
		{"sales staff, 7 years, closed, US", preset.Group{RoleFamily: "sales", Seniority: "staff_plus", ExperienceMinYears: 7,
			Closed: true, Members: []preset.Place{remote("us_only")}},
			[]string{"role_family=sales", "seniority=staff_plus", "experience_min_years=7", "location", "closed"}},
		{"5 years is fine", preset.Group{RoleFamily: "data", Seniority: "mid", ExperienceMinYears: 5, Members: []preset.Place{remote("worldwide")}}, nil},
		{"6 years hides", preset.Group{RoleFamily: "data", Seniority: "mid", ExperienceMinYears: 6, Members: []preset.Place{remote("worldwide")}},
			[]string{"experience_min_years=6"}},
		{"intern", preset.Group{RoleFamily: "fullstack", Seniority: "intern", Members: []preset.Place{remote("europe")}}, []string{"seniority=intern"}},
	} {
		if got := p.Evaluate(c.g); !slices.Equal(got, c.want) {
			t.Errorf("%s: hidden by %v, want %v", c.name, got, c.want)
		}
	}

	// Relocation-track companies: onsite in the EU/EEA, UK, Switzerland or UAE.
	reloc := func(country string) preset.Group {
		return preset.Group{RoleFamily: "backend", Seniority: "mid", Track: "relocation",
			Members: []preset.Place{{Workplace: "onsite", Countries: []string{country}}}}
	}
	for c, hidden := range map[string]bool{"DE": false, "NL": false, "GB": false, "CH": false, "AE": false, "NO": false, "US": true, "SG": true} {
		if got := p.Evaluate(reloc(c)); (got != nil) != hidden {
			t.Errorf("relocation onsite %s: hidden by %v, want hidden=%v", c, got, hidden)
		}
	}
}

// ADR-0011 owner decision: a remote rule passes if ANY of the job's scopes is accepted,
// shown with the multi-country Ashby posting and a preset that accepts one of them.
func TestRemoteRulePassesOnAnyScope(t *testing.T) {
	p, err := preset.Parse([]byte("name: latam\nlocation:\n  - {workplace: [remote], remote_scopes: [\"country:BR\"]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	g := preset.Group{Members: []preset.Place{remote("country:RO", "country:BR", "country:PE")}}
	if got := p.Evaluate(g); got != nil {
		t.Errorf("hidden by %v", got)
	}
}

// closed: only shows closed groups and nothing else.
func TestClosedOnly(t *testing.T) {
	p, err := preset.Parse([]byte("name: archive\nclosed: only\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Evaluate(preset.Group{Closed: true}) != nil || p.Evaluate(preset.Group{}) == nil {
		t.Error("closed: only does not show exactly the closed groups")
	}
}

// M2-08 AC2: the location shown first is the one that made the group pass the location
// rules: of the first member a rule matches, the first location the rule matches alone.
func TestMatchedLocation(t *testing.T) {
	p := load(t)
	place := func(workplace string, scopes, countries []string, locs ...string) preset.Place {
		return preset.Place{Workplace: workplace, Locations: locs, RemoteScopes: scopes, Countries: countries}
	}
	for _, c := range []struct {
		name  string
		track string
		ms    []preset.Place
		want  string
	}{
		{"multi-country remote: the accepted country, not the first", "remote",
			[]preset.Place{place("remote", []string{"country:RO", "country:BR", "country:MA"}, []string{"RO", "BR", "MA"},
				"Romania", "Brazil", "Morocco")}, "Morocco"},
		{"the member that matched, not the first member", "remote",
			[]preset.Place{place("onsite", nil, []string{"DE"}, "Berlin, Germany"), place("remote", []string{"emea"}, nil, "Remote - EMEA")},
			"Remote - EMEA"},
		{"onsite city", "remote",
			[]preset.Place{place("onsite", nil, []string{"FR", "MA"}, "Paris, France", "Casablanca, Morocco")}, "Casablanca, Morocco"},
		{"relocation country from a bare city", "relocation",
			[]preset.Place{place("hybrid", nil, []string{"JP", "NL"}, "Tokyo", "Amsterdam")}, "Amsterdam"},
		{"workplace unknown matches its first location", "remote",
			[]preset.Place{place("unknown", nil, nil, "Tokyo", "Osaka")}, "Tokyo"},
		{"no rule matched", "remote",
			[]preset.Place{place("remote", []string{"us_only"}, []string{"US"}, "Remote, US")}, ""},
		{"matched by the board's country code only: the member's first location", "remote",
			[]preset.Place{place("remote", []string{"country:MA"}, []string{"MA"}, "Remote")}, "Remote"},
	} {
		g := preset.Group{RoleFamily: "backend", Seniority: "senior", Track: c.track, Members: c.ms}
		if got := p.MatchedLocation(g); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if got := (preset.Preset{Name: "none"}).MatchedLocation(preset.Group{Members: []preset.Place{place("remote", nil, nil, "Anywhere")}}); got != "" {
		t.Errorf("no location rules: %q, want empty", got)
	}
}

// M2-09: latam is a remote scope, and LATAM/AMERICAS expand to their member countries in
// a countries rule (like EU_EEA).
func TestRegionAliases(t *testing.T) {
	p, err := preset.Parse([]byte("name: latam\nlocation:\n  - {workplace: [remote], remote_scopes: [latam, americas]}\n  - {workplace: [onsite], countries: [LATAM]}\n  - {workplace: [hybrid], countries: [AMERICAS]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(p.Location[1].Countries, "BR") || slices.Contains(p.Location[1].Countries, "US") ||
		!slices.Contains(p.Location[2].Countries, "US") || !slices.Contains(p.Location[2].Countries, "PA") {
		t.Errorf("aliases expanded to %v and %v", p.Location[1].Countries, p.Location[2].Countries)
	}
	if got := p.Evaluate(preset.Group{Members: []preset.Place{remote("latam")}}); got != nil {
		t.Errorf("remote latam hidden by %v", got)
	}
}
