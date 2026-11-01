package watchlist_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/watchlist"
)

func TestParseValidFile(t *testing.T) {
	got, err := watchlist.Parse([]byte(`
# comment
- { name: Acme, board_type: lever, board_slug: acme, track: remote }
- name: Beta Co
  board_type: greenhouse
  board_slug: betaco
  track: relocation
  excluded: true
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []watchlist.Entry{
		{Name: "Acme", BoardType: "lever", BoardSlug: "acme", Track: "remote", Line: 3},
		{Name: "Beta Co", BoardType: "greenhouse", BoardSlug: "betaco", Track: "relocation", Excluded: true, Line: 4},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// AC2: every invalid entry is reported with its line, not just the first.
func TestParseListsEveryInvalidLine(t *testing.T) {
	_, err := watchlist.Parse([]byte(`- { name: Good, board_type: lever, board_slug: good, track: remote }
- { name: Bad Board, board_type: bamboohr, board_slug: x, track: remote }
- { name: No Slug, board_type: lever, track: remote }
- { name: Bad Track, board_type: ashby, board_slug: y, track: mars }
- { name: Typo, board_type: lever, board_slug: z, track: remote, exclude: true }
- { board_type: lever, board_slug: noname, track: remote }
- { name: Dup, board_type: lever, board_slug: good, track: remote }
- just a string
`))
	if err == nil {
		t.Fatal("Parse accepted an invalid file")
	}
	msg := err.Error()
	for _, want := range []string{
		`line 2: unknown board_type "bamboohr"`,
		"line 3: missing board_slug",
		`line 4: unknown track "mars"`,
		`line 5: unknown field "exclude"`,
		"line 6: missing name",
		"line 7: duplicate board lever/good (first on line 1)",
		"line 8: entry is not a mapping",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not contain %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "line 1:") {
		t.Errorf("valid line 1 reported as invalid:\n%s", msg)
	}
}

func TestParseRejectsEmptyOrNonList(t *testing.T) {
	for name, src := range map[string]string{
		"empty":       "# only a comment\n",
		"empty list":  "[]\n",
		"not a list":  "name: Acme\n",
		"broken yaml": "- { name: [\n",
	} {
		if _, err := watchlist.Parse([]byte(src)); err == nil {
			t.Errorf("%s: Parse returned no error", name)
		}
	}
}

// AC3: the committed seed file is valid, has at least 50 active (not excluded)
// companies, and lists Yassir as excluded.
func TestSeedFile(t *testing.T) {
	path := filepath.Join("..", "..", "data", "companies.yaml")
	entries, err := watchlist.Load(path)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	active, yassir := 0, false
	for _, e := range entries {
		if !e.Excluded {
			active++
		}
		if e.Name == "Yassir" {
			yassir = e.Excluded
		}
	}
	if active < 50 {
		t.Errorf("seed file has %d active companies, want at least 50", active)
	}
	if !yassir {
		t.Error("Yassir is missing from the seed file or not excluded: true")
	}
}

func TestLoadNamesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companies.yaml")
	if err := os.WriteFile(path, []byte("- { name: X, board_type: nope, board_slug: x, track: remote }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := watchlist.Load(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("Load error = %v, want one naming %s", err, path)
	}
}
