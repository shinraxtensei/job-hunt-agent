package collector_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/preset"
)

// AC5 + AC7: the preview counts shown and hidden groups by filter and samples both, with
// reasons; titles are quoted, so control characters in a posting can't reach the terminal.
func TestPreview(t *testing.T) {
	p, err := preset.Parse([]byte("name: t\nrole_family: [backend]\nclosed: hide\n"))
	if err != nil {
		t.Fatal(err)
	}
	gs := []collector.GroupRow{
		{Company: "A", Title: "Backend Engineer", Group: preset.Group{RoleFamily: "backend"}},
		{Company: "B", Title: "Account Executive\x1b[2J", Group: preset.Group{RoleFamily: "sales"}},
		{Company: "C", Title: "Closed Sales Role", Group: preset.Group{RoleFamily: "sales", Closed: true}},
	}
	var b strings.Builder
	collector.Preview(&b, p, gs, 10, rand.New(rand.NewPCG(1, 2)))
	out := b.String()
	for _, want := range []string{
		"3 groups: 1 shown, 2 hidden",
		"role_family", "2 (only this: 1)",
		"closed", "1 (only this: 0)",
		`"Backend Engineer" (A)`,
		`"Account Executive\x1b[2J" (B): role_family=sales`,
		"role_family=sales, closed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("preview lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b") {
		t.Error("raw control character in output")
	}
}
