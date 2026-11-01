// Command collector fetches job postings and stores them. Run from the repo root.
//
//	collector run              every active, non-excluded company, concurrently
//	collector once             one Lever board (COLLECTOR_LEVER_COMPANY)
//	collector companies sync   data/companies.yaml into the companies table
//
// Environment: DATABASE_URL (app role, required); COLLECTOR_CONCURRENCY (default 8);
// COLLECTOR_HOST_INTERVAL (minimum gap per host, default and minimum 1s);
// LEVER_BASE_URL (default https://api.lever.co; EU boards use https://api.eu.lever.co).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/source/ashby"
	"github.com/shinraxtensei/job-hunter/internal/source/greenhouse"
	"github.com/shinraxtensei/job-hunter/internal/source/lever"
	"github.com/shinraxtensei/job-hunter/internal/watchlist"
)

const (
	usage = "usage: collector run | collector once | collector companies sync"

	// watchListPath is relative to the working directory: run from the repo root.
	watchListPath = "data/companies.yaml"

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
		return exitCode(res.Outcome), nil

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
