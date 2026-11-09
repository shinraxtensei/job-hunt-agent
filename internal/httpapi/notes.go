package httpapi

import (
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// Note is one of the owner's dated notes on an application (M6-03). Free text: only ever
// JSON-encoded and rendered as text; never logged.
type Note struct {
	ID        int64      `json:"id"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	EditedAt  *time.Time `json:"edited_at"`
}

// Contact is a person on an application: someone else's personal data (OI-14), kept only
// in the database; never logged, sent to a model or exported.
type Contact struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Email    string `json:"email"`
	LinkedIn string `json:"linkedin"`
	Phone    string `json:"phone"`
}

var phone = regexp.MustCompile(`^[0-9+()./ -]{3,40}$`)

// check validates a contact's fields (format only); the field at fault, or "".
func (c *Contact) check() (string, string) {
	c.Name, c.Role = strings.TrimSpace(c.Name), strings.TrimSpace(c.Role)
	c.Email, c.LinkedIn, c.Phone = strings.TrimSpace(c.Email), strings.TrimSpace(c.LinkedIn), strings.TrimSpace(c.Phone)
	switch {
	case c.Name == "" || utf8.RuneCountInString(c.Name) > 200:
		return "name", "want a name of at most 200 characters"
	case utf8.RuneCountInString(c.Role) > 200:
		return "role", "at most 200 characters"
	case c.Email != "" && !validEmail(c.Email):
		return "email", "want an email address"
	case c.LinkedIn != "" && !validLinkedIn(c.LinkedIn):
		return "linkedin", "want an https://…linkedin.com/ address"
	case c.Phone != "" && !phone.MatchString(c.Phone):
		return "phone", "want digits, spaces and + ( ) . - only"
	}
	return "", ""
}

func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && len(s) <= 254
}

func validLinkedIn(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || len(s) > 300 || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "linkedin.com" || strings.HasSuffix(h, ".linkedin.com")
}

func noteBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Body string `json:"body"`
	}
	if !readJSON(w, r, &body) {
		return "", false
	}
	body.Body = strings.TrimSpace(body.Body)
	if body.Body == "" || utf8.RuneCountInString(body.Body) > 5000 {
		writeError(w, http.StatusBadRequest, "body", "want a note of 1 to 5000 characters")
		return "", false
	}
	return body.Body, true
}

// ensure returns the lead's application id, creating it at `new` if needed.
func (a *api) ensure(w http.ResponseWriter, r *http.Request, lead int64) (int64, bool) {
	id, err := a.q.EnsureApplication(r.Context(), lead)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "id", "no such lead")
		return 0, false
	}
	if err != nil {
		a.fail(w, r, err)
		return 0, false
	}
	return id, true
}

func (a *api) addNote(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	lead, ok := pathID(w, r)
	if !ok {
		return
	}
	body, ok := noteBody(w, r)
	if !ok {
		return
	}
	app, ok := a.ensure(w, r, lead)
	if !ok {
		return
	}
	id, err := a.q.AddNote(r.Context(), store.AddNoteParams{ApplicationID: app, Body: body})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (a *api) editNote(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	body, ok := noteBody(w, r)
	if !ok {
		return
	}
	n, err := a.q.EditNote(r.Context(), store.EditNoteParams{ID: id, Body: body})
	a.written(w, r, n, err, "note")
}

func (a *api) deleteNote(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, err := a.q.DeleteNote(r.Context(), id)
	a.written(w, r, n, err, "note")
}

// written answers an edit or delete of one row: 404 when it didn't exist.
func (a *api) written(w http.ResponseWriter, r *http.Request, n int64, err error, what string) {
	switch {
	case err != nil:
		a.fail(w, r, err)
	case n == 0:
		writeError(w, http.StatusNotFound, "id", "no such "+what)
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func contactBody(w http.ResponseWriter, r *http.Request) (Contact, bool) {
	var c Contact
	if !readJSON(w, r, &c) {
		return c, false
	}
	if c.ID != 0 {
		writeError(w, http.StatusBadRequest, "id", "the id is in the path")
		return c, false
	}
	if field, msg := c.check(); field != "" {
		writeError(w, http.StatusBadRequest, field, msg)
		return c, false
	}
	return c, true
}

func (a *api) addContact(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	lead, ok := pathID(w, r)
	if !ok {
		return
	}
	c, ok := contactBody(w, r)
	if !ok {
		return
	}
	app, ok := a.ensure(w, r, lead)
	if !ok {
		return
	}
	id, err := a.q.AddContact(r.Context(), store.AddContactParams{ApplicationID: app, Name: c.Name, Role: c.Role,
		Email: c.Email, Linkedin: c.LinkedIn, Phone: c.Phone})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (a *api) editContact(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, ok := contactBody(w, r)
	if !ok {
		return
	}
	n, err := a.q.EditContact(r.Context(), store.EditContactParams{ID: id, Name: c.Name, Role: c.Role,
		Email: c.Email, Linkedin: c.LinkedIn, Phone: c.Phone})
	a.written(w, r, n, err, "contact")
}

func (a *api) deleteContact(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, err := a.q.DeleteContact(r.Context(), id)
	a.written(w, r, n, err, "contact")
}

// notesAndContacts loads an application's notes (newest first) and contacts.
func (a *api) notesAndContacts(r *http.Request, app int64) ([]Note, []Contact, error) {
	ns, err := a.q.ListNotes(r.Context(), app)
	if err != nil {
		return nil, nil, err
	}
	cs, err := a.q.ListContacts(r.Context(), app)
	if err != nil {
		return nil, nil, err
	}
	notes, contacts := make([]Note, 0, len(ns)), make([]Contact, 0, len(cs))
	for _, n := range ns {
		x := Note{ID: n.ID, Body: n.Body, CreatedAt: n.CreatedAt.Time.UTC()}
		if n.EditedAt.Valid {
			t := n.EditedAt.Time.UTC()
			x.EditedAt = &t
		}
		notes = append(notes, x)
	}
	for _, c := range cs {
		contacts = append(contacts, Contact{c.ID, c.Name, c.Role, c.Email, c.Linkedin, c.Phone})
	}
	return notes, contacts, nil
}
