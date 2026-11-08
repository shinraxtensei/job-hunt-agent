package httpapi

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/preset"
	"github.com/shinraxtensei/job-hunter/internal/store"
)

// Lead is one duplicate group as the API returns it: the canonical job's record and the
// group's facets.
type Lead struct {
	ID                 int64      `json:"id"` // the canonical posting's id
	Company            Company    `json:"company"`
	Title              string     `json:"title"`
	RoleFamily         string     `json:"role_family"`
	Seniority          string     `json:"seniority"`
	Workplaces         []string   `json:"workplaces"`
	RemoteScopes       []string   `json:"remote_scopes"`
	Countries          []string   `json:"countries"`
	Locations          []string   `json:"locations"`
	MatchedLocation    string     `json:"matched_location"` // the location that passed the location rules; "" if none applied (M2-08)
	ExperienceMinYears *int32     `json:"experience_min_years"`
	Closed             bool       `json:"closed"`
	Team               string     `json:"team"`
	URL                string     `json:"url"`
	PublishedAt        *time.Time `json:"published_at"`
	FirstSeenAt        time.Time  `json:"first_seen_at"` // the group's earliest member
	Origin             string     `json:"origin"`        // collected | manual (added by link, ADR-0015)
	Members            int32      `json:"members"`
	MembersURL         string     `json:"members_url"`
	HiddenBy           []string   `json:"hidden_by,omitempty"` // /api/leads/hidden only

	// The score (M3-08, ADR-0021): null when not scored. Reason and gaps are model output
	// derived from the posting: untrusted, only ever JSON-encoded.
	Score         *float64 `json:"score"` // with freshness as of now
	FitLevel      *string  `json:"fit_level"`
	NotEnoughInfo bool     `json:"not_enough_info"`
	Breakdown     []Factor `json:"breakdown"`
	Reason        *string  `json:"reason"`
	Gaps          []string `json:"gaps"`
	CVVariant     *string  `json:"cv_variant"`
	ReasonCurrent bool     `json:"reason_current"` // false: from older inputs, shown marked
	ScoreStatus   string   `json:"score_status"`   // scored | pending | needs_review | not_queued

	companyID int64
	titleKey  string
	group     preset.Group
}

// Company is a lead's company.
type Company struct {
	Name  string `json:"name"`
	Track string `json:"track"`
}

// load reads every group of an active company, newest first (ADR-0012).
func (a *api) load(ctx context.Context) ([]Lead, error) {
	rows, err := a.q.ListLeads(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing leads: %w", err)
	}
	places, err := a.q.ListJobPlaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing job places: %w", err)
	}
	type key struct {
		company int64
		title   string
	}
	scores, err := a.q.ListLeadScores(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing scores: %w", err)
	}
	byPosting := make(map[int64]store.ListLeadScoresRow, len(scores))
	for _, s := range scores {
		byPosting[s.PostingID] = s
	}
	now := a.now()
	members := map[key][]preset.Place{}
	for _, p := range places {
		k := key{p.CompanyID, p.TitleKey}
		members[k] = append(members[k], preset.Place{Workplace: p.Workplace, Locations: p.Locations,
			RemoteScopes: p.RemoteScopes, Countries: p.Countries})
	}
	out := make([]Lead, 0, len(rows))
	for _, r := range rows {
		l := Lead{
			ID: r.PostingID, Company: Company{r.Company, r.Track}, Title: r.Title,
			RoleFamily: r.RoleFamily, Seniority: r.Seniority, Workplaces: r.Workplaces,
			RemoteScopes: []string{}, Countries: []string{}, Locations: r.Locations,
			Closed: r.Closed, Team: r.Team, URL: r.Url, FirstSeenAt: r.FirstSeenAt.Time.UTC(),
			Origin: r.Origin, Members: r.Members, MembersURL: fmt.Sprintf("/api/leads/%d/members", r.PostingID),
			companyID: r.CompanyID, titleKey: r.TitleKey,
		}
		if r.ExperienceMinYears.Valid {
			l.ExperienceMinYears = &r.ExperienceMinYears.Int32
		}
		if r.PublishedAt.Valid {
			t := r.PublishedAt.Time.UTC()
			l.PublishedAt = &t
		}
		ms := members[key{r.CompanyID, r.TitleKey}]
		for _, m := range ms {
			l.RemoteScopes = union(l.RemoteScopes, m.RemoteScopes)
			l.Countries = union(l.Countries, m.Countries)
		}
		l.group = preset.Group{RoleFamily: r.RoleFamily, Seniority: r.Seniority,
			ExperienceMinYears: int(r.ExperienceMinYears.Int32), Track: r.Track, Closed: r.Closed, Members: ms}
		l.ScoreStatus = "not_queued" // no extract item: the default preset never showed it
		if s, ok := byPosting[l.ID]; ok {
			if err := applyScore(&l, s, now); err != nil {
				return nil, err
			}
		}
		out = append(out, l)
	}
	slices.SortFunc(out, byDate)
	return out, nil
}

