// Package migrations embeds the forward-only SQL migrations and applies them with goose.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// Up applies every pending migration in version order and returns the versions it
// applied. Run it as the database owner role, never as the app role.
func Up(ctx context.Context, db *sql.DB) ([]int64, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	results, err := p.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("applying migrations: %w", err)
	}
	versions := make([]int64, 0, len(results))
	for _, r := range results {
		versions = append(versions, r.Source.Version)
	}
	return versions, nil
}

// Status reports every known migration and whether it has been applied.
func Status(ctx context.Context, db *sql.DB) ([]*goose.MigrationStatus, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	s, err := p.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading migration status: %w", err)
	}
	return s, nil
}

func provider(db *sql.DB) (*goose.Provider, error) {
	p, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return nil, fmt.Errorf("loading migrations: %w", err)
	}
	return p, nil
}
