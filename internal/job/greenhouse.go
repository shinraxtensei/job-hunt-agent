package job

import "strings"

// greenhouse normalises a Greenhouse job (boards-api.greenhouse.io). Greenhouse has no
// workplace field: it is read only from explicit words in the location text, and onsite
// is never guessed (ADR-0008).
func greenhouse(m fields) Record {
	loc := m.obj("location").str("name")
	r := Record{
		Title:       m.str("title"),
		URL:         m.str("absolute_url"),
		Workplace:   greenhouseWorkplace(loc),
		PublishedAt: m.time("first_published"),
	}
	r.Locations = addLocation(r.Locations, loc)
	for _, e := range m.arr("offices") {
		o := elemObj(e)
		l := o.str("location")
		if l == "" {
			l = o.str("name") // offices without a location still name a place
		}
		r.Locations = addLocation(r.Locations, l)
	}
	if d := m.arr("departments"); len(d) > 0 {
		r.Department = elemObj(d[0]).str("name")
	}
	return r
}

// greenhouseWorkplace: "remote"/"anywhere" → Remote, "hybrid" → Hybrid, both or neither
// → Unknown.
func greenhouseWorkplace(loc string) Workplace {
	l := strings.ToLower(loc)
	remote := strings.Contains(l, "remote") || strings.Contains(l, "anywhere")
	hybrid := strings.Contains(l, "hybrid")
	switch {
	case remote && !hybrid:
		return Remote
	case hybrid && !remote:
		return Hybrid
	default:
		return Unknown
	}
}
