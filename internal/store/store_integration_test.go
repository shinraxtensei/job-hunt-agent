//go:build integration

package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/shinraxtensei/job-hunter/db/migrations"
	"github.com/shinraxtensei/job-hunter/internal/store"
)

// URLs for the shared test database, set by TestMain.
var superURL, ownerURL, appURL string

// TestMain starts one throwaway Postgres for the package, from the image pinned in
// deploy/compose.yaml and with the same role init script, so tests see what prod sees.
func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	ctx := context.Background()
	root, err := filepath.Abs("../..")
	if err != nil {
		return 0, fmt.Errorf("resolving repo root: %w", err)
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
	superURL, err = ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 0, fmt.Errorf("connection string: %w", err)
	}
	if ownerURL, err = withUser(superURL, "jobhunt_owner", "test-owner"); err != nil {
		return 0, err
	}
	if appURL, err = withUser(superURL, "jobhunt_app", "test-app"); err != nil {
		return 0, err
	}
	return m.Run(), nil
}

// composeImage returns the postgres image reference pinned in compose.yaml, so the
// tests and the local stack can never drift apart.
func composeImage(path string) (string, error) {
	b, err := os.ReadFile(path)
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

// migrateUp applies all migrations to the shared database as the owner role.
func migrateUp(t *testing.T) []int64 {
	t.Helper()
	return migrateURL(t, ownerURL)
}

// migrateURL applies all migrations to dsn, the way cmd/migrate does.
func migrateURL(t *testing.T, dsn string) []int64 {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening owner connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() }) // close error on a test connection changes nothing
	applied, err := migrations.Up(t.Context(), db)
	if err != nil {
		t.Fatalf("migrations up: %v", err)
	}
	return applied
}

func connect(t *testing.T, rawURL string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), rawURL)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) }) // see migrateUp
	return conn
}

