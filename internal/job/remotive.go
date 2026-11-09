package job

import (
	"strings"
	"time"
)

// remotive normalises a Remotive job (M7-03, ADR-0029). Every Remotive job is remote;
// candidate_required_location ("Worldwide", "Europe, USA") says where candidates may be,
// one location per comma-separated part. publication_date has no zone: UTC.
func remotive(m fields) Record {
	r := Record{
		Title:          m.str("title"),
		URL:            m.str("url"),
		Department:     m.str("category"),
		EmploymentType: m.str("job_type"),
		Workplace:      Remote,
	}
	if t, err := time.Parse("2006-01-02T15:04:05", m.str("publication_date")); err == nil {
		r.PublishedAt = t
	}
	for _, l := range strings.Split(m.str("candidate_required_location"), ",") {
		r.Locations = addLocation(r.Locations, clean(l))
	}
	return r
}
