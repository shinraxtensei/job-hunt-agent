import { useState, type FormEvent } from "react";
import { send, type Contact, type Note } from "./api";

const stamp = (iso: string) => new Date(iso).toLocaleString(undefined, { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
const EMPTY = { name: "", role: "", email: "", linkedin: "", phone: "" };
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

// useRun runs a write, then reloads the application; the error stays on the panel.
function useRun(reload: () => Promise<void>) {
  const [error, setError] = useState<string | null>(null);
  const run = (p: Promise<void>, after?: () => void) => {
    setError(null);
    p.then(() => {
      after?.();
      return reload();
    }).catch((e: unknown) => setError(message(e)));
  };
  return { error, run };
}

function ErrorLine({ error }: { error: string | null }) {
  return error ? (
    <p className="draft-error" role="alert">
      {error}
    </p>
  ) : null;
}

// NotesPanel: dated notes, newest first (M6-03). Text only.
export function NotesPanel(props: { leadId: number; notes: Note[]; reload: () => Promise<void> }) {
  const { leadId, notes, reload } = props;
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState<{ id: number; body: string } | null>(null);
  const { error, run } = useRun(reload);
  const addNote = (e: FormEvent) => {
    e.preventDefault();
    run(send("POST", `/api/leads/${leadId}/notes`, { body: draft }), () => setDraft(""));
  };
  const id = (s: string) => `${s}-${leadId}`;
  return (
    <section className="notes" aria-label="Notes">
      <ErrorLine error={error} />
      <form onSubmit={addNote} className="note-form">
        <label htmlFor={id("note")} className="visually-hidden">
          New note
        </label>
        <textarea id={id("note")} name="note" rows={3} maxLength={5000} value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="What was said, what's next…" />
        <button type="submit" className="button primary" disabled={!draft.trim()}>
          Add note
        </button>
      </form>
      {notes.length === 0 && <p className="quiet-note">No notes yet.</p>}
      <ul className="cards">
        {notes.map((n) =>
          editing?.id === n.id ? (
            <li key={n.id} className="card">
              <label htmlFor={id(`edit-${n.id}`)} className="visually-hidden">
                Edit note
              </label>
              <textarea id={id(`edit-${n.id}`)} rows={3} maxLength={5000} value={editing.body} onChange={(e) => setEditing({ id: n.id, body: e.target.value })} />
              <div className="row-actions">
                <button type="button" className="button primary" onClick={() => run(send("POST", `/api/notes/${n.id}`, { body: editing.body }), () => setEditing(null))}>
                  Save
                </button>
                <button type="button" className="link-button" onClick={() => setEditing(null)}>
                  Cancel
                </button>
              </div>
            </li>
          ) : (
            <li key={n.id} className="card">
              <p className="note-body">{n.body}</p>
              <p className="quiet-note">
                {stamp(n.created_at)}
                {n.edited_at && ` · edited ${stamp(n.edited_at)}`} ·{" "}
                <button type="button" className="link-button" onClick={() => setEditing({ id: n.id, body: n.body })}>
                  Edit
                </button>{" "}
                <button type="button" className="link-button" onClick={() => window.confirm("Delete this note?") && run(send("DELETE", `/api/notes/${n.id}`))}>
                  Delete
                </button>
              </p>
            </li>
          ),
        )}
      </ul>
    </section>
  );
}

// ContactsPanel: the people on an application (M6-03). Other people's data: text only,
// addresses are not links (OI-14).
export function ContactsPanel(props: { leadId: number; contacts: Contact[]; reload: () => Promise<void> }) {
  const { leadId, contacts, reload } = props;
  const [person, setPerson] = useState(EMPTY);
  const { error, run } = useRun(reload);
  const addContact = (e: FormEvent) => {
    e.preventDefault();
    run(send("POST", `/api/leads/${leadId}/contacts`, person), () => setPerson(EMPTY));
  };
  const id = (s: string) => `${s}-${leadId}`;
  return (
    <section className="contacts" aria-label="Contacts">
      <ErrorLine error={error} />
      {contacts.length === 0 && <p className="quiet-note">No contacts yet.</p>}
      <ul className="cards">
        {contacts.map((c) => (
          <li key={c.id} className="card">
            <p className="card-title">{c.name}</p>
            <p className="quiet-note">
              {[c.role, c.email, c.phone, c.linkedin].filter(Boolean).join(" · ")}{" "}
              <button type="button" className="link-button" onClick={() => window.confirm(`Delete ${c.name}?`) && run(send("DELETE", `/api/contacts/${c.id}`))}>
                Delete
              </button>
            </p>
          </li>
        ))}
      </ul>
      <form onSubmit={addContact} className="contact-form">
        {(
          [
            ["name", "Name", "text"],
            ["role", "Role", "text"],
            ["email", "Email", "email"],
            ["linkedin", "LinkedIn URL", "url"],
            ["phone", "Phone", "tel"],
          ] as const
        ).map(([k, text, type]) => (
          <div key={k} className="input-field">
            <label htmlFor={id(`contact-${k}`)}>{text}</label>
            <input
              id={id(`contact-${k}`)}
              name={k}
              type={type}
              autoComplete="off"
              spellCheck={k === "name" || k === "role"}
              value={person[k]}
              required={k === "name"}
              onChange={(e) => setPerson({ ...person, [k]: e.target.value })}
            />
          </div>
        ))}
        <button type="submit" className="button primary" disabled={!person.name.trim()}>
          Add contact
        </button>
      </form>
    </section>
  );
}
