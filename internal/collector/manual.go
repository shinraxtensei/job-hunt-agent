package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
	"github.com/shinraxtensei/job-hunt-agent/internal/source/fetch"
	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// PostingSource is a Source that can fetch one posting by board and id, from its own
// fixed API endpoint (M2-07, ADR-0015).
type PostingSource interface {
	Source
	FetchOne(ctx context.Context, board, id string, limit *rate.Limiter) (job.Raw, error)
}

// Manual is what the owner types for a link that isn't a board posting.
type Manual struct{ Title, Company, Location string }

// AddResult says what `collector add` did.
type AddResult struct {
	PostingID int64
	Existed   bool // already stored: nothing was fetched or written
	Board     bool // a board posting, fetched through its adapter
}

// maxTyped caps each typed field, like the normaliser caps fields it copies.
const maxTyped = 500

// Add stores a job from its link (M2-07, ADR-0015). A link that job.ParseBoardLink accepts
// is fetched through that board's adapter, by board and id, never the link itself; its
// company is created inactive if new. Any other link is never fetched: it is stored from
// m, which then needs a title and a company. Either way the posting is normalised and
// classified in the same transaction, and a link already stored is reported, not added.
// interval is the minimum gap between requests to the board's API host.
func Add(ctx context.Context, db DB, reg Registry, interval time.Duration, link string, m Manual, log *slog.Logger) (AddResult, error) {
	if b, ok := job.ParseBoardLink(link); ok {
		return addBoard(ctx, db, reg, interval, b, log)
	}
	return addOther(ctx, db, link, m, log)
}

func existing(ctx context.Context, q *store.Queries, source, id string) (int64, bool, error) {
	pid, err := q.GetPostingID(ctx, store.GetPostingIDParams{Source: source, SourcePostingID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("looking up %s posting: %w", source, err)
	}
	return pid, true, nil
}

func addBoard(ctx context.Context, db DB, reg Registry, interval time.Duration, b job.BoardLink, log *slog.Logger) (AddResult, error) {
	if id, ok, err := existing(ctx, store.New(db), b.Source, b.ID); err != nil || ok {
		return AddResult{PostingID: id, Existed: ok, Board: true}, err
	}
	src, ok := reg[b.Source].(PostingSource)
	if !ok {
		return AddResult{}, fmt.Errorf("no %s adapter can fetch single postings", b.Source)
	}
	raw, err := src.FetchOne(ctx, b.Board, b.ID, limiter(interval))
	if errors.Is(err, fetch.ErrNotFound) {
		return AddResult{}, fmt.Errorf("%s board %q has no posting %s: it may have closed", b.Source, b.Board, b.ID)
	}
	if err != nil {
		return AddResult{}, err
	}
	var res AddResult
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		companyID, err := q.EnsureManualBoardCompany(ctx, store.EnsureManualBoardCompanyParams{
			Name: b.Board, BoardType: b.Source, BoardSlug: b.Board,
		})
		if err != nil {
			return fmt.Errorf("creating company %s/%s: %w", b.Source, b.Board, err)
		}
		res, err = storeManual(ctx, q, companyID, raw, log)
		if err != nil {
			return err
		}
		return q.SetRechecked(ctx, res.PostingID)
	})
	res.Board = true
	return res, err
}

func addOther(ctx context.Context, db DB, link string, m Manual, log *slog.Logger) (AddResult, error) {
	if job.InsecureBoardLink(link) {
		return AddResult{}, errors.New("this is an http:// link to a job board: use the https:// link, so the posting is fetched from the board instead of stored as a manual duplicate")
	}
	norm, err := job.NormaliseLink(link)
	if err != nil {
		return AddResult{}, fmt.Errorf("%w (only Lever, Greenhouse and Ashby postings are fetched; other links are stored, never fetched)", err)
	}
	m.Title, m.Company, m.Location = strings.TrimSpace(m.Title), strings.TrimSpace(m.Company), strings.TrimSpace(m.Location)
	var missing []string
	if m.Title == "" {
		missing = append(missing, "--title")
	}
	if m.Company == "" {
		missing = append(missing, "--company")
	}
	if len(missing) > 0 {
		return AddResult{}, fmt.Errorf("this isn't a Lever, Greenhouse or Ashby posting, so it is never fetched: give %s",
			strings.Join(missing, " and "))
	}
	for name, v := range map[string]string{"--title": m.Title, "--company": m.Company, "--location": m.Location} {
		if len([]rune(v)) > maxTyped || strings.ContainsRune(v, 0) {
			return AddResult{}, fmt.Errorf("%s is longer than %d characters or contains NUL", name, maxTyped)
		}
	}
	if id, ok, err := existing(ctx, store.New(db), "manual", norm); err != nil || ok {
		return AddResult{PostingID: id, Existed: ok}, err
	}
	payload, err := json.Marshal(map[string]string{"title": m.Title, "company": m.Company, "location": m.Location, "url": norm})
	if err != nil {
		return AddResult{}, fmt.Errorf("encoding manual posting: %w", err)
	}
	var res AddResult
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		companyID, err := q.EnsureBoardlessCompany(ctx, m.Company)
		if err != nil {
			return fmt.Errorf("creating company: %w", err)
		}
		res, err = storeManual(ctx, q, companyID, job.Raw{Source: "manual", SourceID: norm, URL: norm, Title: m.Title, Payload: payload}, log)
		return err
	})
	return res, err
}

