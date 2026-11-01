package job_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shinraxtensei/job-hunter/internal/job"
)

// payloads returns each posting in an adapter's golden file, keyed by its board ID. The
// file is in the board's own response format: a bare array (Lever) or {"jobs": [...]}.
func payloads(t *testing.T, board, file string) map[string][]byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "source", board, "testdata", file))
	if err != nil {
		t.Fatalf("reading %s/%s: %v", board, file, err)
	}
	var elems []json.RawMessage
	if json.Unmarshal(b, &elems) != nil {
		var doc struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("decoding %s/%s: %v", board, file, err)
		}
		elems = doc.Jobs
	}
	out := map[string][]byte{}
	for _, e := range elems {
		var id struct {
			ID json.Number `json:"id"`
		}
		if err := json.Unmarshal(e, &id); err != nil {
			var sid struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(e, &sid) // fixtures always carry an id
			id.ID = json.Number(sid.ID)
		}
		out[id.ID.String()] = e
	}
	return out
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t
}

func check(t *testing.T, source string, payload []byte, want job.Record) {
	t.Helper()
	got, err := job.Normalise(source, payload)
	if err != nil {
		t.Fatalf("Normalise: %v", err)
	}
	if got.Title != want.Title || got.URL != want.URL || got.Workplace != want.Workplace ||
		got.Team != want.Team || got.Department != want.Department || got.EmploymentType != want.EmploymentType ||
		!got.PublishedAt.Equal(want.PublishedAt) || !slices.Equal(got.Locations, want.Locations) ||
		!slices.Equal(got.CountryCodes, want.CountryCodes) {
		t.Errorf("Normalise =\n  %+v\nwant\n  %+v", got, want)
	}
}

// AC1 + AC2: Lever golden files (hybrid, onsite, remote, multi-location).
func TestNormaliseLever(t *testing.T) {
	p := payloads(t, "lever", "spotify_postings.json")
	for k, v := range payloads(t, "lever", "normalise_cases.json") {
		p[k] = v
	}
	for name, c := range map[string]struct {
		id   string
		want job.Record
	}{
		"hybrid": {"2a55d6c9-c564-443a-be3e-a3851db5a93b", job.Record{
			Title: "Account Executive - Commercial, Spotify Technology", URL: "https://jobs.lever.co/spotify/2a55d6c9-c564-443a-be3e-a3851db5a93b",
			Locations: []string{"New York, NY"}, CountryCodes: []string{"US"}, Workplace: job.Hybrid,
			Team: "Platform", Department: "Operations and Business Support", EmploymentType: "Permanent",
			PublishedAt: time.UnixMilli(1792767640768)}},
		"onsite": {"785989a6-b219-43b1-92d9-750a67054f24", job.Record{
			Title: "AV Engineer - Platform", URL: "https://jobs.lever.co/spotify/785989a6-b219-43b1-92d9-750a67054f24",
			Locations: []string{"Stockholm"}, CountryCodes: []string{"SE"}, Workplace: job.Onsite,
			Team: "IT and Network Engineering", Department: "Engineering", EmploymentType: "Permanent",
			PublishedAt: time.UnixMilli(1791537508142)}},
		"remote": {"4310f31c-90e1-4e8d-bdcf-dce39c145b8c", job.Record{
			Title: "Associate – Audiobook Licensing & Author Partnerships", URL: "https://jobs.lever.co/spotify/4310f31c-90e1-4e8d-bdcf-dce39c145b8c",
			Locations: []string{"New York, NY"}, CountryCodes: []string{"US"}, Workplace: job.Remote,
			Team: "Audio Books & Gated Content", Department: "Operations and Business Support", EmploymentType: "Permanent",
			PublishedAt: time.UnixMilli(1783608935950)}},
		"multi-location": {"cf13d272-92ac-407d-89bf-7f20d20f7961", job.Record{
			Title: "Account Executive - Enterprise, Spotify Technology", URL: "https://jobs.lever.co/spotify/cf13d272-92ac-407d-89bf-7f20d20f7961",
			Locations: []string{"New York, NY", "San Francisco, CA"}, CountryCodes: []string{"US"}, Workplace: job.Hybrid,
			Team: "Platform", Department: "Operations and Business Support", EmploymentType: "Permanent",
			PublishedAt: time.UnixMilli(1792767393133)}},
	} {
		t.Run(name, func(t *testing.T) { check(t, "lever", p[c.id], c.want) })
	}
}

