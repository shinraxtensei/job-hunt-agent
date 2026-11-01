// Package tooling holds tests for the repo's build tooling itself: the Makefile
// targets and the lint configuration that define what "green" means.
package tooling

import (
	"os"
	"os/exec"
	"path/filepath"
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
		"check", "test-go", "test-integration", "test-py", "evals",
		"lint", "sqlc", "migration", "vuln", "up", "down", "help",
	} {
		if !strings.Contains(string(out), target) {
			t.Errorf("make help does not list target %q\n%s", target, out)
		}
	}
}

// AC4: targets that are not built yet say so and exit 1, instead of failing silently.
func TestUnimplementedTargetsExitNonZero(t *testing.T) {
	for _, target := range []string{"test-integration", "test-py", "evals", "sqlc", "migration", "up", "down"} {
		t.Run(target, func(t *testing.T) {
			out, err := exec.Command("make", "-C", repoRoot(t), target).CombinedOutput()
			if err == nil {
				t.Fatalf("make %s exited 0, want non-zero\n%s", target, out)
			}
			if !strings.Contains(string(out), "not implemented yet") {
				t.Errorf("make %s does not say it is not implemented yet\n%s", target, out)
			}
		})
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
