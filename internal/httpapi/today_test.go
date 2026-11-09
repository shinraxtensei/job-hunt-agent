package httpapi

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// M7-01 AC2-AC3 (OI-20): "new" starts at the visit before the current one, or at "Mark all
// seen" if later; with no earlier visit, 24 hours ago.
func TestSince(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	at := func(h int) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: now.Add(time.Duration(h) * time.Hour), Valid: true}
	}
	none := pgtype.Timestamptz{}
	for _, c := range []struct {
		name   string
		visits []store.Visit
		want   time.Time
	}{
		{"no visit", nil, now.Add(-24 * time.Hour)},
		{"first visit", []store.Visit{{StartedAt: at(-1), MarkedAt: none}}, now.Add(-24 * time.Hour)},
		{"previous visit", []store.Visit{{StartedAt: at(0)}, {StartedAt: at(-20)}}, now.Add(-20 * time.Hour)},
		{"marked after the previous", []store.Visit{{StartedAt: at(0), MarkedAt: at(-1)}, {StartedAt: at(-20)}}, now.Add(-1 * time.Hour)},
		{"marked before the previous", []store.Visit{{StartedAt: at(0), MarkedAt: at(-30)}, {StartedAt: at(-20)}}, now.Add(-20 * time.Hour)},
	} {
		if got := since(c.visits, now); !got.Equal(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
