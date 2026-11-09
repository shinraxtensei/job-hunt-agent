package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// SkipReasons are OI-16's reasons.
var SkipReasons = []string{"location", "seniority", "stack", "salary", "company", "role_type",
	"visa_relocation", "language", "not_interested", "other"}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// skip skips a lead in one click (M6-05): status `skipped`, and a skip row with the score
// as shown, for M8. Sent again while skipped, it records the reason instead. The answer
// has the previous status, for undo.
func (a *api) skip(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.Reason != "" && !slices.Contains(SkipReasons, body.Reason) {
		writeError(w, http.StatusBadRequest, "reason", "unknown reason")
		return
	}
	if len(body.Note) > 500 {
		writeError(w, http.StatusBadRequest, "note", "at most 500 characters")
		return
	}
	all, err := a.load(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	i := slices.IndexFunc(all, func(l Lead) bool { return l.ID == id })
	if i < 0 {
		writeError(w, http.StatusNotFound, "id", "no such lead")
		return
	}
	l := all[i]
	changed, err := a.status(r, id, "skipped", false, nil)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if changed {
		score, err := json.Marshal(map[string]any{"score": l.Score, "fit_level": l.FitLevel, "breakdown": l.Breakdown})
		if err != nil {
			a.fail(w, r, fmt.Errorf("encoding score: %w", err))
			return
		}
		if _, err := a.q.AddSkip(r.Context(), store.AddSkipParams{PostingID: id, Reason: text(body.Reason),
			Note: text(body.Note), Score: score}); err != nil {
			a.fail(w, r, err)
			return
		}
	} else if _, err := a.q.SetSkipReason(r.Context(), store.SetSkipReasonParams{PostingID: id,
		Reason: text(body.Reason), Note: text(body.Note)}); err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "skipped", "previous": l.Status})
}

// Skip is one skip with its lead, as M8 will read it.
type Skip struct {
	LeadID  int64           `json:"lead_id"`
	Company string          `json:"company"`
	Title   string          `json:"title"`
	Reason  *string         `json:"reason"`
	Note    *string         `json:"note"`
	Score   json.RawMessage `json:"score"`
	At      time.Time       `json:"at"`
}

// skips lists every skip, newest first (M6-05 AC3).
func (a *api) skips(w http.ResponseWriter, r *http.Request) {
	rows, err := a.q.ListSkips(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]Skip, 0, len(rows))
	for _, s := range rows {
		x := Skip{LeadID: s.PostingID, Company: s.Company, Title: s.Title, Score: raw(s.Score), At: s.At.Time.UTC()}
		if s.Reason.Valid {
			x.Reason = &s.Reason.String
		}
		if s.Note.Valid {
			x.Note = &s.Note.String
		}
		out = append(out, x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"skips": out})
}
