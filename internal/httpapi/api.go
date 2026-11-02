// Package httpapi is the read-only leads API (M2-05, ADR-0012): net/http routing, JSON
// only. Posting text in responses is untrusted: it is only ever JSON-encoded.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/shinraxtensei/job-hunter/internal/store"
)

// DB is what the API reads from: the app role's pool.
type DB interface {
	store.DBTX
	Ping(ctx context.Context) error
}

type api struct {
	db         DB
	q          *store.Queries
	presetsDir string
	commit     string
	now        func() time.Time
	log        *slog.Logger
}

// New returns the API's handler. Presets are read from presetsDir on every request, so
// editing one needs no restart.
func New(db DB, presetsDir, commit string, now func() time.Time, log *slog.Logger) http.Handler {
	a := &api{db: db, q: store.New(db), presetsDir: presetsDir, commit: commit, now: now, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/leads", a.leads)
	mux.HandleFunc("GET /api/leads/hidden", a.hidden)
	mux.HandleFunc("GET /api/leads/{id}/members", a.members)
	mux.HandleFunc("GET /api/facets", a.facets)
	mux.HandleFunc("GET /api/presets", a.presets)
	mux.HandleFunc("GET /api/companies", a.companies)
	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "", "no such endpoint")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

// paramError is a bad request naming the parameter at fault.
type paramError struct{ param, msg string }

func (e *paramError) Error() string { return e.param + ": " + e.msg }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the client went away; nothing to do
}

func writeError(w http.ResponseWriter, status int, param, msg string) {
	type body struct {
		Parameter string `json:"parameter,omitempty"`
		Message   string `json:"message"`
	}
	writeJSON(w, status, map[string]body{"error": {param, msg}})
}

// fail answers a parameter error with 400 and anything else with 500, logging only the
// error (never posting text).
func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	if pe := (*paramError)(nil); errors.As(err, &pe) {
		writeError(w, http.StatusBadRequest, pe.param, pe.msg)
		return
	}
	a.log.Error("request failed", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "", "internal error")
}

func (a *api) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.Ping(ctx); err != nil {
		a.log.Error("database unreachable", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database unreachable", "commit": a.commit})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "commit": a.commit})
}
