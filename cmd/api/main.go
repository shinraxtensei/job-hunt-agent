// Command api serves the read-only leads API (M2-05, ADR-0012) and the tracker page
// embedded from web/ (M2-06, ADR-0013).
//
// Environment:
//
//	DATABASE_URL   app-role connection string (required)
//	API_ADDR       listen address (default 127.0.0.1:8080; anything else logs a warning)
//	PRESETS_DIR    filter presets (default data/presets)
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
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shinraxtensei/job-hunter/internal/httpapi"
	"github.com/shinraxtensei/job-hunter/internal/webui"
)

// shutdownTimeout is how long in-flight requests get after a signal.
const shutdownTimeout = 10 * time.Second

func main() {
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
	if !loopback(addr) {
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
	srv := newServer(routes(api, webui.Handler(webui.Dist())))
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

// routes sends the API's paths to the API and everything else to the tracker page.
func routes(api, page http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/healthz", api)
	mux.Handle("/", page)
	return mux
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

// commit is the VCS revision the binary was built from, or "unknown" (go run).
func commit() string {
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
