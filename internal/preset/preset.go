// Package preset loads filter presets (data/presets/<name>.yaml) and evaluates them
// against duplicate groups (ADR-0011). Presets hide; nothing is deleted.
package preset

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/shinraxtensei/job-hunter/internal/job"
)

// Preset is one set of filters. A nil list or pointer, or an empty Closed, is no filter.
type Preset struct {
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	RoleFamily         []string `json:"role_family,omitempty"`
	Seniority          []string `json:"seniority,omitempty"`
	ExperienceMaxYears *int     `json:"experience_max_years,omitempty"` // hide groups whose explicit minimum is above this
	Closed             string   `json:"closed,omitempty"`               // hide | show | only
	Location           []Rule   `json:"location,omitempty"`             // shown if any rule matches any member
}

// Rule is one location rule: every list it sets must match.
type Rule struct {
	Workplace    []string `json:"workplace,omitempty"`
	RemoteScopes []string `json:"remote_scopes,omitempty"`
	Cities       []string `json:"cities,omitempty"`
	Countries    []string `json:"countries,omitempty"`
	Track        []string `json:"track,omitempty"`
}

// Group is what a preset is evaluated on: the canonical job's facets, the company's
// track, whether every member is closed, and each member's place.
type Group struct {
	RoleFamily, Seniority string
	ExperienceMinYears    int // 0: none stated
	Track                 string
	Closed                bool
	Members               []Place
}

// Place is one member's location facets.
type Place struct {
	Workplace                          string
	Locations, RemoteScopes, Countries []string
}

// Evaluate returns every filter that hides g ("role_family=sales", "location", ...), or
// nil when g is shown.
func (p Preset) Evaluate(g Group) []string {
	var hidden []string
	if p.RoleFamily != nil && !slices.Contains(p.RoleFamily, g.RoleFamily) {
		hidden = append(hidden, "role_family="+g.RoleFamily)
	}
	if p.Seniority != nil && !slices.Contains(p.Seniority, g.Seniority) {
		hidden = append(hidden, "seniority="+g.Seniority)
	}
	if p.ExperienceMaxYears != nil && g.ExperienceMinYears > *p.ExperienceMaxYears {
		hidden = append(hidden, "experience_min_years="+strconv.Itoa(g.ExperienceMinYears))
	}
	if p.Location != nil && !p.locationMatches(g) {
		hidden = append(hidden, "location")
	}
	if p.Closed == "hide" && g.Closed || p.Closed == "only" && !g.Closed {
		hidden = append(hidden, "closed")
	}
	return hidden
}

// MatchedLocation returns the location string that made g pass p's location rules (M2-08):
// of the first member a rule matches, the first of its locations that the rule matches on
// its own (classified alone), else that member's first location. "" when p has no location
// rules or none matched. Locations are untrusted text: returned as is, only compared.
func (p Preset) MatchedLocation(g Group) string {
	for _, m := range g.Members {
		for _, r := range p.Location {
			if !r.matches(g.Track, m) {
				continue
			}
			for _, loc := range m.Locations {
				f := job.Classify(job.Record{Locations: []string{loc}, Workplace: job.Workplace(m.Workplace)}, "")
				alone := Place{Workplace: m.Workplace, Locations: []string{loc}, RemoteScopes: f.RemoteScopes, Countries: f.Countries}
				if r.matches(g.Track, alone) {
					return loc
				}
			}
			if len(m.Locations) > 0 {
				return m.Locations[0] // matched through the board's country code, not the text
			}
			return ""
		}
	}
	return ""
}

func (p Preset) locationMatches(g Group) bool {
	for _, m := range g.Members {
		for _, r := range p.Location {
			if r.matches(g.Track, m) {
				return true
			}
		}
	}
	return false
}

func (r Rule) matches(track string, m Place) bool {
	return (r.Workplace == nil || slices.Contains(r.Workplace, m.Workplace)) &&
		(r.Track == nil || slices.Contains(r.Track, track)) &&
		(r.RemoteScopes == nil || anyIn(m.RemoteScopes, r.RemoteScopes)) &&
		(r.Countries == nil || anyIn(m.Countries, r.Countries)) &&
		(r.Cities == nil || slices.ContainsFunc(r.Cities, func(c string) bool {
			return slices.ContainsFunc(m.Locations, func(l string) bool { return job.NamesPlace(l, c) })
		}))
}

func anyIn(have, accepted []string) bool {
	return slices.ContainsFunc(have, func(v string) bool { return slices.Contains(accepted, v) })
}

// euEEA is what the EU_EEA alias expands to: the EU's 27 members plus Iceland,
// Liechtenstein and Norway.
var euEEA = strings.Fields("AT BE BG HR CY CZ DK EE FI FR DE GR HU IE IT LV LT LU MT NL PL PT RO SK SI ES SE IS LI NO")

var (
	keys         = []string{"name", "description", "role_family", "seniority", "experience_max_years", "closed", "location"}
	ruleKeys     = []string{"workplace", "remote_scopes", "cities", "countries", "track"}
	workplaces   = []string{"remote", "hybrid", "onsite", "unknown"}
	tracks       = []string{"morocco", "remote", "relocation", "consulting"} // ADR-0005
	remoteScopes = []string{"worldwide", "emea", "europe", "africa", "latam", "americas", "apac", "us_only", "unknown"}
)

