package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/store"
)

// Registry maps a board type ("lever") to its adapter.
type Registry map[string]Source

// Config bounds a run. cmd/collector parses it from the environment and enforces the
// politeness floor (HostInterval >= 1s); tests may go lower.
type Config struct {
	Concurrency  int           // companies fetched at once (COLLECTOR_CONCURRENCY)
	HostInterval time.Duration // minimum gap between requests to one host; 0 = none
}

// RunResult is what a run did; the same numbers go into its collector_runs row.
type RunResult struct {
	RunID                                 int64
	Outcome                               string // ok, partial, failed or cancelled
	Attempted, Failed, Skipped, New, Seen int    // Seen: already stored, re-checked
}

// DB is what Run needs from the database: queries and transactions, concurrently
// (a *pgxpool.Pool).
type DB interface {
	store.DBTX
	beginner
}

// maxErrorLen caps a recorded error. Ours are short (status and company); this guards
// against a wrapped driver error growing without bound.
const maxErrorLen = 1000

type work struct {
	co      store.ListCollectableCompaniesRow
	src     Source
	limiter *rate.Limiter
}

type failure struct {
	companyID int64
	err       error
}

// Run collects every active, non-excluded company concurrently and records the run in
// collector_runs (ADR-0006). Each company is its own transaction, so a failure or a
// cancellation leaves every company either fully stored or untouched. One company
// failing never stops the others. The returned error is for the run itself (the
// database is unreachable); company failures are in the result.
func Run(ctx context.Context, db DB, reg Registry, cfg Config, log *slog.Logger) (RunResult, error) {
	q := store.New(db)
	runID, err := q.StartRun(ctx)
	if err != nil {
		return RunResult{}, fmt.Errorf("starting run: %w", err)
	}
	res := RunResult{RunID: runID}

	companies, err := q.ListCollectableCompanies(ctx)
	if err != nil {
		res.Outcome = "failed"
		return res, errors.Join(fmt.Errorf("listing companies: %w", err), finish(ctx, db, &res, nil))
	}

	todo, skipped := plan(companies, reg, cfg.HostInterval)
	skippedN := 0
	for boardType, names := range skipped {
		log.Warn("no adapter for board type; skipping companies", "board_type", boardType, "companies", names)
		skippedN += len(names)
	}
	res.Attempted, res.Skipped = len(todo), skippedN

	var (
		mu       sync.Mutex
		failures []failure
		g        errgroup.Group
	)
	g.SetLimit(max(cfg.Concurrency, 1))
	for _, w := range todo {
		g.Go(func() error {
			// The source waits on its host's limiter before each request it makes.
			r, err := collect(ctx, db, w.src, w.co.BoardSlug, w.limiter, log, func(*store.Queries) (int64, error) {
				return w.co.ID, nil
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				res.New += r.New
				res.Seen += r.Existing
			case ctx.Err() != nil:
				// Stopped by cancellation, not a board failure; the run is "cancelled".
			default:
				failures = append(failures, failure{w.co.ID, err})
				log.Error("company failed", "source", w.src.Name(), "company", w.co.BoardSlug, "err", err)
			}
			return nil // never cancel the other companies
		})
	}
	_ = g.Wait() // every goroutine returns nil; failures are collected above

	res.Failed = len(failures)
	switch {
	case ctx.Err() != nil:
		res.Outcome = "cancelled"
	case res.Failed == 0:
		res.Outcome = "ok"
	case res.Failed == res.Attempted:
		res.Outcome = "failed"
	default:
		res.Outcome = "partial"
	}
	if err := finish(ctx, db, &res, failures); err != nil {
		return res, err
	}
	log.Info("run finished", "run_id", res.RunID, "outcome", res.Outcome, "attempted", res.Attempted,
		"failed", res.Failed, "skipped", res.Skipped, "new", res.New, "seen", res.Seen)
	return res, nil
}

// plan pairs each company with its adapter and its host's limiter, interleaved by host
// so one busy host can't fill every concurrency slot while the others wait. Companies
// whose board type has no adapter are returned by board type, for the warning.
//
// ponytail: a slot is held while waiting on its host's limiter; interleaving keeps that
// cheap at 50 companies. A per-host queue would be the upgrade if hosts get lopsided.
func plan(companies []store.ListCollectableCompaniesRow, reg Registry, interval time.Duration) ([]work, map[string][]string) {
	limit := rate.Inf
	if interval > 0 {
		limit = rate.Every(interval)
	}
	limiters := map[string]*rate.Limiter{}
	byHost := map[string][]work{}
	var hosts []string
	skipped := map[string][]string{}
	for _, co := range companies {
		src, ok := reg[co.BoardType]
		if !ok {
			skipped[co.BoardType] = append(skipped[co.BoardType], co.Name)
			continue
		}
		h := src.Host()
		if limiters[h] == nil {
			limiters[h] = rate.NewLimiter(limit, 1)
			hosts = append(hosts, h)
		}
		byHost[h] = append(byHost[h], work{co: co, src: src, limiter: limiters[h]})
	}
	var todo []work
	for i := 0; ; i++ {
		added := false
		for _, h := range hosts {
			if i < len(byHost[h]) {
				todo = append(todo, byHost[h][i])
				added = true
			}
		}
		if !added {
			return todo, skipped
		}
	}
}

// finish records the run's outcome and failures in one transaction. It runs even when
// ctx is cancelled (that is how a cancelled run gets recorded), with its own deadline.
func finish(ctx context.Context, db DB, res *RunResult, failures []failure) error {
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := pgx.BeginFunc(fctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		for _, f := range failures {
			if err := q.AddRunError(fctx, store.AddRunErrorParams{
				RunID: res.RunID, CompanyID: f.companyID, Error: truncate(f.err.Error(), maxErrorLen),
			}); err != nil {
				return fmt.Errorf("recording failure: %w", err)
			}
		}
		return q.FinishRun(fctx, store.FinishRunParams{
			ID:                 res.RunID,
			Outcome:            pgtype.Text{String: res.Outcome, Valid: true},
			CompaniesAttempted: int32(res.Attempted), //nolint:gosec // bounded by the company count
			CompaniesFailed:    int32(res.Failed),    //nolint:gosec // as above
			CompaniesSkipped:   int32(res.Skipped),   //nolint:gosec // as above
			PostingsNew:        int32(res.New),       //nolint:gosec // postings per run, far below 2^31
			PostingsSeen:       int32(res.Seen),      //nolint:gosec // as above
		})
	})
	if err != nil {
		return fmt.Errorf("recording run %d: %w", res.RunID, err)
	}
	return nil
}

// truncate cuts s to at most n bytes without splitting a UTF-8 character (Postgres
// rejects invalid UTF-8).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
