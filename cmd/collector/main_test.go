package main

import (
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestRunConfigDefaults(t *testing.T) {
	cfg, err := runConfig(env(nil))
	if err != nil {
		t.Fatalf("runConfig: %v", err)
	}
	if cfg.Concurrency != 8 || cfg.HostInterval != time.Second {
		t.Errorf("defaults = %+v, want concurrency 8, host interval 1s", cfg)
	}
}

func TestRunConfigFromEnv(t *testing.T) {
	cfg, err := runConfig(env(map[string]string{"COLLECTOR_CONCURRENCY": "3", "COLLECTOR_HOST_INTERVAL": "2s"}))
	if err != nil {
		t.Fatalf("runConfig: %v", err)
	}
	if cfg.Concurrency != 3 || cfg.HostInterval != 2*time.Second {
		t.Errorf("cfg = %+v, want concurrency 3, host interval 2s", cfg)
	}
}

// Politeness can't be configured away: Lever's robots.txt asks for Crawl-delay: 1.
func TestRunConfigRejectsUnsafeValues(t *testing.T) {
	for _, m := range []map[string]string{
		{"COLLECTOR_CONCURRENCY": "0"},
		{"COLLECTOR_CONCURRENCY": "lots"},
		{"COLLECTOR_HOST_INTERVAL": "500ms"},
		{"COLLECTOR_HOST_INTERVAL": "soon"},
	} {
		if _, err := runConfig(env(m)); err == nil {
			t.Errorf("runConfig(%v) returned no error", m)
		}
	}
}

// AC3: a partial run exits 0; only failed and cancelled runs exit non-zero.
func TestExitCode(t *testing.T) {
	for outcome, want := range map[string]int{"ok": 0, "partial": 0, "failed": 1, "cancelled": 1} {
		if got := exitCode(outcome); got != want {
			t.Errorf("exitCode(%q) = %d, want %d", outcome, got, want)
		}
	}
}

// M2-07: `add <url>` takes the link first, then optional flags, and refuses anything else.
func TestParseAddArgs(t *testing.T) {
	link, m, err := parseAddArgs([]string{"https://www.linkedin.com/jobs/view/1", "--title", "Backend Engineer", "--company", "Acme", "--location", "Casablanca"})
	if err != nil || link != "https://www.linkedin.com/jobs/view/1" || m.Title != "Backend Engineer" || m.Company != "Acme" || m.Location != "Casablanca" {
		t.Errorf("parseAddArgs = %q %+v %v", link, m, err)
	}
	if link, _, err := parseAddArgs([]string{"https://jobs.lever.co/x/1"}); err != nil || link != "https://jobs.lever.co/x/1" {
		t.Errorf("link only: %q %v", link, err)
	}
	for _, args := range [][]string{{}, {"--title", "x"}, {"https://a", "https://b"}, {"https://a", "--colour", "blue"}} {
		if _, _, err := parseAddArgs(args); err == nil {
			t.Errorf("parseAddArgs(%q) accepted", args)
		}
	}
}
