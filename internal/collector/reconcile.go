package collector

import (
	"context"
	"fmt"

	"github.com/shinraxtensei/job-hunt-agent/internal/preset"
	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// reconcileBatch is how many postings one enqueue statement (one transaction) covers.
const reconcileBatch = 500

// Reconcile is the queue's only producer (ADR-0001): an extract item for every group p
// shows whose canonical posting has no item for its current content. Hidden groups are
// never enqueued. Idempotent and safe to run concurrently (the unique constraint and
// the upsert's hash check decide); returns how many items it added or re-queued.
func Reconcile(ctx context.Context, db DB, p preset.Preset) (int, error) {
	groups, err := Groups(ctx, db)
	if err != nil {
		return 0, err
	}
	var ids []int64
	var hashes [][]byte
	var matched []string // ADR-0023: the location that passed, for the scorer
	for _, g := range groups {
		if p.Evaluate(g.Group) == nil {
			ids = append(ids, g.PostingID)
			hashes = append(hashes, g.ContentHash)
			matched = append(matched, p.MatchedLocation(g.Group))
		}
	}
	q := store.New(db)
	total := 0
	for start := 0; start < len(ids); start += reconcileBatch {
		end := min(start+reconcileBatch, len(ids))
		n, err := q.EnqueueExtract(ctx, store.EnqueueExtractParams{PostingIds: ids[start:end], ContentHashes: hashes[start:end]})
		if err != nil {
			return total, fmt.Errorf("enqueuing: %w", err)
		}
		total += int(n)
		if err := q.SetMatchedLocations(ctx, store.SetMatchedLocationsParams{PostingIds: ids[start:end], Locations: matched[start:end]}); err != nil {
			return total, fmt.Errorf("storing matched locations: %w", err)
		}
	}
	return total, nil
}
