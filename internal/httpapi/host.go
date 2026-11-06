package httpapi

import (
	"net"
	"net/http"
	"slices"
	"strings"
)

// loopbackHosts are the only names the API answers to: the API has no auth and is
// reachable on loopback only (ADR-0002, ADR-0016), and a page whose domain is rebound
// to 127.0.0.1 sends its own name as Host (ADR-0022).
var loopbackHosts = []string{"127.0.0.1", "localhost", "::1"}

// HostAllowlist serves only requests whose Host is a loopback name on one of ports;
// anything else gets 421 Misdirected Request. It wraps every route, reads and writes.
func HostAllowlist(ports []string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, err := net.SplitHostPort(r.Host)
		if err != nil || !slices.Contains(loopbackHosts, strings.ToLower(host)) || !slices.Contains(ports, port) {
			w.Header().Set("Content-Type", "application/json")
			writeError(w, http.StatusMisdirectedRequest, "", "this server answers only to 127.0.0.1, localhost and [::1]")
			return
		}
		h.ServeHTTP(w, r)
	})
}
