//go:build integration

package store_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/store/storetest"
)

// workItemsColumns reads db/contract/work_items.columns: "name type nullable" per line.
func workItemsColumns(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "db", "contract", "work_items.columns"))
	if err != nil {
		t.Fatal(err)
	}
	var cols []string
	for line := range strings.Lines(string(b)) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			cols = append(cols, line)
		}
	}
	return cols
}

// M3-02 (ADR-0001): the migrated work_items table is exactly the contract Python reads
// (agents/tests/test_queue.py checks the same file).
func TestWorkItemsContract(t *testing.T) {
	migrateUp(t)
	db := storetest.Connect(t, storetest.AppURL)
	rows, err := db.Query(t.Context(), `
		SELECT column_name || ' ' || data_type || ' ' || is_nullable
		FROM information_schema.columns WHERE table_name = 'work_items' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		got = append(got, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := workItemsColumns(t); !slices.Equal(got, want) {
		t.Errorf("work_items columns:\n got %q\nwant %q", got, want)
	}
}

// M3-02: the app role can read and write work_items, nothing more.
func TestWorkItemsGrants(t *testing.T) {
	migrateUp(t)
	db := storetest.Connect(t, storetest.SuperURL)
	for priv, want := range map[string]bool{
		"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
		"TRUNCATE": false, "REFERENCES": false, "TRIGGER": false,
	} {
		var has bool
		if err := db.QueryRow(t.Context(),
			`SELECT has_table_privilege('jobhunt_app', 'work_items', $1)`, priv).Scan(&has); err != nil {
			t.Fatal(err)
		}
		if has != want {
			t.Errorf("jobhunt_app %s on work_items = %v, want %v", priv, has, want)
		}
	}
	var owner bool
	if err := db.QueryRow(t.Context(),
		`SELECT pg_has_role('jobhunt_app', tableowner, 'USAGE') FROM pg_tables WHERE tablename = 'work_items'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner {
		t.Error("jobhunt_app owns work_items (could ALTER or DROP it)")
	}
}
