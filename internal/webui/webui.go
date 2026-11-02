// Package webui serves the tracker page (M2-06, ADR-0013): the vite build of web/,
// embedded in the binary, with a same-origin Content-Security-Policy.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// dist holds web/'s build in dist/app (`make web-build`); dist/.gitkeep is committed so
// Go builds and tests never need Node.
//
//go:embed all:dist
var dist embed.FS

// csp allows only this origin: scripts, styles and API calls (ADR-0013).
const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Dist returns the embedded build (empty when the page hasn't been built).
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist/app")
	if err != nil { // a constant, valid path: unreachable; serve "not built" rather than fail
		return embed.FS{}
	}
	return sub
}

// Handler serves the page in fsys: files by path, index.html for any other page path (the
// app reads its state from the URL), 404 for missing assets.
func Handler(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		index, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.Error(w, "the tracker page is not built: run make web-build", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if _, err := fs.Stat(fsys, name); err == nil {
				h.Set("Cache-Control", "public, max-age=31536000, immutable") // vite names assets by hash
				files.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r)
				return
			}
		}
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		_, _ = w.Write(index) // the client went away; nothing to do
	})
}
