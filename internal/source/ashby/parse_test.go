package ashby

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shinraxtensei/job-hunt-agent/internal/source/fetch"
)

// AC4: parse never panics, and whatever it returns is storable: every record has an
// ID and a URL, carries valid JSON with no NUL escape, and no job is counted twice
// (records + skipped never exceed the jobs in the document; unlisted jobs are neither).
func FuzzParse(f *testing.F) {
	seeds, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil || len(seeds) == 0 {
		f.Fatalf("no seed files: %v", err)
	}
	for _, s := range seeds {
		b, err := os.ReadFile(s) //nolint:gosec // test fixture path from Glob
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		raws, skipped, err := parse(body)
		if err != nil {
			return
		}
		var doc struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if json.Unmarshal(body, &doc) != nil {
			t.Fatalf("parse accepted input that is not a jobs document")
		}
		if len(raws)+skipped.Total() > len(doc.Jobs) {
			t.Fatalf("%d records + %d skipped > %d jobs", len(raws), skipped.Total(), len(doc.Jobs))
		}
		for _, r := range raws {
			if r.SourceID == "" || r.URL == "" || !json.Valid(r.Payload) || fetch.HasNUL(r.Payload) {
				t.Fatalf("unstorable record: %+v", r)
			}
		}
	})
}
