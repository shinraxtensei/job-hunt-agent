package remoteok

import (
	"os"
	"path/filepath"
	"testing"
)

// M8-02 AC1: parsing one recorded response (api.json).
func BenchmarkParse(b *testing.B) {
	body, err := os.ReadFile(filepath.Join("testdata", "api.json"))
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	n := 0
	for b.Loop() {
		raws, _, err := parse(body)
		if err != nil {
			b.Fatal(err)
		}
		n = len(raws)
	}
	b.ReportMetric(float64(n), "postings/op")
}
