// Command collector fetches job postings and stores them.
//
//	DATABASE_URL=postgres://jobhunt_app:...@localhost:5432/jobhunt \
//	COLLECTOR_LEVER_COMPANY=spotify go run ./cmd/collector once
//
//	DATABASE_URL=... go run ./cmd/collector companies sync   # data/companies.yaml
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/source/lever"
	"github.com/shinraxtensei/job-hunter/internal/watchlist"
)

const (
	usage = "usage: collector once | collector companies sync (both read DATABASE_URL; once also COLLECTOR_LEVER_COMPANY)"

	// watchListPath is relative to the working directory: run from the repo root.
	watchListPath = "data/companies.yaml"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	company := os.Getenv("COLLECTOR_LEVER_COMPANY")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, log, os.Args[1:], os.Getenv("DATABASE_URL"), company)
	stop()
	if err != nil {
		attrs := []any{"command", strings.Join(os.Args[1:], " "), "err", err}
		if company != "" && len(os.Args) == 2 && os.Args[1] == "once" {
			attrs = append(attrs, "company", company)
		}
		log.Error("collector failed", attrs...)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger, args []string, dsn, company string) error {
	cmd := strings.Join(args, " ")
	switch cmd {
	case "once":
		if dsn == "" || company == "" {
			return errors.New("DATABASE_URL and COLLECTOR_LEVER_COMPANY must both be set")
		}
		return withConn(ctx, log, dsn, func(conn *pgx.Conn) error {
			_, err := collector.Once(ctx, conn, lever.New(lever.BaseURL), company, log)
			return err
		})
	case "companies sync":
		if dsn == "" {
			return errors.New("DATABASE_URL must be set")
		}
		// Validate the whole file before touching the database.
		entries, err := watchlist.Load(watchListPath)
		if err != nil {
			return err
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
		return errors.New(usage)
	}
}

func withConn(ctx context.Context, log *slog.Logger, dsn string, fn func(*pgx.Conn) error) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Warn("closing database", "err", err)
		}
	}()
	return fn(conn)
}
