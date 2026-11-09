import { useCallback, useEffect, useState } from "react";
import { get, post } from "./api";

export type Followup = {
  application_id: number;
  lead_id: number;
  company: string;
  title: string;
  applied_at: string;
  days: number;
  contact: string;
  message: string;
};

// useFollowups loads the reminders due (M6-04); an error leaves the list empty, so the
// rest of the page still works.
export function useFollowups() {
  const [items, setItems] = useState<Followup[] | null>(null);
  const load = useCallback(
    () =>
      get<{ followups: Followup[] }>("/api/followups")
        .then((r) => setItems(r.followups))
        .catch(() => setItems([])),
    [],
  );
  useEffect(() => {
    void load();
  }, [load]);
  return { items, load };
}

// FollowupCards: applications 7 days after applying with no reply (M6-04), each as a card
// with the follow-up message to copy (OI-15). Nothing is sent from here.
export function FollowupCards({ items, reload }: { items: Followup[]; reload: () => Promise<void> }) {
  const [copied, setCopied] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const act = (id: number, body: { action: "done" } | { action: "snooze"; days: 3 | 7 }) => {
    setError(null);
    post(`/api/applications/${id}/followup`, body)
      .then(() => reload())
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };
  const copy = (f: Followup) => {
    navigator.clipboard
      .writeText(f.message)
      .then(() => setCopied(f.application_id))
      .catch(() => setError("Couldn’t copy: select the message and copy it by hand."));
  };
  return (
    <>
      {error && (
        <p className="draft-error" role="alert">
          {error}
        </p>
      )}
      <ul className="cards">
        {items.map((f) => (
          <li key={f.application_id} className="card">
            <div className="card-main">
              <div>
                <p className="card-title">{f.company}</p>
                <p className="card-sub">{f.title}</p>
              </div>
              <div className="side">
                <span className="tag sand">Follow-up due</span>
                <p className="when">applied {f.days} d ago</p>
              </div>
            </div>
            {f.contact && <p className="quiet-note">Contact: {f.contact}</p>}
            <details>
              <summary>Message</summary>
              <pre className="followup-message">{f.message}</pre>
            </details>
            <div className="row-actions">
              <button type="button" className="button primary" onClick={() => copy(f)}>
                {copied === f.application_id ? "Copied" : "Copy message"}
              </button>
              <button type="button" className="link-button" onClick={() => act(f.application_id, { action: "done" })}>
                Done
              </button>
              <button type="button" className="link-button" onClick={() => act(f.application_id, { action: "snooze", days: 3 })}>
                Snooze 3 days
              </button>
              <button type="button" className="link-button" onClick={() => act(f.application_id, { action: "snooze", days: 7 })}>
                Snooze 7 days
              </button>
            </div>
          </li>
        ))}
      </ul>
    </>
  );
}

// FollowupsPage: every reminder due.
export function FollowupsPage() {
  const { items, load } = useFollowups();
  return (
    <div className="page">
      <h1>Follow-ups</h1>
      <p className="page-sub">Applications 7 days after applying with no reply. Copy the message, send it yourself, then mark it done.</p>
      {items === null ? (
        <p role="status">Loading…</p>
      ) : items.length === 0 ? (
        <p className="empty">Nothing due. Reminders appear 7 days after you set a lead to Applied.</p>
      ) : (
        <FollowupCards items={items} reload={load} />
      )}
    </div>
  );
}
