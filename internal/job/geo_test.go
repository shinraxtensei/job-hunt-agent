package job_test

import (
	"slices"
	"testing"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
)

// M2-09: every ISO 3166-1 alpha-2 code is known, and its English name in a location
// resolves to that code (Georgia excepted: it is also a US state).
func TestEveryISOCodeResolves(t *testing.T) {
	codes := job.CountryCodes()
	if len(codes) != 249 {
		t.Fatalf("%d codes, want 249", len(codes))
	}
	for _, c := range codes {
		name := job.CountryName(c)
		if name == "" {
			t.Errorf("%s has no name", c)
			continue
		}
		got := job.Classify(job.Record{Workplace: job.Onsite, Locations: []string{name}}, "").Countries
		want := []string{c}
		if c == "GE" {
			want = nil
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s %q → %v, want %v", c, name, got, want)
		}
	}
}

func TestCountryAliases(t *testing.T) {
	for alias, code := range map[string]string{
		"UAE": "AE", "Türkiye": "TR", "Turkey": "TR", "Czechia": "CZ", "Czech Republic": "CZ",
		"South Korea": "KR", "Korea": "KR", "North Korea": "KP", "Côte d'Ivoire": "CI", "Ivory Coast": "CI",
		"Hong Kong": "HK", "DRC": "CD", "Republic of the Congo": "CG", "Holland": "NL", "Burma": "MM",
		"Vietnam": "VN", "Viet Nam": "VN", "Russia": "RU", "Eswatini": "SZ", "Swaziland": "SZ",
		"Bosnia and Herzegovina": "BA", "Trinidad and Tobago": "TT", "Saint Lucia": "LC", "St. Lucia": "LC",
		"Saint Kitts and Nevis": "KN", "Guinea-Bissau": "GW", "Papua New Guinea": "PG", "Guinea": "GN",
		"South Sudan": "SS", "Sudan": "SD", "Niger": "NE", "Nigeria": "NG", "Dominican Republic": "DO",
		"Dominica": "DM", "American Samoa": "AS", "Samoa": "WS", "Caribbean Netherlands": "BQ",
		"British Virgin Islands": "VG", "U.S. Virgin Islands": "VI", "Isle of Man": "IM",
		"Maroc": "MA", "Allemagne": "DE", "États-Unis": "US", "New Jersey": "US", "Jersey": "JE",
	} {
		got := job.Classify(job.Record{Workplace: job.Onsite, Locations: []string{alias}}, "").Countries
		if !slices.Equal(got, []string{code}) {
			t.Errorf("%q → %v, want [%s]", alias, got, code)
		}
	}
	for _, s := range []string{"Atlanta, Georgia", "Georgia"} {
		if got := job.Classify(job.Record{Workplace: job.Onsite, Locations: []string{s}}, "").Countries; slices.Contains(got, "GE") {
			t.Errorf("%q → %v: Georgia is ambiguous and must not resolve to GE", s, got)
		}
	}
}

// M2-09: LatAm and the Americas are remote regions; countries there resolve.
func TestLatAmAndAmericas(t *testing.T) {
	for _, c := range []struct {
		loc       string
		scopes    []string
		countries []string
	}{
		{"Remote - LatAm", []string{"latam"}, nil},
		{"Remote, Latin America", []string{"latam"}, nil},
		{"Remote - South America", []string{"latam"}, nil},
		{"Remote, Americas", []string{"americas"}, nil},
		{"Remote - AMER", []string{"americas"}, nil},
		{"Remote - NA", []string{"americas"}, nil},
		{"Panama", []string{"country:PA"}, []string{"PA"}},
		{"Paraguay", []string{"country:PY"}, []string{"PY"}},
		{"Remote - Ecuador", []string{"country:EC"}, []string{"EC"}},
	} {
		got := job.Classify(job.Record{Workplace: job.Remote, Locations: []string{c.loc}}, "")
		if !slices.Equal(got.RemoteScopes, c.scopes) || !slices.Equal(got.Countries, c.countries) {
			t.Errorf("%q: scopes %v countries %v, want %v %v", c.loc, got.RemoteScopes, got.Countries, c.scopes, c.countries)
		}
	}
	latam, americas := job.RegionCountries("latam"), job.RegionCountries("americas")
	for _, c := range []string{"BR", "MX", "PA", "PY", "AR"} {
		if !slices.Contains(latam, c) || !slices.Contains(americas, c) {
			t.Errorf("%s missing from latam or americas", c)
		}
	}
	if slices.Contains(latam, "US") || !slices.Contains(americas, "US") || !slices.Contains(americas, "CA") {
		t.Error("the US and Canada are in the Americas, not LatAm")
	}
	if job.RegionCountries("narnia") != nil {
		t.Error("an unknown region has members")
	}
}

// --report-unknown: a location is unresolved when it names no country and no region.
func TestUnresolvedLocation(t *testing.T) {
	for loc, want := range map[string]bool{"Remote": true, "Distributed": true, "Béziers": true,
		"Panama": false, "Remote - EMEA": false, "Atlanta, Georgia": false, "Berlin": false} {
		if got := job.Unresolved(loc); got != want {
			t.Errorf("Unresolved(%q) = %v, want %v", loc, got, want)
		}
	}
}
