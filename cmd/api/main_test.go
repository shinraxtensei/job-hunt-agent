package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// AC5: the server has read, write and idle timeouts.
func TestServerTimeouts(t *testing.T) {
	s := newServer(http.NotFoundHandler())
	if s.ReadHeaderTimeout == 0 || s.ReadTimeout == 0 || s.WriteTimeout == 0 || s.IdleTimeout == 0 {
		t.Errorf("timeouts not all set: %+v", s)
	}
}

func TestLoopback(t *testing.T) {
	for addr, want := range map[string]bool{"127.0.0.1:8080": true, "localhost:8080": true, "[::1]:8080": true,
		":8080": false, "0.0.0.0:8080": false, "192.168.1.5:8080": false} {
		if got := loopback(addr); got != want {
			t.Errorf("loopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

// AC6: API paths reach the API, everything else the page.
func TestRoutes(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("X-Who", "api") })
	page := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("X-Who", "page") })
	h := routes(api, page)
	for p, want := range map[string]string{"/api/leads": "api", "/api/nothing": "api", "/healthz": "api",
		"/": "page", "/?view=hidden": "page", "/assets/app.js": "page"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if got := rec.Header().Get("X-Who"); got != want {
			t.Errorf("%s went to %q, want %q", p, got, want)
		}
	}
}
