//go:build integration

// Package storetest starts a throwaway Postgres for integration tests: the image
// pinned in deploy/compose.yaml, with the same role init script, so tests see what
// prod sees. Use it from TestMain: func TestMain(m *testing.M) { storetest.Main(m) }.
package storetest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/shinraxtensei/job-hunter/db/migrations"
)

// URLs for the shared test database, one per role. Set by Main before tests run.
var (
	SuperURL string // bootstrap superuser
	OwnerURL string // jobhunt_owner: runs migrations
	AppURL   string // jobhunt_app: what the code under test uses
)

// Main starts the database, runs the package's tests and exits with their status.
func Main(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	ctx := context.Background()
	root, err := repoRoot()
	if err != nil {
		return 0, err
	}
	image, err := composeImage(filepath.Join(root, "deploy", "compose.yaml"))
	if err != nil {
		return 0, err
	}
	ctr, err := postgres.Run(ctx, image,
		postgres.WithDatabase("jobhunt"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("test-superuser"),
		postgres.WithInitScripts(filepath.Join(root, "deploy", "postgres", "initdb", "01-roles.sh")),
		testcontainers.WithEnv(map[string]string{
			"JOBHUNT_OWNER_PASSWORD": "test-owner",
			"JOBHUNT_APP_PASSWORD":   "test-app",
		}),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return 0, fmt.Errorf("starting postgres: %w", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			fmt.Fprintln(os.Stderr, "terminating postgres:", err)
		}
	}()
	SuperURL, err = ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 0, fmt.Errorf("connection string: %w", err)
	}
	if OwnerURL, err = withUser(SuperURL, "jobhunt_owner", "test-owner"); err != nil {
		return 0, err
	}
	if AppURL, err = withUser(SuperURL, "jobhunt_app", "test-app"); err != nil {
		return 0, err
	}
	return m.Run(), nil
}

// repoRoot is three directories up from this file (internal/store/storetest).
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locating storetest source file")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// composeImage returns the postgres image reference pinned in compose.yaml, so the
// tests and the local stack can never drift apart.
func composeImage(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is built from the repo root, not from input
	if err != nil {
		return "", fmt.Errorf("reading compose file: %w", err)
	}
	m := regexp.MustCompile(`(?m)^\s*image:\s*(postgres:\S+)`).FindSubmatch(b)
	if m == nil {
		return "", fmt.Errorf("no postgres image in %s", path)
	}
	return string(m[1]), nil
}

func withUser(rawURL, user, password string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing connection string: %w", err)
	}
	u.User = url.UserPassword(user, password)
	return u.String(), nil
}

// Migrate applies all migrations to dsn, the way cmd/migrate does, and returns the
// versions it applied.
func Migrate(t *testing.T, dsn string) []int64 {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening owner connection: %v", err)
	}
	// Closed before returning, not in t.Cleanup: goleak checks run before cleanups.
	defer func() { _ = db.Close() }() // close error on a test connection changes nothing
	applied, err := migrations.Up(t.Context(), db)
	if err != nil {
		t.Fatalf("migrations up: %v", err)
	}
	return applied
}

// Connect opens a connection closed when the test ends.
func Connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) }) // see Migrate
	return conn
}

// PgCode returns err's Postgres SQLSTATE, or "" if it has none.
func PgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
