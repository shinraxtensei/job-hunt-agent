package job

import (
	"encoding/json"
	"time"
)

// lever normalises a Lever posting (api.lever.co/v0/postings).
func lever(m fields) Record {
	cat := m.obj("categories")
	r := Record{
		Title:          m.str("text"),
		URL:            m.str("hostedUrl"),
		Workplace:      workplace(m.str("workplaceType")),
		Team:           cat.str("team"),
		Department:     cat.str("department"),
		EmploymentType: cat.str("commitment"),
	}
	r.Locations = addLocation(r.Locations, cat.str("location"))
	for _, e := range cat.arr("allLocations") {
		var s string
		_ = json.Unmarshal(e, &s) // a non-string entry is skipped
		r.Locations = addLocation(r.Locations, clean(s))
	}
	if c := m.str("country"); isCountryCode(c) {
		r.CountryCodes = []string{c}
	}
	var ms json.Number
	if json.Unmarshal(m["createdAt"], &ms) == nil {
		if n, err := ms.Int64(); err == nil && n > 0 {
			r.PublishedAt = time.UnixMilli(n)
		}
	}
	return r
}

// isCountryCode reports whether s looks like an ISO 3166-1 alpha-2 code ("US").
func isCountryCode(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}
