package job

// remoteok normalises a Remote OK job (M7-03, ADR-0029). Every job is remote; location,
// free text and often empty, says where candidates may be. Text fields come escaped or
// mis-decoded (FixText).
func remoteok(m fields) Record {
	return Record{
		Title:       FixText(m.str("position")),
		URL:         m.str("url"),
		Locations:   addLocation(nil, FixText(m.str("location"))),
		Workplace:   Remote,
		PublishedAt: m.time("date"),
	}
}
