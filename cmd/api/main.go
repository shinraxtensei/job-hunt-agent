// Command api serves the read-only leads API (M2-05, ADR-0012) and the tracker page
// embedded from web/ (M2-06, ADR-0013).
//
// Environment:
//
//	DATABASE_URL   app-role connection string (required)
//	API_ADDR       listen address (default 127.0.0.1:8080; anything else logs a warning)
//	PRESETS_DIR    filter presets (default data/presets)
//	API_PUBLISHED_ON_LOOPBACK  "true" when a container listens on all interfaces but its
//	               port is published on 127.0.0.1 only (deploy/compose.yaml)
//	API_PUBLIC_PORT  the port the browser uses when it differs from API_ADDR's (Compose
//	               publishes the container's 8080 on JOBHUNT_HTTP_PORT). Only Host
//	               127.0.0.1, localhost or [::1] on these ports is served (ADR-0022).
//
// `api healthcheck` exits 0 when the running API answers /healthz (Compose's healthcheck:
// the image has no shell or curl).
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shinraxtensei/job-hunt-agent/internal/httpapi"
	"github.com/shinraxtensei/job-hunt-agent/internal/webui"
)

// shutdownTimeout is how long in-flight requests get after a signal.
const shutdownTimeout = 10 * time.Second

// buildCommit is set at build time (-ldflags -X main.buildCommit=…) where .git is absent,
// as in the image.
var buildCommit string

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := probe(ctx, healthcheckURL(cmp.Or(os.Getenv("API_ADDR"), "127.0.0.1:8080")))
		cancel()
		if err != nil {
			fmt.Fprintln(os.Stderr, "unhealthy:", err)
			os.Exit(1)
		}
		return
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, log, os.Getenv)
	stop()
	if err != nil {
		log.Error("api failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger, getenv func(string) string) error {
	dsn := getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL must be set")
	}
	addr := cmp.Or(getenv("API_ADDR"), "127.0.0.1:8080")
	switch {
	case loopback(addr):
	case getenv("API_PUBLISHED_ON_LOOPBACK") == "true":
		log.Info("listening on all interfaces in a container; its port is published on 127.0.0.1 only", "addr", addr)
	default:
		log.Warn("API_ADDR is not loopback: the API has no auth (ADR-0002)", "addr", addr)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	api := httpapi.New(pool, cmp.Or(getenv("PRESETS_DIR"), "data/presets"), commit(), time.Now, log)
	srv := newServer(routes(api, webui.Handler(webui.Dist()), allowedPorts(ln.Addr().String(), getenv("API_PUBLIC_PORT"))))
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	log.Info("listening", "addr", ln.Addr().String(), "commit", commit())

	select {
	case err := <-errc:
		return fmt.Errorf("serving: %w", err)
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout) // ctx is already cancelled
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}
	log.Info("stopped")
	return nil
}

// routes sends the API's paths to the API and everything else to the tracker page, for
// loopback Host names on ports only (DNS rebinding, ADR-0022).
func routes(api, page http.Handler, ports []string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/healthz", api)
	mux.Handle("/", page)
	return httpapi.HostAllowlist(ports, mux)
}

// allowedPorts are the listen address's port and, when set and different, the port the
// browser uses.
func allowedPorts(addr, public string) []string {
	var out []string
	if _, p, err := net.SplitHostPort(addr); err == nil {
		out = append(out, p)
	}
	if public != "" && !slices.Contains(out, public) {
		out = append(out, public)
	}
	return out
}

func newServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// loopback reports whether addr binds only to the loopback interface.
func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// healthcheckURL is where the API listening on addr answers /healthz on this machine. It is
// always loopback: only the port is taken from addr, and only if it is a valid number.
func healthcheckURL(addr string) string {
	port := 8080
	if _, p, err := net.SplitHostPort(addr); err == nil {
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
			port = n
		}
	}
	return "http://127.0.0.1:" + strconv.Itoa(port) + "/healthz"
}

// probe returns nil when GET u answers 200.
func probe(ctx context.Context, u string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil) //nolint:gosec // G704: u is healthcheckURL's loopback URL
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: as above, loopback only
	if err != nil {
		return fmt.Errorf("calling %s: %w", u, err)
	}
	_ = resp.Body.Close() // only the status matters
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", u, resp.Status)
	}
	return nil
}

// commit is the revision the binary was built from: the build flag, else the VCS stamp,
// else "unknown" (go run).
func commit() string {
	if buildCommit != "" && buildCommit != "unknown" {
		return buildCommit
	}
	rev, dirty := "unknown", false
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}
