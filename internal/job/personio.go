package job

import (
	"encoding/json"
	"strings"
)

// personio normalises a Personio feed position (M7-02), stored by the adapter as a JSON
// object of the feed's fields. The feed states no workplace: an office named "Remote" is
// read as remote, anything else stays unknown for the classifier.
func personio(m fields) Record {
	r := Record{
		Title:          m.str("name"),
		URL:            m.str("url"),
		Team:           m.str("subcompany"),
		Department:     m.str("department"),
		EmploymentType: m.str("employmentType"),
		PublishedAt:    m.time("createdAt"),
		Workplace:      Unknown,
	}
	r.Locations = addLocation(r.Locations, m.str("office"))
	for _, e := range m.arr("additionalOffices") {
		var s string
		_ = json.Unmarshal(e, &s) // not a string: skipped
		r.Locations = addLocation(r.Locations, clean(s))
	}
	for _, l := range r.Locations {
		if strings.Contains(strings.ToLower(l), "remote") {
			r.Workplace = Remote
		}
	}
	return r
}
