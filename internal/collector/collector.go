// Package collector fetches postings from sources and stores them.
package collector

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

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
	// Fetch returns every published posting on slug's board and how many entries it
	// skipped as malformed. It makes one HTTP request (Run's limiter counts on that).
	Fetch(ctx context.Context, slug string) ([]job.Raw, int, error)
}

// beginner opens transactions: *pgx.Conn and *pgxpool.Pool both are.
type beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Result counts what one company's collection saw.
type Result struct{ Fetched, New, Existing, Skipped int }

// Once fetches one board by slug and stores every posting in one transaction, creating
// the company row if needed (with no track: it is not on the watch list). A failure at
// any point writes nothing. Connect as the app role (ADR-0004).
func Once(ctx context.Context, db beginner, src Source, slug string, log *slog.Logger) (Result, error) {
	return collect(ctx, db, src, slug, log, func(q *store.Queries) (int64, error) {
		return companyID(ctx, q, src.Name(), slug)
	})
}

// collect fetches slug's board, then stores it in one transaction under the company
// that companyID returns. Fetch comes first, so a failed fetch touches nothing.
//
// Logs carry counts and IDs only: posting text is untrusted and never logged.
func collect(ctx context.Context, db beginner, src Source, slug string, log *slog.Logger,
	companyID func(*store.Queries) (int64, error),
) (Result, error) {
	raws, skipped, err := src.Fetch(ctx, slug)
	if err != nil {
		return Result{}, err // adapters name the company
	}
	res := Result{Fetched: len(raws), Skipped: skipped}
	var newIDs []string

	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		co, err := companyID(q)
		if err != nil {
			return err
		}
		for _, r := range raws {
			inserted, err := q.UpsertPosting(ctx, store.UpsertPostingParams{
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
			if inserted {
				newIDs = append(newIDs, r.SourceID)
			}
		}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("collect %s %s: %w", src.Name(), slug, err)
	}

	// Logged only after commit, so the log never reports rows that were rolled back.
	for _, id := range newIDs {
		log.Info("new posting", "source", src.Name(), "company", slug, "posting_id", id)
	}
	res.New = len(newIDs)
	res.Existing = res.Fetched - res.New
	log.Info("collect finished", "source", src.Name(), "company", slug,
		"fetched", res.Fetched, "new", res.New, "existing", res.Existing, "skipped", res.Skipped)
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
