package collector

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunt-agent/internal/preset"
	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// ReextractResult is what `collector enqueue --reextract` found (and re-queued unless
// it was a dry run).
type ReextractResult struct {
	N            int     // shown postings extracted only with an older prompt
	Version      int     // the prompt version the worker runs now
	EstimatedUSD float64 // N × the mean cost of a successful extraction so far
	CostKnown    bool    // false before any successful extraction was logged
}

// Reextract finds the postings p shows whose current content was extracted only with a
// prompt older than the one the worker runs (M3-04 AC7) and, unless dryRun, puts their
// extract items back to pending. Nothing re-queues on its own when a prompt ships;
// hidden jobs are never touched.
func Reextract(ctx context.Context, db DB, p preset.Preset, dryRun bool) (ReextractResult, error) {
	q := store.New(db)
	version, err := q.CurrentPromptVersion(ctx, "extract")
	if errors.Is(err, pgx.ErrNoRows) {
		return ReextractResult{}, errors.New("no prompt version recorded yet: start the worker once (make up)")
	}
	if err != nil {
		return ReextractResult{}, fmt.Errorf("reading the prompt version: %w", err)
	}
	groups, err := Groups(ctx, db)
	if err != nil {
		return ReextractResult{}, err
	}
	var shown []int64
	for _, g := range groups {
		if p.Evaluate(g.Group) == nil {
			shown = append(shown, g.PostingID)
		}
	}
	ids, err := q.OutdatedExtractions(ctx, store.OutdatedExtractionsParams{PostingIds: shown, Version: version})
	if err != nil {
		return ReextractResult{}, fmt.Errorf("finding older extractions: %w", err)
	}
	cost, err := q.AverageExtractCost(ctx)
	if err != nil {
		return ReextractResult{}, fmt.Errorf("reading extraction costs: %w", err)
	}
	res := ReextractResult{N: len(ids), Version: int(version), EstimatedUSD: float64(len(ids)) * cost.Usd, CostKnown: cost.N > 0}
	if dryRun || len(ids) == 0 {
		return res, nil
	}
	n, err := q.RequeueExtract(ctx, ids)
	if err != nil {
		return res, fmt.Errorf("re-queuing: %w", err)
	}
	res.N = int(n)
	return res, nil
}
