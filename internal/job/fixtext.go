package job

import (
	"html"
	"unicode/utf8"
)

// FixText undoes two encoding slips some boards make in plain-text fields (Remote OK,
// M7-03): HTML entities ("&amp;") and UTF-8 bytes read as Latin-1 ("Sothebyâ\u0080\u0099s").
// The repair is applied only when every rune fits a byte, some don't fit ASCII, and the
// bytes are valid UTF-8, so correct accented text is left alone. The result is text: it
// is shown as text, never as markup.
func FixText(s string) string {
	s = html.UnescapeString(s)
	b := make([]byte, 0, len(s))
	high := false
	for _, r := range s {
		if r > 0xFF {
			return s
		}
		high = high || r >= 0x80
		b = append(b, byte(r)) //nolint:gosec // r <= 0xFF, checked above
	}
	if high && utf8.Valid(b) {
		return string(b)
	}
	return s
}
