// Command collector fetches job postings and stores them. Run from the repo root.
//
//	collector run              every active, non-excluded company, concurrently
//	collector once             one Lever board (COLLECTOR_LEVER_COMPANY)
//	collector companies sync   data/companies.yaml into the companies table
//	collector normalise        (re)build missing or stale job records (ADR-0008)
//	collector classify         rebuild facets made by an older classifier (ADR-0011)
//	collector classify --report-unknown
//	                           list the location strings the classifier can't place (M2-09)
//	collector preview <name>   show what data/presets/<name>.yaml shows and hides, and why
//	collector add <url> [--title T --company C --location L]
//	                           add a job by its link (M2-07, ADR-0015): board postings are
//	                           fetched through the board's API; other links never are
//	collector enqueue          queue an extract item for every lead the default preset
//	                           shows that has none for its content (ADR-0001); run, add
//	                           and the worker's start do this too
//	collector enqueue --reextract [--dry-run]
//	                           re-queue the shown postings extracted with an older prompt,
//	                           after printing how many and the estimated cost (M3-04)
//	collector queue            work items by state
//	collector queue retry --failed [--kind extract|reason]
//	                           failed items back to pending with 0 attempts (M3-04)
//
// Environment: DATABASE_URL (app role, required); COLLECTOR_CONCURRENCY (default 8);
// COLLECTOR_HOST_INTERVAL (minimum gap per host, default and minimum 1s);
// LEVER_BASE_URL (default https://api.lever.co; EU boards use https://api.eu.lever.co).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/preset"
	"github.com/shinraxtensei/job-hunter/internal/queue"
	"github.com/shinraxtensei/job-hunter/internal/source/ashby"
	"github.com/shinraxtensei/job-hunter/internal/source/greenhouse"
	"github.com/shinraxtensei/job-hunter/internal/source/lever"
	"github.com/shinraxtensei/job-hunter/internal/watchlist"
)

const (
	usage = "usage: collector run | collector once | collector companies sync | collector normalise | collector classify [--report-unknown] | collector preview <preset> | collector add <url> [--title T --company C --location L] | collector enqueue [--reextract [--dry-run]] | collector queue [retry --failed [--kind extract|reason]]"

	// normaliseBatch is how many postings `collector normalise` writes per transaction.
	normaliseBatch = 500
	// reportUnknownTop is how many location strings `classify --report-unknown` lists.
	reportUnknownTop = 40

	// watchListPath is relative to the working directory: run from the repo root.
	watchListPath = "data/companies.yaml"
	// presetsDir holds the filter presets (ADR-0011).
	presetsDir = "data/presets"

	// stopDeadline is how long the process may take to exit after SIGINT/SIGTERM (AC4).
	stopDeadline = 10 * time.Second
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		// Cancellation is cooperative; if anything ignores it, exit anyway. Postgres
		// rolls back any open transaction when the connection drops, so nothing is
		// half-written either way.
		<-ctx.Done()
		time.Sleep(stopDeadline - time.Second)
		log.Error("collector did not stop in time after a signal; exiting")
		os.Exit(1)
	}()
	code, err := run(ctx, log, os.Args[1:], os.Getenv)
	stop()
	if err != nil {
		attrs := []any{"command", strings.Join(os.Args[1:], " "), "err", err}
		if c := os.Getenv("COLLECTOR_LEVER_COMPANY"); c != "" && len(os.Args) == 2 && os.Args[1] == "once" {
			attrs = append(attrs, "company", c)
		}
		log.Error("collector failed", attrs...)
	}
	os.Exit(code)
}

