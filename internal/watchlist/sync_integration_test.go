//go:build integration

package watchlist_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunter/internal/store/storetest"
	"github.com/shinraxtensei/job-hunter/internal/watchlist"
)

func TestMain(m *testing.M) { storetest.Main(m) }

// conn migrates the shared database and connects as the app role.
func conn(t *testing.T) *pgx.Conn {
	t.Helper()
	storetest.Migrate(t, storetest.OwnerURL)
	return storetest.Connect(t, storetest.AppURL)
}

// slug returns a board slug unique to this test run (the board is the unique key).
func slug(t *testing.T, s string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d", s, time.Now().UnixNano())
}

type row struct {
	name, track      string
	excluded, active bool
}

func read(t *testing.T, c *pgx.Conn, boardSlug string) (row, bool) {
	t.Helper()
	var r row
	err := c.QueryRow(t.Context(),
		`SELECT name, coalesce(track, ''), excluded, active FROM companies
		 WHERE board_type = 'lever' AND board_slug = $1`, boardSlug).Scan(&r.name, &r.track, &r.excluded, &r.active)
	if errors.Is(err, pgx.ErrNoRows) {
		return row{}, false
	}
	if err != nil {
		t.Fatalf("reading company %s: %v", boardSlug, err)
	}
	return r, true
}

func entry(name, boardSlug, track string) watchlist.Entry {
	return watchlist.Entry{Name: name, BoardType: "lever", BoardSlug: boardSlug, Track: track}
}

// AC1: new rows inserted, changed rows updated, removed rows marked inactive (never
// deleted), and a row that comes back is active again.
func TestSyncInsertsUpdatesAndDeactivates(t *testing.T) {
	c := conn(t)
	x, y, z, w := slug(t, "x"), slug(t, "y"), slug(t, "z"), slug(t, "w")

	res, err := watchlist.Sync(t.Context(), c, []watchlist.Entry{
		entry("X", x, "remote"), entry("Y", y, "morocco"), entry("Z", z, "consulting"),
	})
	if err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if res.Inserted != 3 || res.Updated != 0 {
		t.Errorf("first sync = %+v, want 3 inserted, 0 updated", res)
	}

	excludedX := entry("X Renamed", x, "relocation")
	excludedX.Excluded = true
	res, err = watchlist.Sync(t.Context(), c, []watchlist.Entry{
		excludedX, entry("Y", y, "morocco"), entry("W", w, "remote"),
	})
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if res.Inserted != 1 || res.Updated != 1 || res.Unchanged != 1 || res.Deactivated < 1 {
		t.Errorf("second sync = %+v, want 1 inserted, 1 updated, 1 unchanged, >=1 deactivated", res)
	}
	if got, _ := read(t, c, x); got != (row{"X Renamed", "relocation", true, true}) {
		t.Errorf("x = %+v, want renamed, relocation, excluded, active", got)
	}
	got, ok := read(t, c, z)
	if !ok {
		t.Fatal("z was deleted; removed rows must only be marked inactive")
	}
	if got.active {
		t.Errorf("z = %+v, want inactive after removal from the file", got)
	}
	if got, _ := read(t, c, w); !got.active || got.name != "W" {
		t.Errorf("w = %+v, want inserted and active", got)
	}

	res, err = watchlist.Sync(t.Context(), c, []watchlist.Entry{entry("Z", z, "consulting")})
	if err != nil {
		t.Fatalf("third Sync: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("re-adding z = %+v, want 1 updated", res)
	}
	if got, _ := read(t, c, z); !got.active {
		t.Errorf("z = %+v, want active again once back in the file", got)
	}
}

// AC2: an invalid file fails before anything is written, valid entries included.
func TestSyncFileInvalidWritesNothing(t *testing.T) {
	c := conn(t)
	kept := slug(t, "kept")
	if _, err := watchlist.Sync(t.Context(), c, []watchlist.Entry{entry("Kept", kept, "remote")}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	fresh := slug(t, "fresh")
	path := filepath.Join(t.TempDir(), "companies.yaml")
	body := fmt.Sprintf("- { name: Fresh, board_type: lever, board_slug: %s, track: remote }\n"+
		"- { name: Broken, board_type: bamboohr, board_slug: b, track: remote }\n", fresh)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := watchlist.SyncFile(t.Context(), c, path); err == nil {
		t.Fatal("SyncFile accepted an invalid file")
	}
	if _, ok := read(t, c, fresh); ok {
		t.Error("the valid entry in an invalid file was written")
	}
	if got, _ := read(t, c, kept); !got.active {
		t.Error("an invalid file deactivated existing rows")
	}
}

// AC4: syncing the same list twice reports 0 inserted, 0 updated the second time.
func TestSyncTwiceReportsNoChanges(t *testing.T) {
	c := conn(t)
	entries := []watchlist.Entry{entry("A", slug(t, "a"), "remote"), entry("B", slug(t, "b"), "consulting")}
	if _, err := watchlist.Sync(t.Context(), c, entries); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	res, err := watchlist.Sync(t.Context(), c, entries)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if res.Inserted != 0 || res.Updated != 0 || res.Unchanged != 2 {
		t.Errorf("second sync = %+v, want 0 inserted, 0 updated, 2 unchanged", res)
	}
}