func union(have, more []string) []string {
	for _, v := range more {
		if !slices.Contains(have, v) {
			have = append(have, v)
		}
	}
	return have
}

// cursor is the position after the last lead of a page: the sort key of that lead. In
// score order the key starts with the fit level (M3-09: capped levels count) and the
// shown score (unscored: last).
type cursor struct {
	M string  `json:"m,omitempty"` // "score" or "" (date)
	F int     `json:"f,omitempty"` // fit level rank, 0 = very strong
	S float64 `json:"s,omitempty"`
	T int64   `json:"t"` // first seen, Unix microseconds
	C int64   `json:"c"`
	K string  `json:"k"`
}

func cursorOf(l Lead, sort string) string {
	c := cursor{T: l.FirstSeenAt.UnixMicro(), C: l.companyID, K: l.titleKey}
	if sort == "score" {
		c.M, c.F, c.S = sort, levelRank(l), rank(l)
	}
	b, _ := json.Marshal(c) // plain struct: can't fail
	return base64.RawURLEncoding.EncodeToString(b)
}

// rank is a lead's score as shown (rounded), -1 when unscored: what score order sorts by,
// so the order and the cursor agree with the page.
func rank(l Lead) float64 {
	if l.Score == nil {
		return -1
	}
	return *l.Score
}

// levelRank orders fit levels best first; unscored leads come last.
func levelRank(l Lead) int {
	if l.FitLevel == nil {
		return len(FitLevels)
	}
	return slices.Index(FitLevels, *l.FitLevel)
}

// after reports whether l sorts strictly after c: in score order the best fit level, then
// the best score; then newest first, company, title key.
func (c cursor) after(l Lead) bool {
	if c.M == "score" && levelRank(l) != c.F {
		return levelRank(l) > c.F
	}
	if c.M == "score" && rank(l) != c.S {
		return rank(l) < c.S
	}
	t := l.FirstSeenAt.UnixMicro()
	return t < c.T || t == c.T && (l.companyID > c.C || l.companyID == c.C && l.titleKey > c.K)
}

// byDate is load's order: newest first, then company, title key.
func byDate(x, y Lead) int {
	return cmp.Or(y.FirstSeenAt.Compare(x.FirstSeenAt), cmp.Compare(x.companyID, y.companyID),
		cmp.Compare(x.titleKey, y.titleKey))
}

// filters is a request's effective preset plus the filters presets don't have.
type filters struct {
	preset    preset.Preset
	companies []string // lower case
	newOnly   bool
	since     time.Time
	fits      []string // fit levels, or "unscored"
	sort      string   // score (default) | date
	limit     int
	cursor    *cursor
}

