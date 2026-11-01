// Package collector fetches postings from sources and stores them.
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunter/internal/source/lever"
	"github.com/shinraxtensei/job-hunter/internal/store"
)

// Result counts what one run saw.
type Result struct{ Fetched, New, Existing, Skipped int }

// Once fetches company's Lever board and stores every posting in one transaction, so a
// failure at any point writes nothing. Connect as the app role (ADR-0004).
//
// Logs carry counts and IDs only: posting text is untrusted and never logged.
//
// ponytail: Lever only and one company; M1-02 adds the Source interface and concurrency.
func Once(ctx context.Context, conn *pgx.Conn, src *lever.Client, company string, log *slog.Logger) (Result, error) {
	// Fetch before opening the transaction: a failed fetch touches nothing.
	raws, skipped, err := src.Fetch(ctx, company)
	if err != nil {
		return Result{}, err // already names the company
	}
	res := Result{Fetched: len(raws), Skipped: skipped}
	var newIDs []string

	err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		q := store.New(tx)
		co, err := companyID(ctx, q, company)
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
		return Result{}, fmt.Errorf("collect lever %s: %w", company, err)
	}

	// Logged only after commit, so the log never reports rows that were rolled back.
	for _, id := range newIDs {
		log.Info("new posting", "source", "lever", "company", company, "posting_id", id)
	}
	res.New = len(newIDs)
	res.Existing = res.Fetched - res.New
	log.Info("collect finished", "source", "lever", "company", company,
		"fetched", res.Fetched, "new", res.New, "existing", res.Existing, "skipped", res.Skipped)
	return res, nil
}

// companyID returns the id of company's Lever row, creating it if needed.
// ponytail: select-then-insert, safe only without concurrent runs; M1-01 adds
// UNIQUE (board_type, board_slug) and can switch to an upsert.
func companyID(ctx context.Context, q *store.Queries, slug string) (int64, error) {
	co, err := q.GetCompanyByBoard(ctx, store.GetCompanyByBoardParams{BoardType: "lever", BoardSlug: slug})
	if err == nil {
		return co.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("looking up company: %w", err)
	}
	co, err = q.CreateCompany(ctx, store.CreateCompanyParams{Name: slug, BoardType: "lever", BoardSlug: slug})
	if err != nil {
		return 0, fmt.Errorf("creating company: %w", err)
	}
	return co.ID, nil
}
