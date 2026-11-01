//go:build integration

package store_test

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunter/internal/store"
	"github.com/shinraxtensei/job-hunter/internal/store/storetest"
)

func TestMain(m *testing.M) { storetest.Main(m) }

// migrateUp applies all migrations to the shared database as the owner role.
func migrateUp(t *testing.T) []int64 {
	t.Helper()
	return storetest.Migrate(t, storetest.OwnerURL)
}

// freshDatabase creates an empty database owned by jobhunt_owner, dropped after the
// test, and returns the owner's URL for it.
func freshDatabase(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("fresh_%d", time.Now().UnixNano())
	super := storetest.Connect(t, storetest.SuperURL)
	if _, err := super.Exec(t.Context(), "CREATE DATABASE "+name+" OWNER jobhunt_owner"); err != nil {
		t.Fatalf("creating database %s: %v", name, err)
	}
	t.Cleanup(func() {
		// FORCE closes the test's own connections to it first.
		if _, err := super.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("dropping database %s: %v", name, err)
		}
	})
	u, err := url.Parse(storetest.OwnerURL)
	if err != nil {
		t.Fatalf("parsing owner URL: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

// AC2: on an empty database every migration applies in order; a second run is a no-op.
// Uses its own fresh database, so it holds in any test order.
func TestMigrationsApplyInOrderThenNoOp(t *testing.T) {
	files, err := filepath.Glob("../../db/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("listing migration files: %v (found %d)", err, len(files))
	}
	dsn := freshDatabase(t)
	first := storetest.Migrate(t, dsn)
	if len(first) != len(files) {
		t.Fatalf("first run applied %d migrations %v, want all %d", len(first), first, len(files))
	}
	if !slices.IsSorted(first) {
		t.Errorf("migrations applied out of order: %v", first)
	}
	if again := storetest.Migrate(t, dsn); len(again) != 0 {
		t.Errorf("second run applied %v, want none", again)
	}
}

// AC3: the first migration creates companies and postings with the listed columns.
func TestFirstMigrationCreatesTables(t *testing.T) {
	migrateUp(t)
	conn := storetest.Connect(t, storetest.OwnerURL)
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
		// The first migration's columns come first, in order; later migrations may add
		// more after them (M1-01 adds companies.track and companies.active).
		if len(got) < len(cols) || !slices.Equal(got[:len(cols)], cols) {
			t.Errorf("%s columns = %v, want them to start with %v", table, got, cols)
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
	conn := storetest.Connect(t, storetest.AppURL)
	q := store.New(conn)
	co, err := q.CreateCompany(t.Context(), store.CreateCompanyParams{
		Name: "Unique Co", BoardType: "lever", BoardSlug: fmt.Sprintf("unique-co-%d", time.Now().UnixNano()),
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
	if code := storetest.PgCode(err); code != "23505" {
		t.Errorf("duplicate (source, source_posting_id): err = %v (code %q), want unique_violation 23505", err, code)
	}
}

// AC5: the store inserts and reads a company, connected as the app role.
func TestStoreCreatesAndReadsCompany(t *testing.T) {
	migrateUp(t)
	q := store.New(storetest.Connect(t, storetest.AppURL))
	acme := fmt.Sprintf("acme-%d", time.Now().UnixNano()) // boards are unique; -count > 1 reruns
	created, err := q.CreateCompany(t.Context(), store.CreateCompanyParams{
		Name: "Acme", BoardType: "lever", BoardSlug: acme,
	})
	if err != nil {
		t.Fatalf("creating company: %v", err)
	}
	got, err := q.GetCompany(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("reading company: %v", err)
	}
	if got.Name != "Acme" || got.BoardType != "lever" || got.BoardSlug != acme || got.Excluded {
		t.Errorf("read back %+v, want Acme/lever/acme, not excluded", got)
	}
	if !got.CreatedAt.Valid {
		t.Error("created_at was not set")
	}
}

// Security note: the app role is not a superuser and cannot change the schema.
func TestAppRoleIsLeastPrivilege(t *testing.T) {
	migrateUp(t)
	conn := storetest.Connect(t, storetest.AppURL)
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
		if code := storetest.PgCode(err); code != "42501" {
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
	if code := storetest.PgCode(err); code != "42501" {
		t.Errorf("DELETE FROM goose_db_version as app role: err = %v (code %q), want insufficient_privilege 42501", err, code)
	}
}

// Security note: the owner role used by migrations is not a superuser either.
func TestOwnerRoleIsNotSuperuser(t *testing.T) {
	var super bool
	if err := storetest.Connect(t, storetest.OwnerURL).QueryRow(t.Context(),
		`SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&super); err != nil {
		t.Fatalf("reading owner role: %v", err)
	}
	if super {
		t.Error("owner role is a superuser")
	}
}

// M1-01 AC5: one row per board. A second company on the same board is rejected.
func TestCompaniesBoardIsUnique(t *testing.T) {
	migrateUp(t)
	q := store.New(storetest.Connect(t, storetest.AppURL))
	params := store.CreateCompanyParams{
		Name: "First", BoardType: "lever", BoardSlug: fmt.Sprintf("dup-board-%d", time.Now().UnixNano()),
	}
	if _, err := q.CreateCompany(t.Context(), params); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	params.Name = "Second"
	_, err := q.CreateCompany(t.Context(), params)
	if code := storetest.PgCode(err); code != "23505" {
		t.Errorf("same (board_type, board_slug) twice: err = %v (code %q), want unique_violation 23505", err, code)
	}
}