var (
	params = []string{"preset", "role_family", "seniority", "max_experience", "closed", "workplace",
		"remote_scope", "country", "city", "track", "company", "new", "since", "limit", "cursor", "sort", "fit"}
	// location parameters and the preset rule keys they set
	locationParams = [][2]string{{"workplace", "workplace"}, {"remote_scope", "remote_scopes"},
		{"country", "countries"}, {"city", "cities"}, {"track", "track"}}
	presetName = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

// values returns a parameter's values, comma-separated or repeated.
func values(q url.Values, k string) []string {
	var out []string
	for _, v := range q[k] {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// parse builds the request's filters: the named preset (default "default"; "none" for no
// filters), each facet parameter overriding the preset's value for that facet.
func (a *api) parse(q url.Values) (filters, error) {
	for k := range q {
		if !slices.Contains(params, k) {
			return filters{}, &paramError{k, "unknown parameter"}
		}
	}
	f := filters{limit: 50, sort: cmp.Or(q.Get("sort"), "score")}
	if f.sort != "score" && f.sort != "date" {
		return filters{}, &paramError{"sort", "want score or date"}
	}
	for _, v := range values(q, "fit") {
		if !slices.Contains(FitLevels, v) && v != "unscored" {
			return filters{}, &paramError{"fit", fmt.Sprintf("unknown value %q", v)}
		}
		f.fits = append(f.fits, v)
	}
	var err error
	if f.preset, err = a.loadPreset(cmp.Or(q.Get("preset"), "default")); err != nil {
		return filters{}, err
	}
	for _, c := range []struct {
		param   string
		allowed []string
		set     *[]string
	}{{"role_family", job.RoleFamilies, &f.preset.RoleFamily}, {"seniority", job.Seniorities, &f.preset.Seniority}} {
		if vs := values(q, c.param); len(vs) > 0 {
			for _, v := range vs {
				if !slices.Contains(c.allowed, v) {
					return filters{}, &paramError{c.param, fmt.Sprintf("unknown value %q", v)}
				}
			}
			*c.set = vs
		}
	}
	if v := q.Get("max_experience"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 20 {
			return filters{}, &paramError{"max_experience", "want a whole number from 0 to 20"}
		}
		f.preset.ExperienceMaxYears = &n
	}
	if v := q.Get("closed"); v != "" {
		if !slices.Contains([]string{"hide", "show", "only"}, v) {
			return filters{}, &paramError{"closed", "want hide, show or only"}
		}
		f.preset.Closed = v
	}
	var rule preset.Rule
	override := false
	for _, lp := range locationParams {
		vs := values(q, lp[0])
		if len(vs) == 0 {
			continue
		}
		ok, problems := preset.RuleValues(lp[1], vs)
		if len(problems) > 0 {
			return filters{}, &paramError{lp[0], problems[0]}
		}
		override = true
		switch lp[1] {
		case "workplace":
			rule.Workplace = ok
		case "remote_scopes":
			rule.RemoteScopes = ok
		case "countries":
			rule.Countries = ok
		case "cities":
			rule.Cities = ok
		case "track":
			rule.Track = ok
		}
	}
	if override { // ADR-0012: location parameters replace the preset's location rules
		f.preset.Location = []preset.Rule{rule}
	}
	for _, c := range values(q, "company") {
		f.companies = append(f.companies, strings.ToLower(c))
	}
	if v := q.Get("new"); v != "" {
		if f.newOnly, err = strconv.ParseBool(v); err != nil {
			return filters{}, &paramError{"new", "want true or false"}
		}
	}
	if v := q.Get("since"); v != "" {
		if f.since, err = time.Parse(time.RFC3339, v); err != nil {
			if f.since, err = time.Parse(time.DateOnly, v); err != nil {
				return filters{}, &paramError{"since", "want a date (2026-11-01) or an RFC 3339 time"}
			}
		}
	}
	if v := q.Get("limit"); v != "" {
		if f.limit, err = strconv.Atoi(v); err != nil || f.limit < 1 || f.limit > 200 {
			return filters{}, &paramError{"limit", "want a whole number from 1 to 200"}
		}
	}
	if v := q.Get("cursor"); v != "" {
		var c cursor
		b, err := base64.RawURLEncoding.DecodeString(v)
		if err != nil || json.Unmarshal(b, &c) != nil || (c.M == "score") != (f.sort == "score") {
			return filters{}, &paramError{"cursor", "not a cursor this API returned"}
		}
		f.cursor = &c
	}
	return f, nil
}

func (a *api) loadPreset(name string) (preset.Preset, error) {
	if name == "none" {
		return preset.Preset{Name: "none"}, nil
	}
	if !presetName.MatchString(name) {
		return preset.Preset{}, &paramError{"preset", "unknown preset"}
	}
	p, err := preset.Load(filepath.Join(a.presetsDir, name+".yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return preset.Preset{}, &paramError{"preset", "unknown preset"}
	}
	return p, err
}

// split applies the filters presets don't have, then the preset: shown leads, and hidden
// ones with the filters that hid them.
func (a *api) split(all []Lead, f filters) (shown, hidden []Lead) {
	for _, l := range all {
		if len(f.companies) > 0 && !slices.Contains(f.companies, strings.ToLower(l.Company.Name)) ||
			f.newOnly && !l.FirstSeenAt.After(a.now().Add(-24*time.Hour)) ||
			!f.since.IsZero() && l.FirstSeenAt.Before(f.since) ||
			len(f.fits) > 0 && !slices.Contains(f.fits, fitOf(l)) {
			continue
		}
		if l.HiddenBy = f.preset.Evaluate(l.group); l.HiddenBy == nil {
			l.MatchedLocation = f.preset.MatchedLocation(l.group)
			shown = append(shown, l)
		} else {
			hidden = append(hidden, l)
		}
	}
	if f.sort == "score" { // level, then score; stable: equal ones stay newest first
		byScore := func(x, y Lead) int {
			return cmp.Or(cmp.Compare(levelRank(x), levelRank(y)), cmp.Compare(rank(y), rank(x)))
		}
		slices.SortStableFunc(shown, byScore)
		slices.SortStableFunc(hidden, byScore)
	}
	return shown, hidden
}

// fitOf is a lead's value for the fit filter and facet.
func fitOf(l Lead) string {
	if l.FitLevel == nil {
		return "unscored"
	}
	return *l.FitLevel
}

type page struct {
	Total      int    `json:"total"`
	Leads      []Lead `json:"leads"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func paginate(ls []Lead, f filters) page {
	p := page{Total: len(ls), Leads: []Lead{}}
	i := 0
	if f.cursor != nil {
		i = len(ls)
		for j, l := range ls {
			if f.cursor.after(l) {
				i = j
				break
			}
		}
	}
	end := min(i+f.limit, len(ls))
	p.Leads = append(p.Leads, ls[i:end]...)
	if end < len(ls) {
		p.NextCursor = cursorOf(ls[end-1], f.sort)
	}
	return p
}

func (a *api) serveLeads(w http.ResponseWriter, r *http.Request, hidden bool) {
	f, err := a.parse(r.URL.Query())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	all, err := a.load(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	shown, hid := a.split(all, f)
	if hidden {
		shown = hid
	}
	writeJSON(w, http.StatusOK, paginate(shown, f))
}

func (a *api) leads(w http.ResponseWriter, r *http.Request)  { a.serveLeads(w, r, false) }
func (a *api) hidden(w http.ResponseWriter, r *http.Request) { a.serveLeads(w, r, true) }

// facets counts each facet's values with that facet's own filter removed and every
// other filter applied, so a count is how many leads choosing that value would show. The
// location facets drop the location rules together (ADR-0012).
func (a *api) facets(w http.ResponseWriter, r *http.Request) {
	f, err := a.parse(r.URL.Query())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	all, err := a.load(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	counts := map[string]map[string]int{}
	for _, c := range []struct {
		facets []string
		drop   func(*filters)
	}{
		{[]string{"role_family"}, func(f *filters) { f.preset.RoleFamily = nil }},
		{[]string{"seniority"}, func(f *filters) { f.preset.Seniority = nil }},
		{[]string{"workplace", "remote_scope", "country", "track"}, func(f *filters) { f.preset.Location = nil }},
		{[]string{"company"}, func(f *filters) { f.companies = nil }},
		{[]string{"closed"}, func(f *filters) { f.preset.Closed = "" }},
		{[]string{"fit"}, func(f *filters) { f.fits = nil }},
	} {
		without := f
		c.drop(&without)
		shown, _ := a.split(all, without)
		for _, facet := range c.facets {
			counts[facet] = map[string]int{}
			for _, l := range shown {
				for _, v := range facetValues(facet, l) {
					counts[facet][v]++
				}
			}
		}
	}
	shown, _ := a.split(all, f)
	writeJSON(w, http.StatusOK, map[string]any{"total": len(shown), "facets": counts})
}

// facetValues are a lead's values for one facet (a group can have several workplaces).
func facetValues(facet string, l Lead) []string {
	switch facet {
	case "role_family":
		return []string{l.RoleFamily}
	case "seniority":
		return []string{l.Seniority}
	case "workplace":
		return l.Workplaces
	case "remote_scope":
		return l.RemoteScopes
	case "country":
		return l.Countries
	case "track":
		return []string{l.Company.Track}
	case "company":
		return []string{l.Company.Name}
	case "closed":
		return []string{strconv.FormatBool(l.Closed)}
	case "fit":
		return []string{fitOf(l)}
	}
	return nil
}

// presets lists data/presets: each one's filters, or why its file is invalid.
func (a *api) presets(w http.ResponseWriter, r *http.Request) {
	files, err := filepath.Glob(filepath.Join(a.presetsDir, "*.yaml"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	type entry struct {
		ID     string         `json:"id"`
		Preset *preset.Preset `json:"preset,omitempty"`
		Error  string         `json:"error,omitempty"`
	}
	out := []entry{}
	for _, file := range files {
		e := entry{ID: strings.TrimSuffix(filepath.Base(file), ".yaml")}
		if p, err := preset.Load(file); err != nil {
			e.Error = err.Error()
		} else {
			e.Preset = &p
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": out})
}

// companies lists every company with its open leads and how many the default preset keeps.
func (a *api) companies(w http.ResponseWriter, r *http.Request) {
	cs, err := a.q.ListCompanies(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	all, err := a.load(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	def, err := a.loadPreset("default")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	open, kept := map[int64]int{}, map[int64]int{}
	for _, l := range all {
		if !l.Closed {
			open[l.companyID]++
		}
		if def.Evaluate(l.group) == nil {
			kept[l.companyID]++
		}
	}
	type company struct {
		Name      string `json:"name"`
		Track     string `json:"track"`
		BoardType string `json:"board_type"`
		BoardSlug string `json:"board_slug"`
		Active    bool   `json:"active"`
		Excluded  bool   `json:"excluded"`
		OpenLeads int    `json:"open_leads"`
		KeptLeads int    `json:"kept_leads"` // shown by the default preset
	}
	out := make([]company, 0, len(cs))
	for _, c := range cs {
		out = append(out, company{c.Name, c.Track, c.BoardType, c.BoardSlug, c.Active, c.Excluded, open[c.ID], kept[c.ID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"companies": out})
}

// members lists every posting in a lead's group, canonical first, then by first seen.
// Any member's posting id finds the group.
func (a *api) members(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id", "want a lead id")
		return
	}
	rows, err := a.q.ListGroupMembers(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "id", "no such lead")
		return
	}
	type member struct {
		ID          int64     `json:"id"`
		Title       string    `json:"title"`
		Team        string    `json:"team"`
		Department  string    `json:"department"`
		Locations   []string  `json:"locations"`
		Workplace   string    `json:"workplace"`
		URL         string    `json:"url"`
		FirstSeenAt time.Time `json:"first_seen_at"`
		Closed      bool      `json:"closed"`
	}
	out := make([]member, 0, len(rows))
	for _, m := range rows {
		out = append(out, member{m.PostingID, m.Title, m.Team, m.Department, m.Locations, m.Workplace, m.Url,
			m.FirstSeenAt.Time.UTC(), m.Closed})
	}
	writeJSON(w, http.StatusOK, map[string]any{"lead_id": rows[0].PostingID, "members": out})
}

// lead answers one lead by its canonical posting id, with its score (M3-08), whatever
// the presets say.
func (a *api) lead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id", "want a lead id")
		return
	}
	all, err := a.load(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	for _, l := range all {
		if l.ID == id {
			writeJSON(w, http.StatusOK, l)
			return
		}
	}
	writeError(w, http.StatusNotFound, "id", "no such lead")
}

// crossOrigin is Go's CSRF protection (fetch metadata, then Origin): it lets same-origin
// and non-browser requests through and refuses cross-origin ones (ADR-0022).
var crossOrigin = http.NewCrossOriginProtection()

// requestReason marks a lead opened so the worker regenerates a reason made from older
// inputs (ADR-0021), behind the write guards (ADR-0022).
func (a *api) requestReason(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, err := a.q.RequestReason(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"requested": n > 0})
}
