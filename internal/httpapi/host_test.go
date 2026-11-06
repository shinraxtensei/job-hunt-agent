package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// #43 review (ADR-0022, ADR-0016): the API has no auth and relies on loopback, so a DNS
// rebinding page (evil.example resolving to 127.0.0.1) must not reach it. Only loopback
// hosts on the configured ports are served, on every route.
func TestHostAllowlist(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := HostAllowlist([]string{"8080", "8088"}, ok)
	for host, want := range map[string]int{
		"127.0.0.1:8080":      http.StatusNoContent,
		"localhost:8088":      http.StatusNoContent,
		"LOCALHOST:8088":      http.StatusNoContent,
		"[::1]:8080":          http.StatusNoContent,
		"127.0.0.1:9999":      http.StatusMisdirectedRequest, // another port
		"evil.example:8080":   http.StatusMisdirectedRequest, // DNS rebinding
		"evil.example":        http.StatusMisdirectedRequest,
		"127.0.0.1":           http.StatusMisdirectedRequest, // no port
		"192.168.1.10:8080":   http.StatusMisdirectedRequest,
		"localhost.evil:8080": http.StatusMisdirectedRequest,
		"":                    http.StatusMisdirectedRequest,
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			req := httptest.NewRequest(method, "/api/leads", nil)
			req.Host = host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != want {
				t.Errorf("%s Host %q: %d, want %d", method, host, rec.Code, want)
			}
		}
	}
}
