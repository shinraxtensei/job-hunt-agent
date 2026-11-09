package collector

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// writeJob normalises one stored posting and writes its job record (ADR-0008). A payload
// that can't be normalised is logged by posting ID and reported as ok=false: the posting
// stays stored, and the company or batch carries on. err is only for the database.
func writeJob(ctx context.Context, q *store.Queries, postingID, companyID int64, source string,
	payload, postingHash []byte, log *slog.Logger,
) (ok bool, err error) {
	rec, nerr := job.Normalise(source, payload)
	if nerr != nil {
		log.Warn("could not normalise posting", "posting_id", postingID, "source", source, "err", nerr)
		return false, nil
	}
	f := job.Classify(rec, job.Description(source, payload))
	var published pgtype.Timestamptz
	if !rec.PublishedAt.IsZero() {
		published = pgtype.Timestamptz{Time: rec.PublishedAt, Valid: true}
	}
	if err := q.UpsertJob(ctx, store.UpsertJobParams{
		PostingID:          postingID,
		CompanyID:          companyID,
		Title:              rec.Title,
		Url:                rec.URL,
		Locations:          nonNil(rec.Locations),
		CountryCodes:       nonNil(rec.CountryCodes),
		Workplace:          string(rec.Workplace),
		Team:               rec.Team,
		Department:         rec.Department,
		EmploymentType:     rec.EmploymentType,
		PublishedAt:        published,
		NormaliserVersion:  job.NormaliserVersion,
		PostingHash:        postingHash,
		TitleKey:           rec.TitleKey,
		RoleFamily:         f.RoleFamily,
		Seniority:          f.Seniority,
		ExperienceMinYears: years(f),
		RemoteScopes:       nonNil(f.RemoteScopes),
		Countries:          nonNil(f.Countries),
		ClassifierVersion:  job.ClassifierVersion,
	}); err != nil {
		return false, fmt.Errorf("writing job record for posting %d: %w", postingID, err)
	}
	return true, nil
}

// years is the facet's minimum experience as a nullable column: 0 means none stated.
func years(f job.Facets) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(f.ExperienceMinYears), Valid: f.ExperienceMinYears > 0} //nolint:gosec // 1..20
}

// nonNil turns a nil slice into an empty one: the columns are NOT NULL text[].
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// NormaliseResult counts what `collector normalise` did.
type NormaliseResult struct{ Batches, Written, Failed int }

// NormaliseStored (re)writes the job record of every stored posting whose record is
// missing, made by an older normaliser version, or made from an older payload, batchSize
// postings per transaction. Postings that can't be normalised are counted and skipped.
func NormaliseStored(ctx context.Context, db DB, batchSize int, log *slog.Logger) (NormaliseResult, error) {
	var res NormaliseResult
	var after int64
	for {
		var rows []store.ListPostingsToNormaliseRow
		err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
			q := store.New(tx)
			var err error
			rows, err = q.ListPostingsToNormalise(ctx, store.ListPostingsToNormaliseParams{
				AfterID: after, Version: job.NormaliserVersion, BatchSize: int32(batchSize), //nolint:gosec // small, from cmd
			})
			if err != nil {
				return fmt.Errorf("listing postings to normalise: %w", err)
			}
			for _, p := range rows {
				ok, err := writeJob(ctx, q, p.ID, p.CompanyID, p.Source, p.RawPayload, p.ContentHash, log)
				if err != nil {
					return err
				}
				if ok {
					res.Written++
				} else {
					res.Failed++
				}
			}
			return nil
		})
		if err != nil {
			return res, err
		}
		if len(rows) == 0 {
			break
		}
		res.Batches++
		after = rows[len(rows)-1].ID
	}
	log.Info("normalise finished", "version", job.NormaliserVersion, "batches", res.Batches,
		"written", res.Written, "failed", res.Failed)
	return res, nil
}

// ClassifyResult counts what `collector classify` did.
type ClassifyResult struct{ Batches, Written, Failed int }

// ClassifyStored rewrites the facets of every job made by an older classifier version
// (ADR-0011), batchSize jobs per transaction. The record itself is not rewritten; the
// payload is read again only for the title, locations and description. Payloads that can't
// be normalised are counted and skipped.
func ClassifyStored(ctx context.Context, db DB, batchSize int, log *slog.Logger) (ClassifyResult, error) {
	var res ClassifyResult
	var after int64
	for {
		var rows []store.ListJobsToClassifyRow
		err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
			q := store.New(tx)
			var err error
			rows, err = q.ListJobsToClassify(ctx, store.ListJobsToClassifyParams{
				AfterID: after, Version: job.ClassifierVersion, BatchSize: int32(batchSize), //nolint:gosec // small, from cmd
			})
			if err != nil {
				return fmt.Errorf("listing jobs to classify: %w", err)
			}
			for _, j := range rows {
				rec, nerr := job.Normalise(j.Source, j.RawPayload)
				if nerr != nil {
					log.Warn("could not classify job", "posting_id", j.PostingID, "source", j.Source, "err", nerr)
					res.Failed++
					continue
				}
				f := job.Classify(rec, job.Description(j.Source, j.RawPayload))
				if err := q.UpdateJobFacets(ctx, store.UpdateJobFacetsParams{
					PostingID: j.PostingID, RoleFamily: f.RoleFamily, Seniority: f.Seniority,
					ExperienceMinYears: years(f), RemoteScopes: nonNil(f.RemoteScopes), Countries: nonNil(f.Countries),
					ClassifierVersion: job.ClassifierVersion,
				}); err != nil {
					return fmt.Errorf("writing facets of posting %d: %w", j.PostingID, err)
				}
				res.Written++
			}
			return nil
		})
		if err != nil {
			return res, err
		}
		if len(rows) == 0 {
			break
		}
		res.Batches++
		after = rows[len(rows)-1].PostingID
	}
	log.Info("classify finished", "version", job.ClassifierVersion, "batches", res.Batches,
		"written", res.Written, "failed", res.Failed)
	return res, nil
}

// LocationCount is one location string and how many jobs name it.
type LocationCount struct {
	Location string
	Jobs     int
}

// UnresolvedLocations returns up to limit location strings the classifier can't place
// (job.Unresolved), most common first, so gaps in its tables are visible (M2-09).
func UnresolvedLocations(ctx context.Context, db DB, limit int) ([]LocationCount, error) {
	rows, err := store.New(db).CountJobLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting job locations: %w", err)
	}
	var out []LocationCount
	for _, r := range rows {
		if len(out) == limit {
			break
		}
		if job.Unresolved(r.Location) {
			out = append(out, LocationCount{r.Location, int(r.Jobs)})
		}
	}
	return out, nil
}
