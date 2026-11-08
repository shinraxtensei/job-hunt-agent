package httpapi

import "testing"

// M5-03: a download's file name keeps letters and digits in any script; anything else
// becomes "-", so a person or company name can't break the header.
func TestFileWord(t *testing.T) {
	for in, want := range map[string]string{
		"Zoë Côté":                      "Zoë-Côté",
		`Acme "Inc"; filename=evil.exe`: "Acme-Inc-filename-evil-exe",
		"\r\nX-Injected: 1":             "X-Injected-1",
		"":                              "",
	} {
		if got := fileWord(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
