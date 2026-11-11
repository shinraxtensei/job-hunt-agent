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
	offices := []string{m.str("office")}
	for _, e := range m.arr("additionalOffices") {
		var s string
		_ = json.Unmarshal(e, &s) // not a string: skipped
		offices = append(offices, clean(s))
	}
	for _, o := range offices { // the raw values: "Remote" is dropped as a place below
		if strings.Contains(strings.ToLower(o), "remote") {
			r.Workplace = Remote
		}
		r.Locations = addLocation(r.Locations, o)
	}
	return r
}