// freshDatabase creates an empty database owned by jobhunt_owner, dropped after the
// test, and returns the owner's URL for it.
func freshDatabase(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("fresh_%d", time.Now().UnixNano())
	super := connect(t, superURL)
	if _, err := super.Exec(t.Context(), "CREATE DATABASE "+name+" OWNER jobhunt_owner"); err != nil {
		t.Fatalf("creating database %s: %v", name, err)
	}
	t.Cleanup(func() {
		// FORCE closes the test's own connections to it first.
		if _, err := super.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("dropping database %s: %v", name, err)
		}
	})
	u, err := url.Parse(ownerURL)
	if err != nil {
		t.Fatalf("parsing owner URL: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// AC2: on an empty database every migration applies in order; a second run is a no-op.
// Uses its own fresh database, so it holds in any test order.
func TestMigrationsApplyInOrderThenNoOp(t *testing.T) {
	files, err := filepath.Glob("../../db/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("listing migration files: %v (found %d)", err, len(files))
	}
	dsn := freshDatabase(t)
	first := migrateURL(t, dsn)
	if len(first) != len(files) {
		t.Fatalf("first run applied %d migrations %v, want all %d", len(first), first, len(files))
	}
	if !slices.IsSorted(first) {
		t.Errorf("migrations applied out of order: %v", first)
	}
	if again := migrateURL(t, dsn); len(again) != 0 {
		t.Errorf("second run applied %v, want none", again)
	}
}

// AC3: the first migration creates companies and postings with the listed columns.
func TestFirstMigrationCreatesTables(t *testing.T) {
	migrateUp(t)
	conn := connect(t, ownerURL)
	want := map[string][]string{
		"companies": {"id", "name", "board_type", "board_slug", "excluded", "created_at"},
		"postings": {"id", "company_id", "source", "source_posting_id", "url", "title",
			"raw_payload", "first_seen_at", "last_checked_at"},
	}
	for table, cols := range want {
		rows, err := conn.Query(t.Context(),
			`SELECT column_name FROM information_schema.columns
			 WHERE table_schema = 'public' AND table_name = $1 ORDER BY ordinal_position`, table)
		if err != nil {
			t.Fatalf("listing %s columns: %v", table, err)
		}
		got, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatalf("reading %s columns: %v", table, err)
		}
		if !slices.Equal(got, cols) {
			t.Errorf("%s columns = %v, want %v", table, got, cols)
		}
	}
	var payloadType string
	if err := conn.QueryRow(t.Context(),
		`SELECT data_type FROM information_schema.columns
		 WHERE table_name = 'postings' AND column_name = 'raw_payload'`).Scan(&payloadType); err != nil {
		t.Fatalf("reading raw_payload type: %v", err)
	}
	if payloadType != "jsonb" {
		t.Errorf("postings.raw_payload is %s, want jsonb", payloadType)
	}
}

// AC3: (source, source_posting_id) is unique.
func TestPostingsSourceIDIsUnique(t *testing.T) {
	migrateUp(t)
	conn := connect(t, appURL)
	q := store.New(conn)
	co, err := q.CreateCompany(t.Context(), store.CreateCompanyParams{
		Name: "Unique Co", BoardType: "lever", BoardSlug: "unique-co",
	})
	if err != nil {
		t.Fatalf("creating company: %v", err)
	}
	// Unique per run, so the test also passes with -count > 1 on the shared database.
	postingID := fmt.Sprintf("dup-%d", time.Now().UnixNano())
	insert := `INSERT INTO postings (company_id, source, source_posting_id, url, title, raw_payload)
	           VALUES ($1, 'lever', $2, 'https://example.test/1', 'Engineer', '{}')`
	if _, err := conn.Exec(t.Context(), insert, co.ID, postingID); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err = conn.Exec(t.Context(), insert, co.ID, postingID)
	if code := pgCode(err); code != "23505" {
		t.Errorf("duplicate (source, source_posting_id): err = %v (code %q), want unique_violation 23505", err, code)
	}
}

// AC5: the store inserts and reads a company, connected as the app role.
func TestStoreCreatesAndReadsCompany(t *testing.T) {
	migrateUp(t)
	q := store.New(connect(t, appURL))
	created, err := q.CreateCompany(t.Context(), store.CreateCompanyParams{
		Name: "Acme", BoardType: "lever", BoardSlug: "acme",
	})
	if err != nil {
		t.Fatalf("creating company: %v", err)
	}
	got, err := q.GetCompany(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("reading company: %v", err)
	}
	if got.Name != "Acme" || got.BoardType != "lever" || got.BoardSlug != "acme" || got.Excluded {
		t.Errorf("read back %+v, want Acme/lever/acme, not excluded", got)
	}
	if !got.CreatedAt.Valid {
		t.Error("created_at was not set")
	}
}

// Security note: the app role is not a superuser and cannot change the schema.
func TestAppRoleIsLeastPrivilege(t *testing.T) {
	migrateUp(t)
	conn := connect(t, appURL)
	var super, createDB, createRole bool
	if err := conn.QueryRow(t.Context(),
		`SELECT rolsuper, rolcreatedb, rolcreaterole FROM pg_roles WHERE rolname = current_user`,
	).Scan(&super, &createDB, &createRole); err != nil {
		t.Fatalf("reading app role: %v", err)
	}
	if super || createDB || createRole {
		t.Errorf("app role: superuser=%v createdb=%v createrole=%v, want all false", super, createDB, createRole)
	}
	for _, stmt := range []string{
		`CREATE TABLE app_should_not_create (id int)`,
		`DROP TABLE companies`,
		`ALTER TABLE postings ADD COLUMN x int`,
	} {
		_, err := conn.Exec(t.Context(), stmt)
		if code := pgCode(err); code != "42501" {
			t.Errorf("%s as app role: err = %v (code %q), want insufficient_privilege 42501", stmt, err, code)
		}
	}

	// Migration history is read-only to the app. Inside a rolled-back transaction, so
	// a regression can't wipe the history and break the other tests.
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatalf("beginning transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }() // rollback is the point; nothing to keep
	_, err = tx.Exec(t.Context(), `DELETE FROM goose_db_version`)
	if code := pgCode(err); code != "42501" {
		t.Errorf("DELETE FROM goose_db_version as app role: err = %v (code %q), want insufficient_privilege 42501", err, code)
	}
}

// Security note: the owner role used by migrations is not a superuser either.
func TestOwnerRoleIsNotSuperuser(t *testing.T) {
	var super bool
	if err := connect(t, ownerURL).QueryRow(t.Context(),
		`SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&super); err != nil {
		t.Fatalf("reading owner role: %v", err)
	}
	if super {
		t.Error("owner role is a superuser")
	}
}
