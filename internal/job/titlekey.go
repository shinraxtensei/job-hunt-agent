package job

import (
	"strings"
	"unicode"
)

// TitleKey is the normalised title used to group duplicate jobs within a company
// (ADR-0009): accents folded, lowercase, punctuation and whitespace collapsed, and trailing
// segments removed only when they are a work mode, region or gender marker from a fixed
// vocabulary, or a place named in the record's own locations. Seniority words, teams,
// stacks and territories stay.
//
// Titles are untrusted: they are only split and compared as text. No pattern is ever built
// from a title, and a title is never reduced to nothing.
func TitleKey(title string, locations []string) string {
	var locs [][]string
	for _, l := range locations {
		if t := tokens(l); len(t) > 0 {
			locs = append(locs, t)
		}
	}
	s := strings.TrimSpace(title)
	for stripped := true; stripped; {
		stripped = false
		// A final "(...)" whose content qualifies: "Backend Engineer (Berlin)".
		if strings.HasSuffix(s, ")") {
			if i := strings.LastIndex(s, "("); i > 0 && len(tokens(s[:i])) > 0 && removable(s[i+1:len(s)-1], locs) {
				s, stripped = strings.TrimSpace(s[:i]), true
				continue
			}
		}
		// The leftmost separator whose whole suffix qualifies, so "- Bay Area, CA" goes
		// as one piece when it all names a place.
		for i := 0; i < len(s); i++ {
			sep := separatorAt(s, i)
			if sep == 0 || len(tokens(s[:i])) == 0 {
				continue
			}
			if removable(s[i+sep:], locs) {
				s, stripped = strings.TrimSpace(s[:i]), true
				break
			}
		}
	}
	t := tokens(s)
	if trimmed := trimGender(t); len(trimmed) > 0 {
		t = trimmed
	}
	return strings.Join(t, " ")
}

// separators that may start a trailing segment.
var separators = []string{" - ", " – ", " — ", " | ", ", "}

// separatorAt returns the length of the separator starting at s[i], or 0.
func separatorAt(s string, i int) int {
	for _, sep := range separators {
		if strings.HasPrefix(s[i:], sep) {
			return len(sep)
		}
	}
	return 0
}

// vocabulary is the fixed list of words a removable segment may consist of (folded,
// lowercase): work modes, regions, gender markers and fillers. Countries and cities are
// not here: they are matched against the record's own locations.
var vocabulary = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		remote remotely hybrid hybride onsite teletravail distanciel anywhere worldwide global wfh
		full fully first partial partiel complet complete possible
		emea emear amer americas apac apj anz aunz latam namer nam na eu europe european asia mena
		dach nordics benelux cee uk us usa international intl
		h f m w d x
		depuis from in or and ou et en de du`) {
		vocabulary[w] = true
	}
}

// removable reports whether a trailing segment can go: once vocabulary words are
// dropped, what is left is nothing, or appears as whole consecutive words in one of the
// record's locations ("berlin" in "Berlin, Germany"; "ai" is not in "Dubai").
func removable(segment string, locs [][]string) bool {
	var rest []string
	for _, w := range tokens(segment) {
		if !vocabulary[w] {
			rest = append(rest, w)
		}
	}
	if len(rest) == 0 {
		return true
	}
	for _, l := range locs {
		if containsRun(l, rest) {
			return true
		}
	}
	return false
}

// containsRun reports whether run appears as consecutive elements of words.
func containsRun(words, run []string) bool {
	for i := 0; i+len(run) <= len(words); i++ {
		match := true
		for j := range run {
			if words[i+j] != run[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// genderMarkers are trailing word sequences of French and German job titles: "H/F",
// "(F/H)", "(m/w/d)".
var genderMarkers = [][]string{{"h", "f"}, {"f", "h"}, {"m", "w", "d"}, {"w", "m", "d"}, {"m", "f", "d"}, {"f", "m"}, {"m", "f"}, {"h", "f", "x"}}

func trimGender(t []string) []string {
	for _, g := range genderMarkers {
		if len(t) >= len(g) && equal(t[len(t)-len(g):], g) {
			return t[:len(t)-len(g)]
		}
	}
	return t
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// tokens folds accents, lowercases and splits s into words of letters and digits.
func tokens(s string) []string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case fold[r] != "":
			b.WriteString(fold[r])
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Fields(b.String())
}

// fold maps accented Latin letters (lowercase) to plain ASCII. A fixed table: enough for
// French, German, Spanish and Portuguese titles.
var fold = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'æ': "ae",
	'ç': "c", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ñ': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'œ': "oe",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ý': "y", 'ÿ': "y", 'ß': "ss",
}
