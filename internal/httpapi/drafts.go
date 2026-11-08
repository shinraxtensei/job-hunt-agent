package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunter/internal/store"
)

// guardWrite refuses a write that isn't same-origin JSON (ADR-0022): cross-origin
// protection (Sec-Fetch-Site, else Origin vs Host) and a JSON content type, which a
// cross-site form or image can't send without a CORS preflight the API never answers.
func guardWrite(w http.ResponseWriter, r *http.Request) bool {
	if err := crossOrigin.Check(r); err != nil {
		writeError(w, http.StatusForbidden, "", "cross-site requests are refused")
		return false
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusForbidden, "", "want Content-Type: application/json")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id", "want a lead id")
		return 0, false
	}
	return id, true
}

// requestDraft asks for a tailored CV draft (M4-02, ADR-0025): one `requested` row for a
// scored lead; the worker makes it. Nothing is ever sent.
func (a *api) requestDraft(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	draftID, err := a.q.RequestDraft(r.Context(), id)
	switch {
	case errors.Is(err, pgx.ErrNoRows): // not scored, or a request already waits
		writeJSON(w, http.StatusAccepted, map[string]any{"draft_id": nil})
	case err != nil:
		a.fail(w, r, err)
	default:
		writeJSON(w, http.StatusAccepted, map[string]any{"draft_id": draftID})
	}
}

// Draft is one tailored CV draft. Content, coverage and findings are as the worker
// stored them: CV text and model output, only ever JSON-encoded.
type Draft struct {
	ID              int64           `json:"id"`
	Status          string          `json:"status"` // requested | blocked | ready_for_review | approved | discarded | failed
	Variant         *string         `json:"variant"`
	Content         json.RawMessage `json:"content"`
	Coverage        json.RawMessage `json:"coverage"`
	Findings        json.RawMessage `json:"findings"`
	EvidenceVersion *string         `json:"evidence_version"`
	PromptVersion   *int32          `json:"prompt_version"`
	Model           *string         `json:"model"`
	Error           *string         `json:"error"`
	RequestedAt     time.Time       `json:"requested_at"`
	MadeAt          *time.Time      `json:"made_at"`
	DecidedAt       *time.Time      `json:"decided_at"`
}

func raw(b []byte) json.RawMessage {
	if b == nil {
		return json.RawMessage("null")
	}
	return b
}

// drafts lists a lead's drafts, newest first (up to 10).
func (a *api) drafts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rows, err := a.q.ListDrafts(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]Draft, 0, len(rows))
	for _, d := range rows {
		x := Draft{ID: d.ID, Status: d.Status, Content: raw(d.Content), Coverage: raw(d.Coverage),
			Findings: raw(d.Findings), RequestedAt: d.RequestedAt.Time.UTC()}
		if d.Variant.Valid {
			x.Variant = &d.Variant.String
		}
		if d.EvidenceVersion.Valid {
			x.EvidenceVersion = &d.EvidenceVersion.String
		}
		if d.PromptVersion.Valid {
			x.PromptVersion = &d.PromptVersion.Int32
		}
		if d.Model.Valid {
			x.Model = &d.Model.String
		}
		if d.Error.Valid {
			x.Error = &d.Error.String
		}
		if d.MadeAt.Valid {
			t := d.MadeAt.Time.UTC()
			x.MadeAt = &t
		}
		if d.DecidedAt.Valid {
			t := d.DecidedAt.Time.UTC()
			x.DecidedAt = &t
		}
		out = append(out, x)
	}
	evidence, err := a.citedEvidence(r, rows)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"drafts": out, "evidence": evidence})
}

// EvidenceLine is a cited line's text (M4-05 shows it next to the sentence).
type EvidenceLine struct {
	Text    string `json:"text"`
	Retired bool   `json:"retired"`
}

// citedEvidence is the text of every evidence line the drafts cite, by id.
func (a *api) citedEvidence(r *http.Request, rows []store.ListDraftsRow) (map[string]EvidenceLine, error) {
	var ids []string
	for _, d := range rows {
		var c struct {
			Summary []struct {
				EvidenceIDs []string `json:"evidence_ids"`
			} `json:"summary"`
			Roles, Projects []struct {
				Bullets []struct {
					EvidenceIDs []string `json:"evidence_ids"`
				} `json:"bullets"`
			}
		}
		if len(d.Content) == 0 {
			continue
		}
		if err := json.Unmarshal(d.Content, &c); err != nil {
			return nil, fmt.Errorf("decoding draft %d: %w", d.ID, err)
		}
		for _, b := range c.Summary {
			ids = append(ids, b.EvidenceIDs...)
		}
		for _, sec := range append(c.Roles, c.Projects...) {
			for _, b := range sec.Bullets {
				ids = append(ids, b.EvidenceIDs...)
			}
		}
	}
	out := map[string]EvidenceLine{}
	if len(ids) == 0 {
		return out, nil
	}
	lines, err := a.q.EvidenceLines(r.Context(), ids)
	if err != nil {
		return nil, fmt.Errorf("loading evidence lines: %w", err)
	}
	for _, l := range lines {
		out[l.ID] = EvidenceLine{l.Text, l.Retired}
	}
	return out, nil
}

// decide approves or discards a draft (M4-05). Approve needs `ready_for_review`; a
// blocked draft can only be discarded. Nothing is sent or exported (rule 6).
func (a *api) decide(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardWrite(w, r) {
			return
		}
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		_, err := a.q.DecideDraft(r.Context(), store.DecideDraftParams{ID: id, Status: status})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			writeError(w, http.StatusConflict, "", "this draft can't be "+status+" in its current state")
		case err != nil:
			a.fail(w, r, err)
		default:
			writeJSON(w, http.StatusOK, map[string]string{"status": status})
		}
	}
}
