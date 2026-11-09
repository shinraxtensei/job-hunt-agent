package job_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
)

// benchPayloads are the recorded Greenhouse jobs (descriptions included): the biggest payloads
// the collector stores, so the per-posting numbers are an upper bound.
func benchPayloads(b *testing.B) []json.RawMessage {
	b.Helper()
	body, err := os.ReadFile(filepath.Join("..", "source", "greenhouse", "testdata", "cockroachlabs_jobs.json"))
	if err != nil {
		b.Fatal(err)
	}
	var doc struct {
		Jobs []json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || len(doc.Jobs) == 0 {
		b.Fatalf("golden file: %v", err)
	}
	return doc.Jobs
}

// M8-02 AC2: the per-posting cost of the pure steps the collector runs on a new or
// changed posting.
func BenchmarkNormalise(b *testing.B) {
	ps := benchPayloads(b)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if _, err := job.Normalise("greenhouse", ps[i%len(ps)]); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

func BenchmarkClassify(b *testing.B) {
	ps := benchPayloads(b)
	recs := make([]job.Record, len(ps))
	descs := make([]string, len(ps))
	for i, p := range ps {
		r, err := job.Normalise("greenhouse", p)
		if err != nil {
			b.Fatal(err)
		}
		recs[i], descs[i] = r, job.Description("greenhouse", p)
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		job.Classify(recs[i%len(recs)], descs[i%len(descs)])
		i++
	}
}

func BenchmarkTitleKey(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		job.TitleKey("Senior Backend Engineer (Remote - EMEA) - Payments", []string{"Remote - EMEA", "Berlin, Germany"})
	}
}