// AC1 + AC2: Greenhouse golden files. Workplace comes only from explicit words in the
// location text; offices add locations (their name when the location is null).
func TestNormaliseGreenhouse(t *testing.T) {
	p := payloads(t, "greenhouse", "cockroachlabs_jobs.json")
	for k, v := range payloads(t, "greenhouse", "normalise_cases.json") {
		p[k] = v
	}
	for name, c := range map[string]struct {
		id   string
		want job.Record
	}{
		"office city, no workplace word": {"8079608", job.Record{
			Title: "Deal Desk Manager, AMER", URL: "https://www.cockroachlabs.com/careers/job/?gh_jid=8079608",
			Locations: []string{"New York, NY", "New York, NY, United States"}, Workplace: job.Unknown,
			Department: "Sales", PublishedAt: at("2026-07-28T13:56:58-04:00")}},
		"remote (trailing space trimmed)": {"4463390009", job.Record{
			Title: "AI Developer Experience Engineer", URL: "https://job-boards.greenhouse.io/planetscale/jobs/4463390009",
			Locations: []string{"Remote US", "Remote - US"}, Workplace: job.Remote,
			Department: "Marketing", PublishedAt: at("2026-10-21T18:57:46-04:00")}},
		"hybrid": {"5999792004", job.Record{
			Title: "Account Executive, Majors", URL: "https://job-boards.greenhouse.io/vercel/jobs/5999792004",
			Locations: []string{"Hybrid - London", "London, England, United Kingdom"}, Workplace: job.Hybrid,
			Department: "Account Executive", PublishedAt: at("2026-05-21T09:26:09-04:00")}},
		"multi-location (office names)": {"5010674101", job.Record{
			Title: "E&S and OHS Manager - Group - Jumia ( Fulltime)", URL: "https://job-boards.eu.greenhouse.io/jumia/jobs/5010674101",
			Locations: []string{"Africa", "Ghana", "Ivory Coast", "Kenya", "Nigeria"}, Workplace: job.Unknown,
			Department: "Supply Chain", PublishedAt: at("2026-10-21T07:02:44-04:00")}},
	} {
		t.Run(name, func(t *testing.T) { check(t, "greenhouse", p[c.id], c.want) })
	}
}

// AC1 + AC2: Ashby golden files. workplaceType decides; isRemote is ignored.
func TestNormaliseAshby(t *testing.T) {
	p := payloads(t, "ashby", "linear_jobs.json")
	for k, v := range payloads(t, "ashby", "normalise_cases.json") {
		p[k] = v
	}
	for name, c := range map[string]struct {
		id   string
		want job.Record
	}{
		"remote": {"d3bc1ced-3ce4-4086-a050-555055dbb1ff", job.Record{
			Title: "Senior / Staff Fullstack Engineer", URL: "https://jobs.ashbyhq.com/linear/d3bc1ced-3ce4-4086-a050-555055dbb1ff",
			Locations: []string{"Europe"}, Workplace: job.Remote, Team: "Engineering", Department: "Product",
			EmploymentType: "FullTime", PublishedAt: at("2021-04-27T20:13:45.158+00:00")}},
		"hybrid (team trailing space trimmed)": {"30600762-62fe-4ffb-9b6e-98f145329ea3", job.Record{
			Title: "Technical Executive Communications, Senior Manager", URL: "https://jobs.ashbyhq.com/temporal/30600762-62fe-4ffb-9b6e-98f145329ea3",
			Locations: []string{"San Francisco, California", "Seattle, Washington"}, Workplace: job.Hybrid,
			Team: "Marketing", Department: "Marketing", EmploymentType: "FullTime", PublishedAt: at("2026-09-02T17:39:36.525+00:00")}},
		"onsite": {"1464036f-94d5-4dbd-aef8-fe99246a26d4", job.Record{
			Title: "Site Reliability Engineer (US - Pacific time)", URL: "https://jobs.ashbyhq.com/posthog/1464036f-94d5-4dbd-aef8-fe99246a26d4",
			Locations: []string{"San Francisco, CA", "Remote (US)"}, Workplace: job.Onsite,
			Team: "Cloud Foundations", Department: "Engineering", EmploymentType: "FullTime", PublishedAt: at("2026-08-06T08:48:53.622+00:00")}},
		"multi-location": {"915faccb-5bf1-4a1c-9a2d-ccad28fd48f7", job.Record{
			Title: "Senior Developer Success Engineer - West", URL: "https://jobs.ashbyhq.com/temporal/915faccb-5bf1-4a1c-9a2d-ccad28fd48f7",
			Locations: []string{"United States (Remote)", "Los Angeles, California", "Seattle, Washington", "San Francisco, California", "Portland, Oregon"},
			Workplace: job.Remote, Team: "Customer Support", Department: "Customer Support", EmploymentType: "FullTime",
			PublishedAt: at("2026-06-12T18:12:56.259+00:00")}},
		"no workplaceType → unknown": {"db498a8c-c8d8-405b-b86a-0943a5122e7d", job.Record{
			Title: "Senior Cloud Software Engineer - Efficiency Engineering", URL: "https://jobs.ashbyhq.com/clickhouse/db498a8c-c8d8-405b-b86a-0943a5122e7d",
			Locations: []string{"NAMER and LatAm"}, Workplace: job.Unknown, Team: "Cloud Engineering", Department: "Engineering",
			EmploymentType: "FullTime", PublishedAt: at("2026-08-13T06:40:24.278+00:00")}},
	} {
		t.Run(name, func(t *testing.T) { check(t, "ashby", p[c.id], c.want) })
	}
}

