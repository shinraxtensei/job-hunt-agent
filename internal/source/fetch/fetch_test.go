package fetch_test

import (
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/source/fetch"
)

func TestHasNUL(t *testing.T) {
	for in, want := range map[string]bool{
		`{"a":"x\u0000y"}`:   true,  // a real NUL
		`{"a":"\\u0000"}`:    false, // escaped backslash, then the text u0000
		`{"a":"\\\u0000"}`:   true,  // escaped backslash, then a real NUL
		`{"a":"\u00000"}`:    true,  // NUL followed by "0"
		`{"a":"\u0001"}`:     false, // another control character: Postgres accepts it
		`{"a":"u0000"}`:      false, // no backslash at all
		`{"a":"plain text"}`: false,
	} {
		if got := fetch.HasNUL([]byte(in)); got != want {
			t.Errorf("fetch.HasNUL(%s) = %v, want %v", in, got, want)
		}
	}
}
