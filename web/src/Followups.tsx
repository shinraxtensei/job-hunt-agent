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

// Followups: applications 7 days after applying with no reply (M6-04), each with the
// follow-up message to copy (OI-15). Nothing is sent from here: copying is the owner's.
export function Followups() {
  const [items, setItems] = useState<Followup[]>([]);
  const [copied, setCopied] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(
    () =>
      get<{ followups: Followup[] }>("/api/followups")
        .then((r) => setItems(r.followups))
        .catch(() => setItems([])), // the leads still work without reminders
    [],
  );
  useEffect(() => {
    void load();
  }, [load]);
  const act = (id: number, body: { action: "done" } | { action: "snooze"; days: 3 | 7 }) => {
    setError(null);
    post(`/api/applications/${id}/followup`, body)
      .then(() => load())
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };
  const copy = (f: Followup) => {
    navigator.clipboard
      .writeText(f.message)
      .then(() => setCopied(f.application_id))
      .catch(() => setError("Couldn’t copy: select the message and copy it by hand."));
  };
  if (items.length === 0) return null;
  return (
    <section className="followups" aria-labelledby="followups-title">
      <h2 id="followups-title">Follow-ups due ({items.length})</h2>
      {error && (
        <p className="draft-error" role="alert">
          {error}
        </p>
      )}
      <ul className="plain">
        {items.map((f) => (
          <li key={f.application_id}>
            <p>
              <b>{f.company}</b> · {f.title} · applied {f.days} days ago{f.contact && <> · {f.contact}</>}
            </p>
            <details>
              <summary>Message</summary>
              <pre className="followup-message">{f.message}</pre>
            </details>
            <div className="followup-actions">
              <button type="button" className="button" onClick={() => copy(f)}>
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
    </section>
  );
}
