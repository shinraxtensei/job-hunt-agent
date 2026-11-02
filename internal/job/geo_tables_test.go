package job_test

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

var codeRe = regexp.MustCompile(`^[A-Z]{2}$`)

// strictRows reads a geo table the way it must be written: the header, then code,value
// rows, every code an upper-case alpha-2 in known (when known is non-nil), every value
// non-empty and unique. The classifier's own reader stops at the first malformed row, so
// this test is what keeps a typo from silently dropping the rest of a table.
func strictRows(b []byte, header string, known map[string]bool) (map[string]string, error) {
	r := csv.NewReader(bytes.NewReader(b))
	r.Comment = '#'
	r.FieldsPerRecord = 2
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}
	if strings.Join(head, ",") != header {
		return nil, fmt.Errorf("header %q, want %q", strings.Join(head, ","), header)
	}
	rows := map[string]string{} // value → code
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		line, _ := r.FieldPos(0)
		code, value := rec[0], strings.TrimSpace(rec[1])
		switch {
		case !codeRe.MatchString(code):
			return nil, fmt.Errorf("line %d: %q is not an alpha-2 code", line, code)
		case known != nil && !known[code]:
			return nil, fmt.Errorf("line %d: unknown code %q", line, code)
		case value == "" || value != rec[1]:
			return nil, fmt.Errorf("line %d: empty or padded value %q", line, rec[1])
		}
		key := strings.ToLower(value)
		if prev, dup := rows[key]; dup {
			return nil, fmt.Errorf("line %d: %q repeats (also %s)", line, value, prev)
		}
		rows[key] = code
	}
}

// The checked-in tables are strict: any malformed row, unknown code or empty alias fails.
func TestCountryTablesAreStrict(t *testing.T) {
	iso, err := os.ReadFile("geo/iso3166-1.csv")
	if err != nil {
		t.Fatal(err)
	}
	names, err := strictRows(iso, "code,name", nil)
	if err != nil {
		t.Fatalf("iso3166-1.csv: %v", err)
	}
	known := map[string]bool{}
	for _, c := range names {
		known[c] = true
	}
	if len(known) != 249 || len(names) != 249 {
		t.Fatalf("iso3166-1.csv: %d codes, %d names; want 249 each", len(known), len(names))
	}
	aliases, err := os.ReadFile("geo/aliases.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := strictRows(aliases, "code,alias", known); err != nil {
		t.Fatalf("aliases.csv: %v", err)
	}
}

// The strict reader itself rejects each kind of mistake.
func TestStrictRowsRejects(t *testing.T) {
	known := map[string]bool{"MA": true, "FR": true}
	for name, body := range map[string]string{
		"three fields": "code,alias\nMA,Maroc,extra\n",
		"one field":    "code,alias\nMA\n",
		"lower code":   "code,alias\nma,Maroc\n",
		"unknown code": "code,alias\nZZ,Nowhere\n",
		"empty alias":  "code,alias\nMA,\n",
		"padded alias": "code,alias\nMA, Maroc\n",
		"duplicate":    "code,alias\nMA,Maroc\nFR,maroc\n",
		"bad header":   "code,name\nMA,Maroc\n",
		"broken quote": "code,alias\nMA,\"Maroc\n",
	} {
		if _, err := strictRows([]byte(body), "code,alias", known); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := strictRows([]byte("# comment\ncode,alias\nMA,Maroc\nFR,France\n"), "code,alias", known); err != nil {
		t.Errorf("a good table was rejected: %v", err)
	}
}
