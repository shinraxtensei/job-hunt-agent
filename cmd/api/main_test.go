package main

import (
	"net/http"
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
