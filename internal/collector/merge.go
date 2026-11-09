package collector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// ErrMerge is a merge the owner must fix (an unknown or ambiguous company, a watch-list
// company merged away, conflicting applications); nothing was changed.
var ErrMerge = errors.New("can't merge")

// MergePlan is what a company merge moves (M6-06).
type MergePlan struct {
	From, Into                   store.FindCompaniesRow
	Postings, Jobs, Applications int64
}

func (p MergePlan) String() string {
	return fmt.Sprintf("%s (#%d) into %s (#%d): %d postings, %d jobs, %d applications",
		p.From.Name, p.From.ID, p.Into.Name, p.Into.ID, p.Postings, p.Jobs, p.Applications)
}

// pick resolves a merge argument (an id or a name) to one company; manual says which kind
// a name should prefer when both exist.
func pick(rows []store.FindCompaniesRow, arg string, manual bool) (store.FindCompaniesRow, error) {
	var want []store.FindCompaniesRow
	for _, r := range rows {
		if fmt.Sprint(r.ID) == arg || !r.BoardType.Valid == manual {
			want = append(want, r)
		}
	}
	if len(want) == 0 {
		want = rows
	}
	switch len(want) {
	case 0:
		return store.FindCompaniesRow{}, fmt.Errorf("%w: no company %q", ErrMerge, arg)
	case 1:
		return want[0], nil
	}
	ids := make([]string, len(want))
	for i, r := range want {
		ids[i] = fmt.Sprintf("#%d", r.ID)
	}
	return store.FindCompaniesRow{}, fmt.Errorf("%w: %q is %s: give the id", ErrMerge, arg, strings.Join(ids, ", "))
}

// MergeCompany moves a manual (board-less) company's postings, jobs and applications into
// another company in one transaction. The emptied company stays as it was: inactive, as
// hand-added companies always are (ADR-0015), and never deleted.
// Duplicate groups follow (job_groups is a view). With dryRun nothing changes.
func MergeCompany(ctx context.Context, db DB, from, into string, dryRun bool) (MergePlan, error) {
	var plan MergePlan
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := store.New(tx)
		fr, err := q.FindCompanies(ctx, from)
		if err != nil {
			return fmt.Errorf("finding %q: %w", from, err)
		}
		if plan.From, err = pick(fr, from, true); err != nil {
			return err
		}
		ir, err := q.FindCompanies(ctx, into)
		if err != nil {
			return fmt.Errorf("finding %q: %w", into, err)
		}
		if plan.Into, err = pick(ir, into, false); err != nil {
			return err
		}
		switch {
		case plan.From.ID == plan.Into.ID:
			return fmt.Errorf("%w: a company can't be merged into itself", ErrMerge)
		case plan.From.BoardType.Valid:
			return fmt.Errorf("%w: %s (#%d) is on the watch list (%s); only a company added by hand can be merged away",
				ErrMerge, plan.From.Name, plan.From.ID, plan.From.BoardType.String)
		}
		conflicts, err := q.MergeConflicts(ctx, store.MergeConflictsParams{FromID: plan.From.ID, IntoID: plan.Into.ID})
		if err != nil {
			return fmt.Errorf("checking applications: %w", err)
		}
		if len(conflicts) > 0 {
			return fmt.Errorf("%w: both companies have an application for %s; close one of them first",
				ErrMerge, strings.Join(conflicts, ", "))
		}
		c, err := q.MergeCounts(ctx, plan.From.ID)
		if err != nil {
			return fmt.Errorf("counting: %w", err)
		}
		plan.Postings, plan.Jobs, plan.Applications = c.Postings, c.Jobs, c.Applications
		if dryRun {
			return nil
		}
		p := store.MovePostingsParams{FromID: plan.From.ID, IntoID: plan.Into.ID}
		if _, err := q.MovePostings(ctx, p); err != nil {
			return fmt.Errorf("moving postings: %w", err)
		}
		if _, err := q.MoveJobs(ctx, store.MoveJobsParams(p)); err != nil {
			return fmt.Errorf("moving jobs: %w", err)
		}
		if _, err := q.MoveApplications(ctx, store.MoveApplicationsParams(p)); err != nil {
			return fmt.Errorf("moving applications: %w", err)
		}
		return nil
	})
	return plan, err
}
