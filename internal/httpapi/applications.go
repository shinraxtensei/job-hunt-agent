package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunter/internal/store"
)

// Statuses are the PRD's tracker statuses, in their usual order (M6-01).
var Statuses = []string{"new", "shortlisted", "drafted", "applied", "replied", "interviewing",
	"offer", "rejected", "closed", "skipped"}

// readJSON decodes a small JSON body (a few fields typed by the owner); unknown fields are
// refused.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "", "want a JSON body: "+err.Error())
		return false
	}
	return true
}

// setStatus records the owner's status for a lead (M6-01): the application is created on
// the first change, and every change is kept in its history. Any status may follow any.
func (a *api) setStatus(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !slices.Contains(Statuses, body.Status) {
		writeError(w, http.StatusBadRequest, "status", "unknown status")
		return
	}
	changed, err := a.status(r, id, body.Status, false, nil)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "id", "no such lead")
	case err != nil:
		a.fail(w, r, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"status": body.Status, "changed": changed})
	}
}

// status sets a lead's status; whether it changed. pgx.ErrNoRows: no such lead.
func (a *api) status(r *http.Request, posting int64, status string, auto bool, onlyFrom []string) (bool, error) {
	_, err := a.q.SetStatus(r.Context(), store.SetStatusParams{PostingID: posting, Status: status,
		Auto: auto, OnlyFrom: append([]string{}, onlyFrom...)})
	if !errors.Is(err, pgx.ErrNoRows) {
		return err == nil, err
	}
	// nothing inserted: the lead exists and its status stayed, or there is no such lead
	if _, err := a.q.GetApplication(r.Context(), posting); err == nil {
		return false, nil
	}
	exists, err := a.q.LeadExists(r.Context(), posting)
	if err != nil {
		return false, fmt.Errorf("finding lead: %w", err)
	}
	if !exists {
		return false, pgx.ErrNoRows
	}
	return false, nil
}

// Event is one status change.
type Event struct {
	Status string    `json:"status"`
	At     time.Time `json:"at"`
	Auto   bool      `json:"auto"`
	Note   string    `json:"note,omitempty"` // M6-04: "follow-up done", "follow-up snoozed 3 days"
}

// SentDocument is a document marked sent (M5-04), linked to its application.
type SentDocument struct {
	ID     int64     `json:"id"`
	Kind   string    `json:"kind"`
	SentAt time.Time `json:"sent_at"`
}

// application answers a lead's application: status, history (newest first), the posting
// as applied to, and the documents sent. null when the lead never left `new`.
func (a *api) application(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	app, err := a.q.GetApplication(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{"application": nil})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	hist, err := a.q.ApplicationHistory(r.Context(), app.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	docs, err := a.q.SentDocuments(r.Context(), app.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	notes, contacts, err := a.notesAndContacts(r, app.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := struct {
		ID        int64           `json:"id"`
		Status    string          `json:"status"`
		StatusAt  time.Time       `json:"status_at"`
		AppliedAt *time.Time      `json:"applied_at"`
		Snapshot  json.RawMessage `json:"snapshot"` // posting text: untrusted, JSON-encoded only
		History   []Event         `json:"history"`
		Documents []SentDocument  `json:"documents"`
		Notes     []Note          `json:"notes"`    // M6-03
		Contacts  []Contact       `json:"contacts"` // M6-03: personal data, never logged
	}{ID: app.ID, Status: app.Status, StatusAt: app.StatusAt.Time.UTC(), Snapshot: raw(app.Snapshot),
		History: []Event{}, Documents: []SentDocument{}, Notes: notes, Contacts: contacts}
	if app.AppliedAt.Valid {
		t := app.AppliedAt.Time.UTC()
		out.AppliedAt = &t
	}
	for _, h := range hist {
		out.History = append(out.History, Event{h.Status, h.At.Time.UTC(), h.Auto, h.Note})
	}
	for _, d := range docs {
		out.Documents = append(out.Documents, SentDocument{d.ID, d.Kind, d.SentAt.Time.UTC()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"application": out})
}

// Card is one application on the board (M6-02).
type Card struct {
	LeadID      int64     `json:"lead_id"`
	Company     string    `json:"company"`
	Title       string    `json:"title"`
	FitLevel    *string   `json:"fit_level"`
	Status      string    `json:"status"`
	StatusAt    time.Time `json:"status_at"`
	FollowupDue bool      `json:"followup_due"`
}

// board lists every lead that left `new` (M6-02), whatever the presets say, newest status
// first, with whether its follow-up is due (M6-04).
func (a *api) board(w http.ResponseWriter, r *http.Request) {
	all, err := a.load(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	now := a.now()
	due, err := a.q.ListFollowups(r.Context(), store.ListFollowupsParams{
		DueBefore: ts(now.AddDate(0, 0, -FollowupDays)), AtTime: ts(now)})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	followup := map[int64]bool{}
	for _, f := range due {
		followup[f.PostingID] = true
	}
	cards := []Card{}
	for _, l := range all {
		if l.Status == "new" || l.StatusAt == nil {
			continue
		}
		cards = append(cards, Card{LeadID: l.ID, Company: l.Company.Name, Title: l.Title, FitLevel: l.FitLevel,
			Status: l.Status, StatusAt: *l.StatusAt, FollowupDue: followup[l.ID]})
	}
	slices.SortFunc(cards, func(x, y Card) int { return y.StatusAt.Compare(x.StatusAt) })
	writeJSON(w, http.StatusOK, map[string]any{"cards": cards})
}