// run executes one command and returns the exit code.
func run(ctx context.Context, log *slog.Logger, args []string, getenv func(string) string) (int, error) {
	dsn := getenv("DATABASE_URL")
	if dsn == "" {
		return 1, errors.New("DATABASE_URL must be set")
	}
	leverURL := getenv("LEVER_BASE_URL")
	if leverURL == "" {
		leverURL = lever.BaseURL
	}

	if len(args) == 2 && args[0] == "preview" {
		return preview(ctx, dsn, args[1])
	}
	if len(args) >= 1 && args[0] == "add" {
		return add(ctx, log, dsn, leverURL, args[1:], getenv)
	}

	switch strings.Join(args, " ") {
	case "run":
		cfg, err := runConfig(getenv)
		if err != nil {
			return 1, err
		}
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return 1, fmt.Errorf("connecting to database: %w", err)
		}
		defer pool.Close()
		reg := collector.Registry{
			"lever":      lever.New(leverURL),
			"greenhouse": greenhouse.New(greenhouse.BaseURL),
			"ashby":      ashby.New(ashby.BaseURL),
		}
		res, err := collector.Run(ctx, pool, reg, cfg, log)
		if err != nil {
			return 1, err
		}
		if code := exitCode(res.Outcome); code != 0 {
			return code, nil
		}
		return reconcile(ctx, log, pool)

	case "enqueue":
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return 1, fmt.Errorf("connecting to database: %w", err)
		}
		defer pool.Close()
		return reconcile(ctx, log, pool)

	case "enqueue --reextract", "enqueue --reextract --dry-run":
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return 1, fmt.Errorf("connecting to database: %w", err)
		}
		defer pool.Close()
		return reextract(ctx, pool, strings.HasSuffix(strings.Join(args, " "), "--dry-run"))

	case "queue retry --failed", "queue retry --failed --kind extract", "queue retry --failed --kind reason":
		kind := "extract"
		if strings.HasSuffix(strings.Join(args, " "), "reason") {
			kind = "reason" // M3-06: queued by the worker itself (ADR-0021)
		}
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return 1, fmt.Errorf("connecting to database: %w", err)
		}
		defer pool.Close()
		n, err := queue.RetryFailed(ctx, pool, kind)
		if err != nil {
			return 1, err
		}
		fmt.Printf("retrying %d failed %s items\n", n, kind)
		return 0, nil

	case "queue":
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return 1, fmt.Errorf("connecting to database: %w", err)
		}
		defer pool.Close()
		c, err := queue.Count(ctx, pool)
		if err != nil {
			return 1, err
		}
		fmt.Printf("pending %d\nrunning %d\nfailed  %d (needs review)\ndone    %d\n", c.Pending, c.Running, c.Failed, c.Done)
		return 0, nil

	case "normalise":
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return 1, fmt.Errorf("connecting to database: %w", err)
		}
		defer pool.Close()
		// Failures are counted in the report; they don't fail the command.
		if _, err := collector.NormaliseStored(ctx, pool, normaliseBatch, log); err != nil {
			return 1, err
		}
		return 0, nil

	case "classify":
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return 1, fmt.Errorf("connecting to database: %w", err)
		}
		defer pool.Close()
		if _, err := collector.ClassifyStored(ctx, pool, normaliseBatch, log); err != nil {
			return 1, err
		}
		return 0, nil

	case "classify --report-unknown":
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return 1, fmt.Errorf("connecting to database: %w", err)
		}
		defer pool.Close()
		locs, err := collector.UnresolvedLocations(ctx, pool, reportUnknownTop)
		if err != nil {
			return 1, err
		}
		fmt.Printf("%d most common location strings the classifier can't place (jobs, location):\n", len(locs))
		for _, l := range locs {
			fmt.Printf("%6d  %q\n", l.Jobs, l.Location) // quoted: posting text is untrusted
		}
		return 0, nil

	case "once":
		company := getenv("COLLECTOR_LEVER_COMPANY")
		if company == "" {
			return 1, errors.New("COLLECTOR_LEVER_COMPANY must be set")
		}
		return withConn(ctx, log, dsn, func(conn *pgx.Conn) error {
			_, err := collector.Once(ctx, conn, lever.New(leverURL), company, log)
			return err
		})

	case "companies sync":
		// Validate the whole file before touching the database.
		entries, err := watchlist.Load(watchListPath)
		if err != nil {
			return 1, err
		}
		return withConn(ctx, log, dsn, func(conn *pgx.Conn) error {
			res, err := watchlist.Sync(ctx, conn, entries)
			if err != nil {
				return err
			}
			log.Info("companies synced", "file", watchListPath, "entries", len(entries),
				"inserted", res.Inserted, "updated", res.Updated, "unchanged", res.Unchanged,
				"deactivated", res.Deactivated)
			return nil
		})

	default:
		return 1, errors.New(usage)
	}
}

// runConfig reads the run's bounds from the environment. The host interval can't go
// below 1s: Lever's robots.txt asks for Crawl-delay: 1, and no board we use documents
// a higher rate.
func runConfig(getenv func(string) string) (collector.Config, error) {
	cfg := collector.Config{Concurrency: 8, HostInterval: time.Second}
	if v := getenv("COLLECTOR_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return cfg, fmt.Errorf("COLLECTOR_CONCURRENCY=%q: want a whole number >= 1", v)
		}
		cfg.Concurrency = n
	}
	if v := getenv("COLLECTOR_HOST_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Second {
			return cfg, fmt.Errorf("COLLECTOR_HOST_INTERVAL=%q: want a duration >= 1s", v)
		}
		cfg.HostInterval = d
	}
	return cfg, nil
}

// exitCode maps a run's outcome to the process exit code. A partial run still did its
// job for every board that answered, so it exits 0; its failures are in
// collector_run_errors.
func exitCode(outcome string) int {
	if outcome == "ok" || outcome == "partial" {
		return 0
	}
	return 1
}

