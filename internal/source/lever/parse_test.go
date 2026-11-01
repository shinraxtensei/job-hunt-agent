package lever

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/source/fetch"
)

// AC4: parse never panics, and whatever it returns is storable: every record has an
// ID and a URL, carries valid JSON with no NUL escape, and records plus skipped
// entries account for every element of the list.
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
	f.Add([]byte(`[{"id":"x","hostedUrl":"u","text":"\\u0000"}]`)) // escaped backslash: kept
	f.Fuzz(func(t *testing.T, body []byte) {
		raws, skipped, err := parse(body)
		if err != nil {
			return
		}
		var elems []json.RawMessage
		if json.Unmarshal(body, &elems) != nil {
			t.Fatalf("parse accepted input that is not a JSON array")
		}
		if len(raws)+skipped != len(elems) {
			t.Fatalf("%d records + %d skipped != %d elements", len(raws), skipped, len(elems))
		}
		for _, r := range raws {
			if r.SourceID == "" || r.URL == "" || !json.Valid(r.Payload) || fetch.HasNUL(r.Payload) {
				t.Fatalf("unstorable record: %+v", r)
			}
		}
	})
}
