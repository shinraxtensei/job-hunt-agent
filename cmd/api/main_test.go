package main

import (
	"fmt"
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
	h := routes(api, page, []string{"8080"})
	for p, want := range map[string]string{"/api/leads": "api", "/api/nothing": "api", "/healthz": "api",
		"/": "page", "/?view=hidden": "page", "/assets/app.js": "page"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Host = "127.0.0.1:8080"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Who"); got != want {
			t.Errorf("%s went to %q, want %q", p, got, want)
		}
		// #43 review: a rebound domain reaches neither the API nor the page.
		req.Host = "evil.example:8080"
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMisdirectedRequest || rec.Header().Get("X-Who") != "" {
			t.Errorf("%s with a forged Host: %d via %q, want 421 and no handler", p, rec.Code, rec.Header().Get("X-Who"))
		}
	}
}

func TestAllowedPorts(t *testing.T) {
	for _, c := range []struct{ addr, public, want string }{
		{"127.0.0.1:8080", "", "[8080]"},
		{"0.0.0.0:8080", "8088", "[8080 8088]"},
		{"0.0.0.0:8080", "8080", "[8080]"},
		{":9000", "", "[9000]"},
	} {
		if got := fmt.Sprint(allowedPorts(c.addr, c.public)); got != c.want {
			t.Errorf("allowedPorts(%q, %q) = %s, want %s", c.addr, c.public, got, c.want)
		}
	}
}

// L-01: the image has no curl, so the API binary checks itself for Compose's healthcheck.
func TestHealthcheckURL(t *testing.T) {
	for addr, want := range map[string]string{
		"0.0.0.0:8080":   "http://127.0.0.1:8080/healthz",
		":8080":          "http://127.0.0.1:8080/healthz",
		"127.0.0.1:9000": "http://127.0.0.1:9000/healthz",
		"[::1]:8080":     "http://127.0.0.1:8080/healthz",
		"evil.com:80":    "http://127.0.0.1:80/healthz",   // never another host
		"0.0.0.0:http":   "http://127.0.0.1:8080/healthz", // not a number: the default
		"garbage":        "http://127.0.0.1:8080/healthz",
	} {
		if got := healthcheckURL(addr); got != want {
			t.Errorf("healthcheckURL(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestProbe(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ok.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer down.Close()
	if err := probe(t.Context(), ok.URL); err != nil {
		t.Errorf("healthy: %v", err)
	}
	if err := probe(t.Context(), down.URL); err == nil {
		t.Error("503 counted as healthy")
	}
	if err := probe(t.Context(), "http://127.0.0.1:1/healthz"); err == nil {
		t.Error("nothing listening counted as healthy")
	}
}

// The image is built without .git, so the commit comes from a build flag.
func TestCommitPrefersBuildFlag(t *testing.T) {
	old := buildCommit
	defer func() { buildCommit = old }()
	buildCommit = "abc1234"
	if got := commit(); got != "abc1234" {
		t.Errorf("commit() = %q, want the build flag", got)
	}
}
