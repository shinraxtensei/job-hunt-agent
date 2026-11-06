package httpapi

import (
	"math"
	"testing"
	"time"
)

// M3-08: the API's freshness and fit level match score.py's (the same table as
// agents/tests/test_score.py): whole days since published.
func TestFreshness(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	p := freshnessParams{Weight: 5, FreshDays: 7, StaleDays: 45, Unknown: 0.5}
	for _, c := range []struct {
		days float64 // -1: not published
		want float64
	}{{0, 1}, {7, 1}, {7.9, 1}, {26, 0.5}, {26.9, 0.5}, {45, 0}, {400, 0}, {-1, 0.5}} {
		var pub *time.Time
		if c.days >= 0 {
			at := now.Add(-time.Duration(c.days * 24 * float64(time.Hour)))
			pub = &at
		}
		if got := freshness(pub, now, p); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("freshness(%v days) = %v, want %v", c.days, got, c.want)
		}
	}
}

func TestFitLevel(t *testing.T) {
	b := bands{VeryStrong: 73, Strong: 53, Good: 33, Stretch: 32}
	for total, want := range map[float64]string{100: "very_strong", 73: "very_strong", 72.9: "strong",
		53: "strong", 33: "good", 32.5: "stretch", 31.9: "no", 0: "no"} {
		if got := fitLevel(total, 15, b); got != want {
			t.Errorf("fitLevel(%v) = %s, want %s", total, got, want)
		}
	}
}

// M3-09: the location gate, the same table as agents/tests/test_score.py.
func TestLocationGate(t *testing.T) {
	b := bands{VeryStrong: 73, Strong: 53, Good: 33, Stretch: 32, LocationGate: 5}
	for _, c := range []struct {
		total, location float64
		want            string
	}{{80, 3, "stretch"}, {80, 5, "very_strong"}, {60, 4.9, "stretch"}, {40, 0, "stretch"},
		{32.5, 3, "stretch"}, {10, 3, "no"}} {
		if got := fitLevel(c.total, c.location, b); got != c.want {
			t.Errorf("fitLevel(%v, location %v) = %s, want %s", c.total, c.location, got, c.want)
		}
	}
}
