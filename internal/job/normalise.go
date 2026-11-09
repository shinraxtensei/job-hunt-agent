package job

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Workplace is where the work happens, as the board states it (ADR-0008).
type Workplace string

// The four workplace values; the jobs table CHECK allows exactly these.
const (
	Remote  Workplace = "remote"
	Hybrid  Workplace = "hybrid"
	Onsite  Workplace = "onsite"
	Unknown Workplace = "unknown"
)

// NormaliserVersion is stored on every jobs row. Bump it whenever Normalise's output
// for the same payload changes; `collector normalise` then rebuilds older rows.
const NormaliserVersion = 2 // 2: title_key (M2-02, ADR-0009)

// maxField caps every string copied from a payload (Security notes: untrusted input).
const maxField = 500

// Record is one posting in the common shape, whichever board it came from. Empty text
// means the board didn't say; PublishedAt is zero when unknown.
type Record struct {
	Title          string
	URL            string
	Locations      []string // as published, primary first, no duplicates
	CountryCodes   []string // ISO 3166-1 alpha-2, only where the board gives codes
	Workplace      Workplace
	Team           string
	Department     string
	EmploymentType string // as published; classified later (M2-03)
	PublishedAt    time.Time
	TitleKey       string // groups duplicates within a company (ADR-0009)
}

// Normalise turns one stored posting payload into a Record. It is pure and never
// panics. A missing, null or wrongly typed field leaves that field empty or Unknown;
// only a payload that isn't a JSON object, or an unknown source, is an error.
func Normalise(source string, payload []byte) (Record, error) {
	var m fields
	if err := json.Unmarshal(payload, &m); err != nil || m == nil {
		return Record{}, errors.New("payload is not a JSON object")
	}
	var r Record
	switch source {
	case "lever":
		r = lever(m)
	case "greenhouse":
		r = greenhouse(m)
	case "ashby":
		r = ashby(m)
	case "personio":
		r = personio(m)
	case "remotive":
		r = remotive(m)
	case "manual":
		r = manual(m)
	default:
		return Record{}, fmt.Errorf("no normaliser for source %q", source)
	}
	if r.Workplace == "" {
		r.Workplace = Unknown
	}
	r.TitleKey = TitleKey(r.Title, r.Locations)
	return r, nil
}

// fields is a JSON object whose values are read leniently.
type fields map[string]json.RawMessage

// obj returns m[k] as an object, or an empty one.
func (m fields) obj(k string) fields {
	var o fields
	_ = json.Unmarshal(m[k], &o) // wrong type or missing: empty object
	return o
}

// arr returns m[k] as an array, or nil.
func (m fields) arr(k string) []json.RawMessage {
	var a []json.RawMessage
	_ = json.Unmarshal(m[k], &a) // wrong type or missing: no elements
	return a
}

// str returns m[k] as a cleaned string, or "".
func (m fields) str(k string) string {
	var s string
	_ = json.Unmarshal(m[k], &s) // wrong type or missing: empty
	return clean(s)
}

// time parses m[k] as an RFC 3339 timestamp, or returns zero.
func (m fields) time(k string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, m.str(k))
	if err != nil {
		return time.Time{}
	}
	return t
}

// elemObj decodes one array element as an object, or an empty one.
func elemObj(e json.RawMessage) fields {
	var o fields
	_ = json.Unmarshal(e, &o) // wrong type: empty object
	return o
}

// clean trims s, drops NUL characters and caps it at maxField runes.
func clean(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
	if r := []rune(s); len(r) > maxField {
		s = string(r[:maxField])
	}
	return s
}

// addLocation appends loc unless it is empty or already present.
func addLocation(locs []string, loc string) []string {
	if loc == "" {
		return locs
	}
	for _, l := range locs {
		if l == loc {
			return locs
		}
	}
	return append(locs, loc)
}

// workplace maps a board's stated workplace value (any case) to a Workplace.
func workplace(v string) Workplace {
	switch strings.ToLower(v) {
	case "remote":
		return Remote
	case "hybrid":
		return Hybrid
	case "onsite", "on-site":
		return Onsite
	default:
		return Unknown
	}
}

// manual normalises a posting typed by hand (M2-07, ADR-0015): the payload is what the
// owner typed. The workplace is read from the location words, like Greenhouse's.
func manual(m fields) Record {
	loc := m.str("location")
	r := Record{Title: m.str("title"), URL: m.str("url"), Workplace: greenhouseWorkplace(loc)}
	r.Locations = addLocation(r.Locations, loc)
	return r
}
