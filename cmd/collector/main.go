// Command collector fetches job postings and stores them.
//
//	DATABASE_URL=postgres://jobhunt_app:...@localhost:5432/jobhunt \
//	COLLECTOR_LEVER_COMPANY=spotify go run ./cmd/collector once
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunter/internal/collector"
	"github.com/shinraxtensei/job-hunter/internal/source/lever"
)

const usage = "usage: collector once (reads DATABASE_URL and COLLECTOR_LEVER_COMPANY)"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	company := os.Getenv("COLLECTOR_LEVER_COMPANY")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, log, os.Args[1:], os.Getenv("DATABASE_URL"), company)
	stop()
	if err != nil {
		log.Error("collector failed", "company", company, "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger, args []string, dsn, company string) error {
	if len(args) != 1 || args[0] != "once" {
		return errors.New(usage)
	}
	if dsn == "" || company == "" {
		return errors.New("DATABASE_URL and COLLECTOR_LEVER_COMPANY must both be set")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Warn("closing database", "err", err)
		}
	}()
	_, err = collector.Once(ctx, conn, lever.New(lever.BaseURL), company, log)
	return err
}