// Load reads and validates the preset at path.
func Load(path string) (Preset, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the path is the operator's own config file
	if err != nil {
		return Preset{}, fmt.Errorf("reading preset: %w", err)
	}
	p, err := Parse(b)
	if err != nil {
		return Preset{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Parse validates a preset, reporting every problem with its line, not just the first.
func Parse(b []byte) (Preset, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return Preset{}, fmt.Errorf("parsing YAML: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return Preset{}, errors.New("want a mapping with at least a name")
	}
	var p Preset
	var problems []string
	bad := func(n *yaml.Node, format string, a ...any) {
		problems = append(problems, fmt.Sprintf("line %d: ", n.Line)+fmt.Sprintf(format, a...))
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		switch k.Value {
		case "name":
			p.Name = v.Value
		case "description":
			p.Description = v.Value
		case "role_family":
			p.RoleFamily = list(v, "role_family", job.RoleFamilies, bad)
		case "seniority":
			p.Seniority = list(v, "seniority", job.Seniorities, bad)
		case "experience_max_years":
			if n, err := strconv.Atoi(v.Value); err != nil || v.Kind != yaml.ScalarNode || n < 0 || n > 20 {
				bad(v, "experience_max_years must be a whole number from 0 to 20")
			} else {
				p.ExperienceMaxYears = &n
			}
		case "closed":
			if p.Closed = v.Value; !slices.Contains([]string{"hide", "show", "only"}, p.Closed) {
				bad(v, "closed must be hide, show or only")
			}
		case "location":
			p.Location = rules(v, bad)
		default:
			bad(k, "unknown key %q (want one of %s)", k.Value, strings.Join(keys, ", "))
		}
	}
	if strings.TrimSpace(p.Name) == "" {
		problems = append(problems, "missing name")
	}
	if len(problems) > 0 {
		return Preset{}, fmt.Errorf("%d problems:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return p, nil
}

type reporter func(n *yaml.Node, format string, a ...any)

// list reads a sequence of strings, each one in allowed (nil: any non-empty string).
func list(v *yaml.Node, what string, allowed []string, bad reporter) []string {
	if v.Kind != yaml.SequenceNode {
		bad(v, "%s must be a list", what)
		return nil
	}
	out := []string{}
	for _, e := range v.Content {
		switch {
		case e.Kind != yaml.ScalarNode || strings.TrimSpace(e.Value) == "":
			bad(e, "%s entries must be non-empty strings", what)
		case allowed != nil && !slices.Contains(allowed, e.Value):
			bad(e, "unknown %s %q", what, e.Value)
		default:
			out = append(out, e.Value)
		}
	}
	return out
}

func rules(v *yaml.Node, bad reporter) []Rule {
	if v.Kind != yaml.SequenceNode {
		bad(v, "location must be a list of rules")
		return nil
	}
	out := []Rule{}
	for _, n := range v.Content {
		if n.Kind != yaml.MappingNode || len(n.Content) == 0 {
			bad(n, "empty location rule (it would match everything)")
			continue
		}
		var r Rule
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, val := n.Content[i], n.Content[i+1]
			if !slices.Contains(ruleKeys, k.Value) {
				bad(k, "unknown location key %q (want one of %s)", k.Value, strings.Join(ruleKeys, ", "))
				continue
			}
			vals, problems := RuleValues(k.Value, list(val, k.Value, nil, bad))
			for _, msg := range problems {
				bad(val, "%s", msg)
			}
			r.set(k.Value, vals)
		}
		out = append(out, r)
	}
	return out
}

// RuleValues checks the values of one location rule key ("workplace", "remote_scopes",
// "cities", "countries", "track") and returns them with EU_EEA expanded, plus a message
// for each invalid value. The API uses it for query parameters, so a preset file and a URL
// accept exactly the same values.
func RuleValues(key string, values []string) (out, problems []string) {
	for _, v := range values {
		ok := true
		switch key {
		case "workplace":
			ok = slices.Contains(workplaces, v)
		case "track":
			ok = slices.Contains(tracks, v)
		case "remote_scopes":
			code, isCountry := strings.CutPrefix(v, "country:")
			ok = slices.Contains(remoteScopes, v) || isCountry && isCode(code)
		case "countries":
			switch v {
			case "EU_EEA":
				out = append(out, euEEA...)
				continue
			case "LATAM", "AMERICAS":
				out = append(out, job.RegionCountries(strings.ToLower(v))...)
				continue
			}
			if !isCode(v) {
				problems = append(problems, fmt.Sprintf("country %q is not an ISO alpha-2 code, EU_EEA, LATAM or AMERICAS", v))
				continue
			}
		}
		if !ok {
			problems = append(problems, fmt.Sprintf("unknown %s %q", strings.TrimSuffix(key, "s"), v))
			continue
		}
		out = append(out, v)
	}
	return out, problems
}

func (r *Rule) set(key string, vals []string) {
	switch key {
	case "workplace":
		r.Workplace = vals
	case "track":
		r.Track = vals
	case "cities":
		r.Cities = vals
	case "remote_scopes":
		r.RemoteScopes = vals
	case "countries":
		r.Countries = vals
	}
}

// isCode reports whether s is an upper-case ISO 3166-1 alpha-2 code.
func isCode(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}
