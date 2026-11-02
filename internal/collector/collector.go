// Package collector fetches postings from sources and stores them.
package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunter/internal/job"
	"github.com/shinraxtensei/job-hunter/internal/store"
)

// Source is one job board's adapter. Defined here, where it is used; adapters live in
// internal/source/<name>.
type Source interface {
	// Name is the board type it serves ("lever"), which is also job.Raw.Source.
	Name() string
	// Host is the API host it calls; Run rate-limits per host.
	Host() string
	// Fetch returns every published posting on slug's board and the entries it skipped
	// as malformed or unstorable (by ID where the ID was readable). It must wait on limit
	// before every HTTP request it makes, retries included: limit is shared by every
	// company on Host.
	Fetch(ctx context.Context, slug string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error)
}

// beginner opens transactions: *pgx.Conn and *pgxpool.Pool both are.
type beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Result counts what one company's collection saw. JobsFailed counts new or changed
// postings that were stored but could not be normalised (ADR-0008).
type Result struct{ Fetched, New, Changed, Seen, Skipped, JobsFailed, Closed, Reopened int }

// EmptyBoardGuard: a successful fetch that returns no postings for a company with more
// open postings than this closes nothing and fails the company (ADR-0010): an outage or an
// API change must not close a whole board.
const EmptyBoardGuard = 10

// CloseRatioGuardMin: a fetch that would close more than half of a company's open postings,
// and at least this many, closes nothing and fails the company (ADR-0010): a partial outage
// must not close most of a board.
const CloseRatioGuardMin = 10

// Once fetches one board by slug and stores every posting in one transaction, creating
// the company row if needed (with no track: it is not on the watch list). A failure at
// any point writes nothing. Connect as the app role (ADR-0004). Requests (retries
// included) are spaced at least 1s apart, Lever's Crawl-delay.
func Once(ctx context.Context, db beginner, src Source, slug string, log *slog.Logger) (Result, error) {
	limit := rate.NewLimiter(rate.Every(time.Second), 1)
	return collect(ctx, db, src, slug, limit, log, func(q *store.Queries) (int64, error) {
		return companyID(ctx, q, src.Name(), slug)
	})
}

// collect fetches slug's board, then stores it in one transaction under the company
// that companyID returns. Fetch comes first, so a failed fetch touches nothing.
//
// Logs carry counts and IDs only: posting text is untrusted and never logged.
func collect(ctx context.Context, db beginner, src Source, slug string, limit *rate.Limiter,
	log *slog.Logger, companyID func(*store.Queries) (int64, error),
) (Result, error) {
	raws, skipped, err := src.Fetch(ctx, slug, limit)
	if err != nil {
		return Result{}, err // adapters name the company
	}
	res := Result{Fetched: len(raws), Skipped: skipped.Total()}
	var newIDs, reopenedIDs []string

	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		co, err := companyID(q)
		if err != nil {
			return err
		}
		if len(raws) == 0 {
			open, err := q.CountOpenPostings(ctx, store.CountOpenPostingsParams{CompanyID: co, Source: src.Name()})
			if err != nil {
				return fmt.Errorf("counting open postings: %w", err)
			}
			if open > EmptyBoardGuard {
				return fmt.Errorf("empty board, refusing to close %d postings", open)
			}
		}
		// Skipped entries with a readable ID are still on the board: never close them.
		seen := append(make([]string, 0, len(raws)+len(skipped.IDs)), skipped.IDs...)
		for _, r := range raws {
			seen = append(seen, r.SourceID)
			r2, err := q.UpsertPosting(ctx, store.UpsertPostingParams{
				CompanyID:       co,
				Source:          r.Source,
				SourcePostingID: r.SourceID,
				Url:             r.URL,
				Title:           r.Title,
				RawPayload:      r.Payload,
			})
			if err != nil {
				return fmt.Errorf("storing posting %s: %w", r.SourceID, err)
			}
			if r2.Reopened {
				reopenedIDs = append(reopenedIDs, r.SourceID)
			}
			switch {
			case r2.Inserted:
				newIDs = append(newIDs, r.SourceID)
			case r2.Changed:
				res.Changed++
			default:
				continue // unchanged: its job record is current, leave it alone
			}
			// New or changed: (re)write its job record in this same transaction.
			ok, err := writeJob(ctx, q, r2.ID, co, r.Source, r.Payload, r2.ContentHash, log)
			if err != nil {
				return err
			}
			if !ok {
				res.JobsFailed++
			}
		}
		// The fetch succeeded (we are here), so whatever the board no longer lists is gone,
		// unless that is most of the board, which looks like a partial outage.
		toClose, err := q.CountPostingsNotIn(ctx, store.CountPostingsNotInParams{CompanyID: co, Source: src.Name(), SeenIds: seen})
		if err != nil {
			return fmt.Errorf("counting postings to close: %w", err)
		}
		if toClose >= CloseRatioGuardMin {
			open, err := q.CountOpenPostings(ctx, store.CountOpenPostingsParams{CompanyID: co, Source: src.Name()})
			if err != nil {
				return fmt.Errorf("counting open postings: %w", err)
			}
			if toClose*2 > open {
				return fmt.Errorf("refusing to close %d of %d postings", toClose, open)
			}
		}
		closed, err := q.ClosePostingsNotIn(ctx, store.ClosePostingsNotInParams{CompanyID: co, Source: src.Name(), SeenIds: seen})
		if err != nil {
			return fmt.Errorf("closing missing postings: %w", err)
		}
		res.Closed = int(closed)
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("collect %s %s: %w", src.Name(), slug, err)
	}

	// Logged only after commit, so the log never reports rows that were rolled back.
	for _, id := range newIDs {
		log.Info("new posting", "source", src.Name(), "company", slug, "posting_id", id)
	}
	for _, id := range reopenedIDs {
		log.Info("posting reopened", "source", src.Name(), "company", slug, "posting_id", id)
	}
	res.Reopened = len(reopenedIDs)
	res.New = len(newIDs)
	res.Seen = res.Fetched - res.New - res.Changed
	log.Info("collect finished", "source", src.Name(), "company", slug,
		"fetched", res.Fetched, "new", res.New, "changed", res.Changed, "seen", res.Seen, "skipped", res.Skipped,
		"jobs_failed", res.JobsFailed, "closed", res.Closed, "reopened", res.Reopened)
	return res, nil
}

// companyID returns the id of the company on this board, creating it if needed. One
// upsert on the unique board key (ADR-0005), so concurrent runs share one row.
func companyID(ctx context.Context, q *store.Queries, boardType, slug string) (int64, error) {
	id, err := q.UpsertCompanyID(ctx, store.UpsertCompanyIDParams{Name: slug, BoardType: boardType, BoardSlug: slug})
	if err != nil {
		return 0, fmt.Errorf("upserting company: %w", err)
	}
	return id, nil
}
