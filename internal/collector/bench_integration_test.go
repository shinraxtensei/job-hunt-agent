//go:build integration

package collector_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunt-agent/internal/collector"
	"github.com/shinraxtensei/job-hunt-agent/internal/job"
	"github.com/shinraxtensei/job-hunt-agent/internal/store/storetest"
)

const benchBoard = 1000

// bigBoard returns the same benchBoard Lever-shaped postings on every fetch; gen changes
// their content (a new generation is a board where every posting changed).
type bigBoard struct{ gen int }

func (*bigBoard) Name() string { return "lever" }
func (*bigBoard) Host() string { return "bench.test" }
func (s *bigBoard) Fetch(ctx context.Context, slug string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	if err := limit.Wait(ctx); err != nil {
		return nil, job.Skipped{}, err
	}
	out := make([]job.Raw, benchBoard)
	for i := range out {
		id := fmt.Sprintf("%s-%04d", slug, i)
		out[i] = job.Raw{
			Source: "lever", SourceID: id, URL: "https://jobs.lever.co/bench/" + id, Title: "Backend Engineer",
			Payload: fmt.Appendf(nil, `{"id":%q,"text":"Backend Engineer %d","categories":{"location":"Berlin, Germany",`+
				`"team":"Platform","commitment":"Full-time"},"workplaceType":"hybrid","createdAt":1730000000000,`+
				`"descriptionPlain":"We build payments in Go and PostgreSQL. 5+ years of experience. Gen %d."}`, id, i, s.gen),
		}
	}
	return out, job.Skipped{}, nil
}

func benchPool(b *testing.B) *pgxpool.Pool {
	b.Helper()
	storetest.Migrate(b, storetest.OwnerURL)
	pool, err := pgxpool.New(b.Context(), storetest.AppURL)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	return pool
}

func report(b *testing.B) {
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N*benchBoard), "µs/posting")
}

// M8-02 AC3: a board of 1,000 postings seen for the first time (insert, normalise,
// classify, job record), one transaction.
func BenchmarkStoreNewBoard(b *testing.B) {
	pool := benchPool(b)
	src := &bigBoard{}
	i := 0
	for b.Loop() {
		i++
		if _, err := collector.Once(b.Context(), pool, src, fmt.Sprintf("new-%d-%d", b.N, i), quiet()); err != nil {
			b.Fatal(err)
		}
	}
	report(b)
}

// The daily case: the same 1,000 postings again, nothing changed (hash compare, no job
// rewrite).
func BenchmarkStoreUnchangedBoard(b *testing.B) {
	pool := benchPool(b)
	src := &bigBoard{}
	slug := fmt.Sprintf("same-%d", b.N)
	if _, err := collector.Once(b.Context(), pool, src, slug, quiet()); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := collector.Once(b.Context(), pool, src, slug, quiet()); err != nil {
			b.Fatal(err)
		}
	}
	report(b)
}

// Every posting changed: update, normalise and rewrite each job record.
func BenchmarkStoreChangedBoard(b *testing.B) {
	pool := benchPool(b)
	src := &bigBoard{}
	slug := fmt.Sprintf("changed-%d", b.N)
	if _, err := collector.Once(b.Context(), pool, src, slug, quiet()); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		src.gen++
		if _, err := collector.Once(b.Context(), pool, src, slug, quiet()); err != nil {
			b.Fatal(err)
		}
	}
	report(b)
}
