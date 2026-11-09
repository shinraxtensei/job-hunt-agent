// Package watchlist loads the reviewed company list (data/companies.yaml) and syncs it
// into the companies table (ADR-0005).
package watchlist

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.yaml.in/yaml/v3"

	"github.com/shinraxtensei/job-hunter/internal/store"
)

// Entry is one company on the watch list.
type Entry struct {
	Name, BoardType, BoardSlug, Track string
	Excluded                          bool
	Line                              int // line in the file, for error messages
}

// Allowed values; the database CHECK constraints say the same (ADR-0005).
var (
	boardTypes = map[string]bool{"lever": true, "greenhouse": true, "ashby": true, "workable": true, "personio": true, "remotive": true}
	tracks     = map[string]bool{"morocco": true, "remote": true, "relocation": true, "consulting": true}
	fields     = map[string]bool{"name": true, "board_type": true, "board_slug": true, "track": true, "excluded": true}
)

// Load reads and validates the watch list at path.
func Load(path string) ([]Entry, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the path is the operator's own config file
	if err != nil {
		return nil, fmt.Errorf("reading watch list: %w", err)
	}
	entries, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return entries, nil
}

// Parse validates a watch list. It reports every invalid entry with its line, not
// just the first, and returns no entries unless all are valid.
func Parse(b []byte) ([]Entry, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.SequenceNode || len(doc.Content[0].Content) == 0 {
		return nil, errors.New("want a non-empty list of companies")
	}

	var (
		entries  []Entry
		problems []string
		seen     = map[string]int{} // board key -> first line
	)
	for _, n := range doc.Content[0].Content {
		e, errs := entry(n)
		if len(errs) == 0 {
			key := e.BoardType + "/" + e.BoardSlug
			if first, dup := seen[key]; dup {
				errs = append(errs, fmt.Sprintf("duplicate board %s (first on line %d)", key, first))
			} else {
				seen[key] = e.Line
			}
		}
		for _, msg := range errs {
			problems = append(problems, fmt.Sprintf("line %d: %s", n.Line, msg))
		}
		entries = append(entries, e)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%d problems:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return entries, nil
}

// entry decodes and checks one list item, returning every problem it has.
func entry(n *yaml.Node) (Entry, []string) {
	e := Entry{Line: n.Line}
	if n.Kind != yaml.MappingNode {
		return e, []string{"entry is not a mapping"}
	}
	var errs []string
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i].Value; !fields[k] {
			errs = append(errs, fmt.Sprintf("unknown field %q", k))
		}
	}
	var raw struct {
		Name      string `yaml:"name"`
		BoardType string `yaml:"board_type"`
		BoardSlug string `yaml:"board_slug"`
		Track     string `yaml:"track"`
		Excluded  bool   `yaml:"excluded"`
	}
	if err := n.Decode(&raw); err != nil {
		return e, append(errs, err.Error())
	}
	e.Name, e.BoardType, e.BoardSlug, e.Track, e.Excluded = raw.Name, raw.BoardType, raw.BoardSlug, raw.Track, raw.Excluded

	if strings.TrimSpace(e.Name) == "" {
		errs = append(errs, "missing name")
	}
	if !boardTypes[e.BoardType] {
		errs = append(errs, fmt.Sprintf("unknown board_type %q", e.BoardType))
	}
	switch {
	case e.BoardSlug == "":
		errs = append(errs, "missing board_slug")
	case strings.ContainsAny(e.BoardSlug, "/ \t\n"):
		errs = append(errs, fmt.Sprintf("board_slug %q contains '/' or whitespace", e.BoardSlug))
	}
	if !tracks[e.Track] {
		errs = append(errs, fmt.Sprintf("unknown track %q", e.Track))
	}
	return e, errs
}

// Result counts what a sync changed. Deactivated rows were active and are no longer
// in the file.
type Result struct{ Inserted, Updated, Unchanged, Deactivated int }

// SyncFile loads the watch list at path and syncs it. An invalid file fails before
// anything is written.
func SyncFile(ctx context.Context, conn *pgx.Conn, path string) (Result, error) {
	entries, err := Load(path)
	if err != nil {
		return Result{}, err
	}
	return Sync(ctx, conn, entries)
}

// Sync makes companies match entries in one transaction: new boards are inserted,
// changed ones updated (and reactivated), and active rows not in entries marked
// inactive. Nothing is deleted. Connect as the app role (ADR-0004).
func Sync(ctx context.Context, conn *pgx.Conn, entries []Entry) (Result, error) {
	var res Result
	err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		q := store.New(tx)
		keys := make([]string, 0, len(entries))
		for _, e := range entries {
			keys = append(keys, e.BoardType+"/"+e.BoardSlug)
			inserted, err := q.UpsertWatchedCompany(ctx, store.UpsertWatchedCompanyParams{
				Name:      e.Name,
				BoardType: e.BoardType,
				BoardSlug: e.BoardSlug,
				Track:     pgtype.Text{String: e.Track, Valid: true},
				Excluded:  e.Excluded,
			})
			switch {
			case errors.Is(err, pgx.ErrNoRows): // the upsert's WHERE found nothing to change
				res.Unchanged++
			case err != nil:
				return fmt.Errorf("syncing %s/%s: %w", e.BoardType, e.BoardSlug, err)
			case inserted:
				res.Inserted++
			default:
				res.Updated++
			}
		}
		n, err := q.DeactivateCompaniesNotIn(ctx, keys)
		if err != nil {
			return fmt.Errorf("deactivating removed companies: %w", err)
		}
		res.Deactivated = int(n)
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("syncing watch list: %w", err)
	}
	return res, nil
}
