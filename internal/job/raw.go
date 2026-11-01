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
