// Package queue reads the work queue (ADR-0001). Go only produces items (the reconcile
// step in internal/collector); the Python worker claims them.
package queue

import (
	"context"
	"fmt"

	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// Counts is the queue by state.
type Counts struct {
	Pending int `json:"pending"`
	Running int `json:"running"` // includes expired leases not yet claimed again
	Failed  int `json:"failed"`  // needs review (ADR-0001)
	Done    int `json:"done"`
}

// Count returns how many work items are in each state.
func Count(ctx context.Context, db store.DBTX) (Counts, error) {
	rows, err := store.New(db).CountWorkItems(ctx)
	if err != nil {
		return Counts{}, fmt.Errorf("counting work items: %w", err)
	}
	var c Counts
	for _, r := range rows {
		switch r.State {
		case "pending":
			c.Pending = int(r.N)
		case "running":
			c.Running = int(r.N)
		case "failed":
			c.Failed = int(r.N)
		case "done":
			c.Done = int(r.N)
		}
	}
	return c, nil
}

// RetryFailed puts the failed items of a kind back to pending with 0 attempts and no
// error (M3-04 AC8), e.g. after a bug fix; it returns how many.
func RetryFailed(ctx context.Context, db store.DBTX, kind string) (int, error) {
	n, err := store.New(db).RetryFailed(ctx, kind)
	if err != nil {
		return 0, fmt.Errorf("retrying failed items: %w", err)
	}
	return int(n), nil
}
