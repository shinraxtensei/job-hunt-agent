// Package job is the domain: job records, hard rules and dedup. Pure; no I/O.
package job

import "encoding/json"

// Raw is one posting as a source returned it: the payload bytes exactly as received,
// plus the few fields every source must supply to store it. Postings are untrusted
// input; Raw carries them as data and nothing here interprets the payload.
type Raw struct {
	Source   string          // adapter name, e.g. "lever"
	SourceID string          // the source's own posting ID
	URL      string          // public posting page
	Title    string          // posting title as published
	Payload  json.RawMessage // the posting's original JSON, byte for byte
}

// Skipped describes the entries a source dropped as malformed or unstorable. IDs lists
// those whose ID could still be read: they are on the board, so they must count as seen
// (never closed, ADR-0010). Unreadable counts the rest.
type Skipped struct {
	IDs        []string
	Unreadable int
}

// Add records a skipped entry; an empty id counts as unreadable.
func (s *Skipped) Add(id string) {
	if id == "" {
		s.Unreadable++
		return
	}
	s.IDs = append(s.IDs, id)
}

// Total is how many entries were skipped.
func (s Skipped) Total() int { return len(s.IDs) + s.Unreadable }