// ADR-0008: "Remote or Hybrid" is ambiguous, so unknown; never guess onsite.
func TestNormaliseGreenhouseWorkplaceWords(t *testing.T) {
	for loc, want := range map[string]job.Workplace{
		"Remote or Hybrid - Berlin": job.Unknown,
		"Anywhere":                  job.Remote,
		"REMOTE - EMEA":             job.Remote,
		"Hybrid":                    job.Hybrid,
		"Office Based - London, UK": job.Unknown,
		"":                          job.Unknown,
	} {
		got, err := job.Normalise("greenhouse", []byte(`{"title":"x","location":{"name":`+jsonString(loc)+`}}`))
		if err != nil || got.Workplace != want {
			t.Errorf("location %q: workplace = %q, %v; want %q", loc, got.Workplace, err, want)
		}
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s) // marshalling a string cannot fail
	return string(b)
}

// AC3: missing, null or wrongly typed fields become empty / unknown; the record is
// still produced.
func TestNormaliseToleratesBadFields(t *testing.T) {
	for source, payload := range map[string]string{
		"lever":      `{"text": 42, "hostedUrl": null, "categories": "nope", "workplaceType": ["remote"], "country": "usa", "createdAt": "yesterday"}`,
		"greenhouse": `{"title": null, "location": "Remote", "offices": [null, 3, {"location": 7}], "departments": {}, "first_published": "not a date"}`,
		"ashby":      `{"title": {}, "location": 5, "secondaryLocations": [{"location": null}, "x"], "workplaceType": 1, "publishedAt": 0}`,
	} {
		got, err := job.Normalise(source, []byte(payload))
		if err != nil {
			t.Errorf("%s: Normalise failed: %v (bad fields must not fail the record)", source, err)
			continue
		}
		if got.Title != "" || got.URL != "" || got.Workplace != job.Unknown || len(got.Locations) != 0 ||
			len(got.CountryCodes) != 0 || !got.PublishedAt.IsZero() || got.Team != "" || got.Department != "" {
			t.Errorf("%s: got %+v, want every field empty / unknown", source, got)
		}
	}
}

// AC3: only a payload that isn't a JSON object, or an unknown source, is an error.
func TestNormaliseErrors(t *testing.T) {
	for _, c := range []struct{ source, payload string }{
		{"lever", `[1, 2]`}, {"lever", `"text"`}, {"lever", `{`}, {"workable", `{}`},
	} {
		if _, err := job.Normalise(c.source, []byte(c.payload)); err == nil {
			t.Errorf("Normalise(%s, %s) returned no error", c.source, c.payload)
		}
	}
}

// Security notes: copied strings are capped at 500 characters (by rune, still valid
// UTF-8); locations are de-duplicated.
func TestNormaliseCapsAndDedupes(t *testing.T) {
	long := strings.Repeat("é", 600)
	got, err := job.Normalise("ashby", []byte(`{"title":`+jsonString(long)+`,"location":"Paris","secondaryLocations":[{"location":"Paris"},{"location":`+jsonString(long)+`}]}`))
	if err != nil {
		t.Fatalf("Normalise: %v", err)
	}
	if n := len([]rune(got.Title)); n != 500 {
		t.Errorf("title is %d runes, want 500", n)
	}
	if len(got.Locations) != 2 || got.Locations[0] != "Paris" || len([]rune(got.Locations[1])) != 500 {
		t.Errorf("locations = %d entries, want Paris once plus the capped long one", len(got.Locations))
	}
}

// AC3: never panics, and never errors on a JSON object, whatever its fields hold.
func FuzzNormalise(f *testing.F) {
	for _, board := range []string{"lever", "greenhouse", "ashby"} {
		files, _ := filepath.Glob(filepath.Join("..", "source", board, "testdata", "*.json")) // pattern is constant
		for _, file := range files {
			b, err := os.ReadFile(file) //nolint:gosec // test fixture path from Glob
			if err != nil {
				f.Fatal(err)
			}
			var elems []json.RawMessage
			if json.Unmarshal(b, &elems) != nil {
				var doc struct {
					Jobs []json.RawMessage `json:"jobs"`
				}
				_ = json.Unmarshal(b, &doc) // a fixture that isn't a jobs doc just adds no seeds
				elems = doc.Jobs
			}
			for _, e := range elems {
				f.Add(board, []byte(e))
			}
		}
	}
	f.Fuzz(func(t *testing.T, source string, payload []byte) {
		rec, err := job.Normalise(source, payload)
		var obj map[string]json.RawMessage
		isObject := json.Unmarshal(payload, &obj) == nil && obj != nil
		known := source == "lever" || source == "greenhouse" || source == "ashby"
		if known && isObject && err != nil {
			t.Fatalf("Normalise(%s) failed on a JSON object: %v", source, err)
		}
		if err != nil {
			return
		}
		switch rec.Workplace {
		case job.Remote, job.Hybrid, job.Onsite, job.Unknown:
		default:
			t.Fatalf("workplace %q is not one of the four values", rec.Workplace)
		}
		for _, s := range append([]string{rec.Title, rec.URL, rec.Team, rec.Department, rec.EmploymentType}, rec.Locations...) {
			if len([]rune(s)) > 500 || strings.ContainsRune(s, 0) {
				t.Fatalf("copied string over 500 runes or containing NUL: %q", s)
			}
		}
	})
}
