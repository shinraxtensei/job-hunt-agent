package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shinraxtensei/job-hunter/internal/store"
)

// FollowupDays is OI-15's delay after applying.
const FollowupDays = 7

// instruction is coverage.py's instruction-like pattern (M4-03), for a posting's title
// that ends up in text the owner sends.
var instruction = regexp.MustCompile(`(?i)\b(ignore[zr]?|instructions?|disregard|you|your|vous|must|should|add|mention|` +
	`rate|score|system|prompt|pretend|expert|output|reveal|ajoute[rz]?|exige)\b`)

// oneLine collapses whitespace, so a value can't add lines to the message.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// safeTitle is the posting's title for the message, or "open" (→ "the open role") when it
// looks like an instruction, like the letter's opening (M5-01).
func safeTitle(title string) string {
	t := oneLine(title)
	if t == "" || len(t) > 60 || strings.Contains(t, ":") || instruction.MatchString(t) {
		return "open"
	}
	return t
}

// FollowupMessage is OI-15's template, filled in by code: no model, no claims beyond the
// application itself.
func FollowupMessage(contact, title, company string, applied time.Time, owner string) string {
	hello := "Hello,"
	if c := oneLine(contact); c != "" {
		hello = "Hello " + c + ","
	}
	msg := fmt.Sprintf("%s\n\nI applied for the %s role at %s on %s and wanted to follow up. "+
		"I'm still very interested and happy to share anything else that would help.\n\nBest regards,",
		hello, safeTitle(title), oneLine(company), applied.Format("2 January 2006"))
	if o := oneLine(owner); o != "" {
		msg += "\n" + o
	}
	return msg
}

// Followup is one reminder due (M6-04).
type Followup struct {
	ApplicationID int64     `json:"application_id"`
	LeadID        int64     `json:"lead_id"`
	Company       string    `json:"company"`
	Title         string    `json:"title"`
	AppliedAt     time.Time `json:"applied_at"`
	Days          int       `json:"days"`
	Contact       string    `json:"contact"`
	Message       string    `json:"message"` // to copy; nothing is sent (rule 6)
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// followups lists the reminders due now, oldest application first.
func (a *api) followups(w http.ResponseWriter, r *http.Request) {
	now := a.now()
	rows, err := a.q.ListFollowups(r.Context(), store.ListFollowupsParams{
		DueBefore: ts(now.AddDate(0, 0, -FollowupDays)), AtTime: ts(now)})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]Followup, 0, len(rows))
	for _, f := range rows {
		applied := f.AppliedAt.Time.UTC()
		out = append(out, Followup{ApplicationID: f.ID, LeadID: f.PostingID, Company: f.Company, Title: f.Title,
			AppliedAt: applied, Days: int(now.Sub(applied).Hours() / 24), Contact: f.Contact,
			Message: FollowupMessage(f.Contact, f.Title, f.Company, applied, f.Owner)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"followups": out})
}

// followupAction marks a reminder done or snoozes it 3 or 7 days (OI-15), in the history.
func (a *api) followupAction(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Action string `json:"action"`
		Days   int    `json:"days"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	now := a.now()
	p := store.FollowupActionParams{ID: id}
	switch {
	case body.Action == "done" && body.Days == 0:
		p.Done, p.Note = true, "follow-up done"
	case body.Action == "snooze" && (body.Days == 3 || body.Days == 7):
		p.SnoozeUntil, p.Note = ts(now.AddDate(0, 0, body.Days)), fmt.Sprintf("follow-up snoozed %d days", body.Days)
	default:
		writeError(w, http.StatusBadRequest, "action", `want {"action":"done"} or {"action":"snooze","days":3|7}`)
		return
	}
	n, err := a.q.FollowupAction(r.Context(), p)
	a.written(w, r, n, err, "applied application")
}