// storeManual stores raw as a manual posting of companyID and writes its job record.
func storeManual(ctx context.Context, q *store.Queries, companyID int64, raw job.Raw, log *slog.Logger) (AddResult, error) {
	up, err := q.UpsertPosting(ctx, store.UpsertPostingParams{
		CompanyID: companyID, Source: raw.Source, SourcePostingID: raw.SourceID,
		Url: raw.URL, Title: raw.Title, RawPayload: raw.Payload,
	})
	if err != nil {
		return AddResult{}, fmt.Errorf("storing posting: %w", err)
	}
	if err := q.MarkManual(ctx, up.ID); err != nil {
		return AddResult{}, fmt.Errorf("marking posting %d manual: %w", up.ID, err)
	}
	if _, err := writeJob(ctx, q, up.ID, companyID, raw.Source, raw.Payload, up.ContentHash, log); err != nil {
		return AddResult{}, err
	}
	return AddResult{PostingID: up.ID, Existed: !up.Inserted}, nil
}

func limiter(interval time.Duration) *rate.Limiter {
	if interval <= 0 {
		return rate.NewLimiter(rate.Inf, 1)
	}
	return rate.NewLimiter(rate.Every(interval), 1)
}

// recheckResult counts what recheckManual did.
type recheckResult struct{ Rechecked, Closed, Reopened int }

// recheckManual fetches, one by one, the manual board postings of companies that aren't
// collected and that weren't rechecked for two days (M2-07 AC4): gone (404) → closed;
// found → stored like a collected posting (reopened if it was closed). It waits on the
// run's per-host limiters. A failed recheck is logged and tried again next run.
func recheckManual(ctx context.Context, db DB, reg Registry, limiters map[string]*rate.Limiter, interval time.Duration, log *slog.Logger) (recheckResult, error) {
	var res recheckResult
	due, err := store.New(db).ListManualToRecheck(ctx)
	if err != nil {
		return res, fmt.Errorf("listing manual postings to recheck: %w", err)
	}
	for _, p := range due {
		if ctx.Err() != nil {
			return res, nil // cancelled: the run reports it
		}
		src, ok := reg[p.Source].(PostingSource)
		if !ok {
			continue
		}
		lim := limiters[src.Host()]
		if lim == nil {
			lim = limiter(interval)
			limiters[src.Host()] = lim
		}
		raw, err := src.FetchOne(ctx, p.BoardSlug, p.SourcePostingID, lim)
		switch {
		case errors.Is(err, fetch.ErrNotFound):
			n, err := store.New(db).CloseRechecked(ctx, p.ID)
			if err != nil {
				return res, fmt.Errorf("closing posting %d: %w", p.ID, err)
			}
			res.Closed += int(n)
		case err != nil:
			log.Warn("recheck failed", "posting_id", p.ID, "source", p.Source, "err", err)
			continue
		default:
			err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
				q := store.New(tx)
				up, err := q.UpsertPosting(ctx, store.UpsertPostingParams{
					CompanyID: p.CompanyID, Source: raw.Source, SourcePostingID: raw.SourceID,
					Url: raw.URL, Title: raw.Title, RawPayload: raw.Payload,
				})
				if err != nil {
					return fmt.Errorf("storing posting %d: %w", p.ID, err)
				}
				if up.Reopened {
					res.Reopened++
				}
				if up.Changed || up.Reopened {
					if _, err := writeJob(ctx, q, up.ID, p.CompanyID, raw.Source, raw.Payload, up.ContentHash, log); err != nil {
						return err
					}
				}
				return q.SetRechecked(ctx, p.ID)
			})
			if err != nil {
				return res, err
			}
		}
		res.Rechecked++
	}
	return res, nil
}
