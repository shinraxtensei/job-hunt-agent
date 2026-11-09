//go:build integration

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/shinraxtensei/job-hunt-agent/internal/store/storetest"
)

func TestMain(m *testing.M) { storetest.Main(m) }

// AC4: SIGINT or SIGTERM during a run cancels the in-flight request, writes nothing
// half-done, records the run as cancelled, and the process exits within 10 seconds.
func TestRunStopsOnSignal(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "collector")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building collector: %v\n%s", err, out)
	}
	storetest.Migrate(t, storetest.OwnerURL)
	db := storetest.Connect(t, storetest.AppURL)

	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			if _, err := db.Exec(t.Context(), `UPDATE companies SET active = false`); err != nil {
				t.Fatalf("resetting companies: %v", err)
			}
			slug := fmt.Sprintf("hang-%d", time.Now().UnixNano())
			if _, err := db.Exec(t.Context(),
				`INSERT INTO companies (name, board_type, board_slug, track) VALUES ('Hang', 'lever', $1, 'remote')`,
				slug); err != nil {
				t.Fatalf("adding company: %v", err)
			}

			started := make(chan struct{})
			var once sync.Once
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				once.Do(func() { close(started) })
				<-r.Context().Done() // hang until the client goes away
			}))
			defer srv.Close()

			var out bytes.Buffer
			cmd := exec.Command(bin, "run")
			cmd.Env = append(os.Environ(), "DATABASE_URL="+storetest.AppURL, "LEVER_BASE_URL="+srv.URL)
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Start(); err != nil {
				t.Fatalf("starting collector: %v", err)
			}
			select {
			case <-started:
			case <-time.After(30 * time.Second):
				_ = cmd.Process.Kill() // test is failing anyway
				t.Fatalf("collector never reached Lever\n%s", out.String())
			}

			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatalf("signalling: %v", err)
			}
			sent := time.Now()
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			var err error
			select {
			case err = <-exited:
			case <-time.After(10 * time.Second):
				_ = cmd.Process.Kill()
				t.Fatalf("collector still running 10s after %s\n%s", sig, out.String())
			}
			t.Logf("exited %v after %s", time.Since(sent).Round(time.Millisecond), sig)

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Errorf("exit = %v, want exit code 1 for a cancelled run\n%s", err, out.String())
			}
			var outcome string
			var ended bool
			var postings int
			if err := db.QueryRow(context.Background(),
				`SELECT coalesce(outcome, ''), ended_at IS NOT NULL,
				        (SELECT count(*) FROM postings p JOIN companies c ON c.id = p.company_id WHERE c.board_slug = $1)
				 FROM collector_runs ORDER BY id DESC LIMIT 1`, slug).Scan(&outcome, &ended, &postings); err != nil {
				t.Fatalf("reading run: %v", err)
			}
			if outcome != "cancelled" || !ended || postings != 0 {
				t.Errorf("run outcome=%q ended=%v postings=%d, want cancelled, ended, 0", outcome, ended, postings)
			}
		})
	}
}
