// Command migrate applies the embedded SQL migrations. Forward-only: there is no down.
//
//	MIGRATIONS_DATABASE_URL=postgres://jobhunt_owner:...@localhost:5432/jobhunt go run ./cmd/migrate up
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"github.com/shinraxtensei/job-hunt-agent/db/migrations"
)

const usage = "usage: migrate up|status (connects with MIGRATIONS_DATABASE_URL, the owner role)"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Args[1:], os.Getenv("MIGRATIONS_DATABASE_URL"))
	stop()
	if err != nil {
		slog.Error("migrate failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, dsn string) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "status") {
		return errors.New(usage)
	}
	if dsn == "" {
		return errors.New("MIGRATIONS_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Warn("closing database", "err", err)
		}
	}()

	if args[0] == "status" {
		statuses, err := migrations.Status(ctx, db)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			slog.Info("migration", "version", s.Source.Version, "file", s.Source.Path, "state", s.State)
		}
		return nil
	}

	applied, err := migrations.Up(ctx, db)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		slog.Info("no pending migrations")
	}
	for _, v := range applied {
		slog.Info("applied migration", "version", v)
	}
	return nil
}
