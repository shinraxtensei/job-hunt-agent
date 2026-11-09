package job_test

import (
	"testing"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
)

// M7-03: Remote OK sends names HTML-escaped and as UTF-8 read as Latin-1; both are undone.
// Text that is already right is left alone.
func TestFixText(t *testing.T) {
	for in, want := range map[string]string{
		"Sothebyâ\u0080\u0099s":                   "Sotheby’s",
		"Lead â\u0080\u0094 Logistics & Supply":   "Lead — Logistics & Supply",
		"Transportation Partners &amp; Logistics": "Transportation Partners & Logistics",
		"Société Générale":                        "Société Générale",
		"M&A Praktikum":                           "M&A Praktikum",
		"Ingénieur Ã":                             "Ingénieur Ã", // not valid UTF-8 once repaired: kept
		"&lt;script&gt;":                          "<script>",    // text only, never markup
	} {
		if got := job.FixText(in); got != want {
			t.Errorf("FixText(%q) = %q, want %q", in, got, want)
		}
	}
}
