// Package collector fetches postings from sources and stores them.
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

// boardWide is a Source whose postings belong to many companies (ADR-0029): each Raw
// names its company, and closing is scoped to the source, not one company.
type boardWide interface{ BoardWide() bool }

// minInterval is a Source fetched at most once per interval (Remotive: 4 a day).
type minInterval interface{ MinInterval() time.Duration }

func isBoardWide(src Source) bool {
	b, ok := src.(boardWide)
	return ok && b.BoardWide()
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
	board := isBoardWide(src)

	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		co, err := companyID(q)
		if err != nil {
			return err
		}
		if len(raws) == 0 {
			open, err := countOpen(ctx, q, board, co, src.Name())
			if err != nil {
				return fmt.Errorf("counting open postings: %w", err)
			}
			if open > EmptyBoardGuard {
				return fmt.Errorf("empty board, refusing to close %d postings", open)
			}
		}
		// Skipped entries with a readable ID are still on the board: never close them.
		seen := append(make([]string, 0, len(raws)+len(skipped.IDs)), skipped.IDs...)
		owners := map[string]int64{} // a board's company names, resolved once per fetch
		for _, r := range raws {
			seen = append(seen, r.SourceID)
			owner := co
			if board {
				if owner, err = boardCompany(ctx, q, owners, src.Name(), r.Company); err != nil {
					return err
				}
			}
			r2, err := q.UpsertPosting(ctx, store.UpsertPostingParams{
				CompanyID:       owner,
				Source:          r.Source,
				SourcePostingID: r.SourceID,
				Url:             r.URL,
				Title:           r.Title,
				RawPayload:      r.Payload,
			})
			if err != nil {
				return fmt.Errorf("storing posting %s: %w", r.SourceID, err)
			}
			if board && r2.Inserted {
				if err := q.MarkBoard(ctx, r2.ID); err != nil {
					return fmt.Errorf("marking posting %s: %w", r.SourceID, err)
				}
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
			ok, err := writeJob(ctx, q, r2.ID, owner, r.Source, r.Payload, r2.ContentHash, log)
			if err != nil {
				return err
			}
			if !ok {
				res.JobsFailed++
			}
		}
		// The fetch succeeded (we are here), so whatever the board no longer lists is gone,
		// unless that is most of the board, which looks like a partial outage.
		toClose, err := countNotIn(ctx, q, board, co, src.Name(), seen)
		if err != nil {
			return fmt.Errorf("counting postings to close: %w", err)
		}
		if toClose >= CloseRatioGuardMin {
			open, err := countOpen(ctx, q, board, co, src.Name())
			if err != nil {
				return fmt.Errorf("counting open postings: %w", err)
			}
			if toClose*2 > open {
				return fmt.Errorf("refusing to close %d of %d postings", toClose, open)
			}
		}
		var closed int64
		if board {
			closed, err = q.CloseSourcePostingsNotIn(ctx, store.CloseSourcePostingsNotInParams{Source: src.Name(), SeenIds: seen})
		} else {
			closed, err = q.ClosePostingsNotIn(ctx, store.ClosePostingsNotInParams{CompanyID: co, Source: src.Name(), SeenIds: seen})
		}
		if err != nil {
			return fmt.Errorf("closing missing postings: %w", err)
		}
		res.Closed = int(closed)
		if err := q.SetFetchedAt(ctx, co); err != nil {
			return fmt.Errorf("recording the fetch: %w", err)
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

// countOpen counts the open postings a fetch could close: the company's from this source,
// or a board-wide source's across companies (ADR-0029).
func countOpen(ctx context.Context, q *store.Queries, board bool, co int64, source string) (int64, error) {
	if board {
		return q.CountOpenSourcePostings(ctx, source)
	}
	return q.CountOpenPostings(ctx, store.CountOpenPostingsParams{CompanyID: co, Source: source})
}

func countNotIn(ctx context.Context, q *store.Queries, board bool, co int64, source string, seen []string) (int64, error) {
	if board {
		return q.CountSourcePostingsNotIn(ctx, store.CountSourcePostingsNotInParams{Source: source, SeenIds: seen})
	}
	return q.CountPostingsNotIn(ctx, store.CountPostingsNotInParams{CompanyID: co, Source: source, SeenIds: seen})
}

// boardCompany is a board posting's company (ADR-0029): a company of that name, a
// watch-list one first, else a new board-less one. Names are untrusted: trimmed, and only
// compared and stored, as manual adds' are.
func boardCompany(ctx context.Context, q *store.Queries, cache map[string]int64, boardType, name string) (int64, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return 0, errors.New("board posting without a company")
	}
	key := strings.ToLower(name)
	if id, ok := cache[key]; ok {
		return id, nil
	}
	id, err := q.FindCompanyByName(ctx, store.FindCompanyByNameParams{Name: name, BoardType: boardType})
	if errors.Is(err, pgx.ErrNoRows) {
		id, err = q.EnsureBoardlessCompany(ctx, name)
	}
	if err != nil {
		return 0, fmt.Errorf("company %q: %w", name, err)
	}
	cache[key] = id
	return id, nil
}
