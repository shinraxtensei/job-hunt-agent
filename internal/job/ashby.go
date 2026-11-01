package job

// ashby normalises an Ashby job (api.ashbyhq.com/posting-api). workplaceType decides
// the workplace; isRemote is ignored because it is true for hybrid jobs too (ADR-0008).
func ashby(m fields) Record {
	r := Record{
		Title:          m.str("title"),
		URL:            m.str("jobUrl"),
		Workplace:      workplace(m.str("workplaceType")),
		Team:           m.str("team"),
		Department:     m.str("department"),
		EmploymentType: m.str("employmentType"),
		PublishedAt:    m.time("publishedAt"),
	}
	r.Locations = addLocation(r.Locations, m.str("location"))
	for _, e := range m.arr("secondaryLocations") {
		r.Locations = addLocation(r.Locations, elemObj(e).str("location"))
	}
	return r
}