func withConn(ctx context.Context, log *slog.Logger, dsn string, fn func(*pgx.Conn) error) (int, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return 1, fmt.Errorf("connecting to database: %w", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Warn("closing database", "err", err)
		}
	}()
	if err := fn(conn); err != nil {
		return 1, err
	}
	return 0, nil
}

// presetName is a preset file's base name: it never reaches outside presetsDir.
var presetName = regexp.MustCompile(`^[a-z0-9_-]+$`)

// preview prints what a preset shows and hides over the stored groups, with 10 random
// samples of each.
func preview(ctx context.Context, dsn, name string) (int, error) {
	if !presetName.MatchString(name) {
		return 1, fmt.Errorf("preset name %q: want lowercase letters, digits, - or _", name)
	}
	p, err := preset.Load(filepath.Join(presetsDir, name+".yaml"))
	if err != nil {
		return 1, err
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return 1, fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()
	groups, err := collector.Groups(ctx, pool)
	if err != nil {
		return 1, err
	}
	collector.Preview(os.Stdout, p, groups, 10, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))) //nolint:gosec // sampling, not security
	return 0, nil
}

// parseAddArgs reads `add <url> [--title T] [--company C] [--location L]`.
func parseAddArgs(args []string) (string, collector.Manual, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", collector.Manual{}, errors.New("usage: collector add <url> [--title T --company C --location L]")
	}
	var m collector.Manual
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&m.Title, "title", "", "job title (links that aren't board postings)")
	fs.StringVar(&m.Company, "company", "", "company name (links that aren't board postings)")
	fs.StringVar(&m.Location, "location", "", "location, optional")
	if err := fs.Parse(args[1:]); err != nil {
		return "", collector.Manual{}, fmt.Errorf("add: %w", err)
	}
	if fs.NArg() > 0 {
		return "", collector.Manual{}, fmt.Errorf("add: one link at a time (unexpected %q)", fs.Arg(0))
	}
	return args[0], m, nil
}

// add stores a job from its link and says what happened.
func add(ctx context.Context, log *slog.Logger, dsn, leverURL string, args []string, getenv func(string) string) (int, error) {
	link, m, err := parseAddArgs(args)
	if err != nil {
		return 2, err
	}
	cfg, err := runConfig(getenv)
	if err != nil {
		return 1, err
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return 1, fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()
	reg := collector.Registry{
		"lever":      lever.New(leverURL),
		"greenhouse": greenhouse.New(greenhouse.BaseURL),
		"ashby":      ashby.New(ashby.BaseURL),
	}
	res, err := collector.Add(ctx, pool, reg, cfg.HostInterval, link, m, log)
	if err != nil {
		return 1, err
	}
	kind := "manual posting (never fetched)"
	if res.Board {
		kind = "board posting"
	}
	if res.Existed {
		fmt.Printf("already exists (id %d)\n", res.PostingID)
	} else {
		fmt.Printf("added %s, id %d\n", kind, res.PostingID)
	}
	return reconcile(ctx, log, pool)
}

// reextract re-queues the shown postings extracted with an older prompt (M3-04 AC7),
// after printing how many and the estimated cost; a dry run only prints.
func reextract(ctx context.Context, pool *pgxpool.Pool, dryRun bool) (int, error) {
	p, err := preset.Load(filepath.Join(presetsDir, "default.yaml"))
	if err != nil {
		return 1, err
	}
	// The count and estimate come from a dry run first, so they print before anything
	// changes.
	res, err := collector.Reextract(ctx, pool, p, true)
	if err != nil {
		return 1, err
	}
	cost := "unknown (no successful extraction logged yet)"
	if res.CostKnown {
		cost = fmt.Sprintf("~$%.4f", res.EstimatedUSD)
	}
	fmt.Printf("%d shown postings were extracted with a prompt older than v%d; estimated cost %s\n", res.N, res.Version, cost)
	if dryRun || res.N == 0 {
		return 0, nil
	}
	res, err = collector.Reextract(ctx, pool, p, false)
	if err != nil {
		return 1, err
	}
	fmt.Printf("re-queued %d\n", res.N)
	return 0, nil
}

// reconcile queues extraction for what the default preset shows (ADR-0001).
func reconcile(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool) (int, error) {
	p, err := preset.Load(filepath.Join(presetsDir, "default.yaml"))
	if err != nil {
		return 1, err
	}
	n, err := collector.Reconcile(ctx, pool, p)
	if err != nil {
		return 1, err
	}
	log.Info("queue reconciled", "preset", p.Name, "enqueued", n)
	return 0, nil
}
