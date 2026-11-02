//go:build integration

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/shinraxtensei/job-hunter/internal/store/storetest"
)

func TestMain(m *testing.M) { storetest.Main(m) }

// proc is a running API binary and everything it has logged.
type proc struct {
	cmd  *exec.Cmd
	addr string
	mu   sync.Mutex
	logs strings.Builder
	eof  chan struct{} // closed once stdout is fully read (the process has exited)
}

func (p *proc) output() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.logs.String()
}

// stop sends sig and waits for the process to exit, reading all of its output first:
// Wait closes the pipe, so calling it earlier could lose the last lines.
func (p *proc) stop(t *testing.T, sig syscall.Signal) error {
	t.Helper()
	if err := p.cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.eof:
	case <-time.After(15 * time.Second):
		t.Fatalf("api did not stop\n%s", p.output())
	}
	return p.cmd.Wait()
}

// start builds and runs the API on an ephemeral port and waits for the address it logs.
func start(t *testing.T, bindHost string) *proc {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "api")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building api: %v\n%s", err, out)
	}
	storetest.Migrate(t, storetest.OwnerURL)
	p := &proc{cmd: exec.Command(bin), eof: make(chan struct{})}
	p.cmd.Env = append(os.Environ(), "DATABASE_URL="+storetest.AppURL, "API_ADDR="+bindHost+":0",
		"PRESETS_DIR=../../data/presets")
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	p.cmd.Stderr = p.cmd.Stdout
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.cmd.Process.Kill() }) // already exited when the test passes
	addr := make(chan string, 1)
	go func() {
		defer close(p.eof)
		lines := bufio.NewScanner(stdout)
		for lines.Scan() {
			p.mu.Lock()
			p.logs.WriteString(lines.Text() + "\n")
			p.mu.Unlock()
			var l struct{ Msg, Addr string }
			if json.Unmarshal(lines.Bytes(), &l) == nil && l.Msg == "listening" {
				addr <- l.Addr
			}
		}
	}()
	select {
	case p.addr = <-addr:
		return p
	case <-time.After(30 * time.Second):
		t.Fatalf("api never listened\n%s", p.output())
		return nil
	}
}

// AC4 + AC5: the API serves /healthz on loopback by default and shuts down cleanly on
// SIGINT and SIGTERM.
func TestServesAndStopsOnSignal(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			p := start(t, "127.0.0.1")
			resp, err := http.Get("http://" + p.addr + "/healthz")
			if err != nil || resp.StatusCode != http.StatusOK {
				t.Fatalf("healthz: %v %v", resp, err)
			}
			_ = resp.Body.Close()
			var exit *exec.ExitError
			if err := p.stop(t, sig); errors.As(err, &exit) {
				t.Fatalf("exit code %d\n%s", exit.ExitCode(), p.output())
			}
			if !strings.Contains(p.output(), `"msg":"stopped"`) {
				t.Errorf("no clean stop logged:\n%s", p.output())
			}
			if strings.Contains(p.output(), "not loopback") {
				t.Errorf("warned on a loopback bind:\n%s", p.output())
			}
		})
	}
}

// Security notes: binding to anything but loopback logs a warning.
func TestWarnsOnNonLoopbackBind(t *testing.T) {
	p := start(t, "0.0.0.0")
	_ = p.stop(t, syscall.SIGTERM) // exit status is checked in the signal test
	if !strings.Contains(p.output(), "not loopback") {
		t.Errorf("no warning for 0.0.0.0:\n%s", p.output())
	}
}
