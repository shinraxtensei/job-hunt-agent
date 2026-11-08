package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

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
	Letter          json.RawMessage `json:"letter"` // M5-01
	Coverage        json.RawMessage `json:"coverage"`
	Findings        json.RawMessage `json:"findings"`
	EvidenceVersion *string         `json:"evidence_version"`
	PromptVersion   *int32          `json:"prompt_version"`
	Model           *string         `json:"model"`
	Error           *string         `json:"error"`
	RequestedAt     time.Time       `json:"requested_at"`
	MadeAt          *time.Time      `json:"made_at"`
	DecidedAt       *time.Time      `json:"decided_at"`
	Documents       []Document      `json:"documents"` // M5-04: its exported PDFs, newest first
}

// Document is one exported PDF, without its bytes (M5-04).
type Document struct {
	ID     int64      `json:"id"`
	Kind   string     `json:"kind"` // cv | letter
	SHA256 string     `json:"sha256"`
	MadeAt time.Time  `json:"made_at"`
	SentAt *time.Time `json:"sent_at"`
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
	docs, err := a.q.ListDocuments(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	byDraft := map[int64][]Document{}
	for _, d := range docs {
		x := Document{ID: d.ID, Kind: d.Kind, SHA256: hex.EncodeToString(d.Sha256), MadeAt: d.MadeAt.Time.UTC()}
		if d.SentAt.Valid {
			t := d.SentAt.Time.UTC()
			x.SentAt = &t
		}
		byDraft[d.DraftID] = append(byDraft[d.DraftID], x)
	}
	out := make([]Draft, 0, len(rows))
	for _, d := range rows {
		x := Draft{ID: d.ID, Status: d.Status, Content: raw(d.Content), Letter: raw(d.Letter), Coverage: raw(d.Coverage),
			Findings: raw(d.Findings), RequestedAt: d.RequestedAt.Time.UTC(),
			Documents: append([]Document{}, byDraft[d.ID]...)}
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

type cited struct {
	EvidenceIDs []string `json:"evidence_ids"`
}

// citedEvidence is the text of every evidence line the drafts (CV and letter) cite, by id.
func (a *api) citedEvidence(r *http.Request, rows []store.ListDraftsRow) (map[string]EvidenceLine, error) {
	var ids []string
	for _, d := range rows {
		var c struct {
			Summary         []cited `json:"summary"`
			Roles, Projects []struct {
				Bullets []cited `json:"bullets"`
			}
		}
		var l struct {
			Paragraphs [][]cited `json:"paragraphs"`
		}
		if len(d.Content) == 0 {
			continue
		}
		if err := json.Unmarshal(d.Content, &c); err != nil {
			return nil, fmt.Errorf("decoding draft %d: %w", d.ID, err)
		}
		if len(d.Letter) > 0 {
			if err := json.Unmarshal(d.Letter, &l); err != nil {
				return nil, fmt.Errorf("decoding draft %d letter: %w", d.ID, err)
			}
		}
		for _, p := range l.Paragraphs {
			for _, b := range p {
				ids = append(ids, b.EvidenceIDs...)
			}
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
		posting, err := a.q.DecideDraft(r.Context(), store.DecideDraftParams{ID: id, Status: status})
		if err == nil && status == "approved" { // OI-11: the lead moves on to drafted by itself
			_, err = a.status(r, posting, "drafted", true, []string{"new", "shortlisted"})
		}
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

// markSent records that the owner sent a document (M5-04): sent_at is set once, and
// marking it again returns the first time. The system sends nothing (rule 6).
func (a *api) markSent(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	at, err := a.q.MarkDocumentSent(r.Context(), id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "id", "no such document")
	case err != nil:
		a.fail(w, r, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"sent_at": at.Time.UTC()})
	}
}

// fileWord keeps letters and digits (any script) for a download's file name; anything
// else becomes "-", so a name can't break the header.
func fileWord(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// documentPDF serves one exported PDF as a download (M5-03): the owner's own data, the
// bytes exactly as stored (ADR-0026).
func (a *api) documentPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := a.q.DocumentPDF(r.Context(), id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "id", "no such document")
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}
	kind := map[string]string{"cv": "CV", "letter": "Cover-letter"}[d.Kind]
	name := strings.Join(slices.DeleteFunc([]string{fileWord(d.Person), kind, fileWord(d.Company)},
		func(s string) bool { return s == "" }), "-") + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(d.Pdf) // the client went away: nothing to do
}
