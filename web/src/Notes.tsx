import { useState, type FormEvent } from "react";
import { send, type Contact, type Note } from "./api";

const stamp = (iso: string) => new Date(iso).toLocaleString(undefined, { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
const EMPTY = { name: "", role: "", email: "", linkedin: "", phone: "" };

// NotesAndContacts: dated notes (newest first) and the people on an application (M6-03).
// Everything renders as text; contact addresses are not links (OI-14).
export function NotesAndContacts(props: { leadId: number; notes: Note[]; contacts: Contact[]; reload: () => Promise<void> }) {
  const { leadId, notes, contacts, reload } = props;
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState<{ id: number; body: string } | null>(null);
  const [person, setPerson] = useState(EMPTY);
  const [error, setError] = useState<string | null>(null);
  const run = (p: Promise<void>, after?: () => void) => {
    setError(null);
    p.then(() => {
      after?.();
      return reload();
    }).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };
  const addNote = (e: FormEvent) => {
    e.preventDefault();
    run(send("POST", `/api/leads/${leadId}/notes`, { body: draft }), () => setDraft(""));
  };
  const addContact = (e: FormEvent) => {
    e.preventDefault();
    run(send("POST", `/api/leads/${leadId}/contacts`, person), () => setPerson(EMPTY));
  };
  const id = (s: string) => `${s}-${leadId}`;
  return (
    <div className="notes">
      {error && (
        <p className="draft-error" role="alert">
          {error}
        </p>
      )}
      <section aria-label="Notes">
        <p className="mini-head">Notes</p>
        <form onSubmit={addNote}>
          <label htmlFor={id("note")} className="visually-hidden">
            New note
          </label>
          <textarea id={id("note")} name="note" rows={2} maxLength={5000} value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="What was said, what's next…" />
          <button type="submit" className="button" disabled={!draft.trim()}>
            Add note
          </button>
        </form>
        <ul className="plain">
          {notes.map((n) =>
            editing?.id === n.id ? (
              <li key={n.id}>
                <label htmlFor={id(`edit-${n.id}`)} className="visually-hidden">
                  Edit note
                </label>
                <textarea id={id(`edit-${n.id}`)} rows={2} maxLength={5000} value={editing.body} onChange={(e) => setEditing({ id: n.id, body: e.target.value })} />
                <button type="button" className="button" onClick={() => run(send("POST", `/api/notes/${n.id}`, { body: editing.body }), () => setEditing(null))}>
                  Save
                </button>{" "}
                <button type="button" className="link-button" onClick={() => setEditing(null)}>
                  Cancel
                </button>
              </li>
            ) : (
              <li key={n.id}>
                <p className="note-body">{n.body}</p>
                <p className="cv-dates">
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
      <section aria-label="Contacts">
        <p className="mini-head">Contacts</p>
        <ul className="plain">
          {contacts.map((c) => (
            <li key={c.id}>
              <b>{c.name}</b>
              {[c.role, c.email, c.phone, c.linkedin].filter(Boolean).map((v) => ` · ${v}`)}{" "}
              <button type="button" className="link-button" onClick={() => window.confirm(`Delete ${c.name}?`) && run(send("DELETE", `/api/contacts/${c.id}`))}>
                Delete
              </button>
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
            <div key={k} className="field">
              <label htmlFor={id(`contact-${k}`)}>{text}</label>
              <input id={id(`contact-${k}`)} name={k} type={type} value={person[k]} required={k === "name"} onChange={(e) => setPerson({ ...person, [k]: e.target.value })} />
            </div>
          ))}
          <button type="submit" className="button" disabled={!person.name.trim()}>
            Add contact
          </button>
        </form>
      </section>
    </div>
  );
}
