package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/shinraxtensei/job-hunter/internal/webui"
)

var built = fstest.MapFS{
	"index.html":       {Data: []byte(`<!doctype html><script type="module" src="/assets/app.js"></script>`)},
	"assets/app.js":    {Data: []byte(`console.log(1)`)},
	"assets/style.css": {Data: []byte(`body{}`)},
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// AC6: the built page is served at /, its assets by path, any other page path falls back
// to index.html, and every response carries the same-origin CSP.
func TestServesBuiltPageWithCSP(t *testing.T) {
	h := webui.Handler(built)
	for _, c := range []struct{ path, contains, contentType string }{
		{"/", `src="/assets/app.js"`, "text/html"},
		{"/?view=hidden&role_family=sales", `src="/assets/app.js"`, "text/html"},
		{"/some/deep/link", `src="/assets/app.js"`, "text/html"},
		{"/assets/app.js", "console.log", "text/javascript"},
		{"/assets/style.css", "body{}", "text/css"},
	} {
		rec := get(h, c.path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), c.contains) ||
			!strings.HasPrefix(rec.Header().Get("Content-Type"), c.contentType) {
			t.Errorf("%s: %d %q %q", c.path, rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
		}
		csp := rec.Header().Get("Content-Security-Policy")
		for _, want := range []string{"default-src 'self'", "script-src 'self'", "style-src 'self'",
			"connect-src 'self'", "base-uri 'none'", "form-action 'none'", "frame-ancestors 'none'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP %q lacks %q", c.path, csp, want)
			}
		}
		if strings.Contains(csp, "unsafe") || strings.Contains(csp, "http") {
			t.Errorf("%s: CSP allows more than same origin: %q", c.path, csp)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: missing nosniff or referrer policy", c.path)
		}
	}
	if rec := get(h, "/assets/missing.js"); rec.Code != http.StatusNotFound {
		t.Errorf("a missing asset answered %d, want 404 (not the page)", rec.Code)
	}
}

// Without a build (Go tests and builds never need Node), / says how to build the page.
func TestNotBuilt(t *testing.T) {
	rec := get(webui.Handler(fstest.MapFS{}), "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "make web-build") {
		t.Errorf("got %d %q", rec.Code, rec.Body.String())
	}
}

// The embedded files are the vite build output directory.
func TestDistIsTheBuildDirectory(t *testing.T) {
	if webui.Dist() == nil {
		t.Fatal("no embedded filesystem")
	}
}
