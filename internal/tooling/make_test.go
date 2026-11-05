// Package tooling holds tests for the repo's build tooling itself: the Makefile
// targets and the lint configuration that define what "green" means.
package tooling

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot is the module root, two levels up from internal/tooling.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	return root
}

// AC4: `make help` lists every target in the CLAUDE.md Commands table.
func TestMakeHelpListsEveryCommand(t *testing.T) {
	out, err := exec.Command("make", "-C", repoRoot(t), "help").CombinedOutput()
	if err != nil {
		t.Fatalf("make help: %v\n%s", err, out)
	}
	for _, target := range []string{
		"check", "test-go", "test-integration", "test-py", "evals", "evals-replay",
		"lint", "sqlc", "sqlc-check", "migration", "vuln", "up", "down", "fuzz", "help",
	} {
		if !strings.Contains(string(out), target) {
			t.Errorf("make help does not list target %q\n%s", target, out)
		}
	}
}

// writeBadModule writes a throwaway module containing one file that is both
// badly formatted and ignores an error return.
func writeBadModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	write("go.mod", "module bad\n\ngo 1.25\n")
	write("bad.go", "package bad\n\nimport \"os\"\n\nfunc Bad( ) {\n os.Remove(\"/tmp/nope\")\n}\n")
	return dir
}

// AC2: the real fmt-check target fails on a formatting error and names the file.
func TestFmtCheckCatchesFormattingError(t *testing.T) {
	dir := writeBadModule(t)
	out, err := exec.Command("make", "-C", repoRoot(t), "fmt-check", "GO_DIRS="+dir).CombinedOutput()
	if err == nil {
		t.Fatalf("make fmt-check exited 0 on an unformatted file\n%s", out)
	}
	if !strings.Contains(string(out), "bad.go") {
		t.Errorf("make fmt-check did not name the unformatted file\n%s", out)
	}
}

// golangciLintBinary builds the version pinned in tools.mod and returns its path, so
// the fixture module below can be linted from outside this module.
func golangciLintBinary(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "tool", "-modfile=tools.mod", "-n", "golangci-lint")
	cmd.Dir = repoRoot(t)
	// golangci-lint needs no cgo, and building it without one avoids depending on a
	// working C toolchain just to lint.
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("locating golangci-lint: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// AC2: an unchecked error return is caught and the file and rule are named.
func TestLintCatchesUncheckedError(t *testing.T) {
	dir := writeBadModule(t)
	cmd := exec.Command(golangciLintBinary(t), "run",
		"--config", filepath.Join(repoRoot(t), ".golangci.yml"), "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("golangci-lint exited 0 on a file with an unchecked error\n%s", out)
	}
	// Either errcheck or gosec's G104 may claim it first; both name the rule.
	named := strings.Contains(string(out), "errcheck") || strings.Contains(string(out), "G104")
	if !strings.Contains(string(out), "bad.go") || !named {
		t.Errorf("golangci-lint output does not name the file and the rule\n%s", out)
	}
}

// M0-03 AC4: `make migration name=add_x` creates one timestamped goose SQL file.
func TestMakeMigrationCreatesTimestampedFile(t *testing.T) {
	dir := t.TempDir()
	out, err := exec.Command("make", "-C", repoRoot(t), "migration", "name=add_x", "MIGRATIONS_DIR="+dir).CombinedOutput()
	if err != nil {
		t.Fatalf("make migration: %v\n%s", err, out)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	if len(entries) != 1 || !regexp.MustCompile(`^\d{14}_add_x\.sql$`).MatchString(entries[0].Name()) {
		t.Fatalf("want one file named <YYYYMMDDHHMMSS>_add_x.sql, got %v", entries)
	}
	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading migration: %v", err)
	}
	if !strings.Contains(string(body), "-- +goose Up") {
		t.Errorf("migration has no goose Up annotation:\n%s", body)
	}
}

// M0-03 AC4: a missing or non-snake_case name is rejected and creates nothing.
func TestMakeMigrationRejectsBadName(t *testing.T) {
	for _, name := range []string{"", "Add-X", "../x"} {
		dir := t.TempDir()
		out, err := exec.Command("make", "-C", repoRoot(t), "migration", "name="+name, "MIGRATIONS_DIR="+dir).CombinedOutput()
		if err == nil {
			t.Errorf("make migration name=%q exited 0\n%s", name, out)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("make migration name=%q created %v", name, entries)
		}
	}
}
