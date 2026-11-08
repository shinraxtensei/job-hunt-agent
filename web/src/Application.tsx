import { useCallback, useEffect, useState } from "react";
import { STATUSES, get, post, type Application, type Lead, type Status } from "./api";
import { label } from "./labels";
import { NotesAndContacts } from "./Notes";
import { ExternalLink } from "./safe";

// OI-16's skip reasons, in the order offered.
const REASONS = ["location", "seniority", "stack", "salary", "company", "role_type", "visa_relocation", "language", "not_interested", "other"];

const when = (iso: string) => new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });

// ApplicationPanel: a lead's status (changeable), its dated history, the posting as
// applied to, and the documents sent (M6-01). Changing the status is an ADR-0022 write.
export function ApplicationPanel({ lead, onStatus }: { lead: Lead; onStatus?: (s: Status) => void }) {
  const [app, setApp] = useState<Application | null | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(
    () =>
      get<{ application: Application | null }>(`/api/leads/${lead.id}/application`)
        .then((r) => setApp(r.application))
        .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e))),
    [lead.id],
  );
  useEffect(() => {
    void load();
  }, [load]);

  const change = (s: Status) => {
    setError(null);
    post(`/api/leads/${lead.id}/status`, { status: s })
      .then(() => {
        onStatus?.(s);
        return load();
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };
  const [undo, setUndo] = useState<Status | null>(null); // the status before a skip
  const [reason, setReason] = useState<string | null>(null);
  const skip = (why?: string, note?: string) => {
    setError(null);
    post<{ previous: Status }>(`/api/leads/${lead.id}/skip`, why ? { reason: why, note: note ?? "" } : {})
      .then((r) => {
        if (!why) setUndo(r.previous);
        else setReason(why);
        onStatus?.("skipped");
        return load();
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };
  const current = app?.status ?? lead.status;
  const id = `status-${lead.id}`;
  return (
    <section className="application" aria-label="Application">
      <div className="field status-field">
        <label htmlFor={id}>Status</label>
        <select id={id} name="status" value={current} onChange={(e) => change(e.target.value as Status)}>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {label("status", s)}
            </option>
          ))}
        </select>
        {current !== "skipped" && (
          <button type="button" className="button" onClick={() => skip()}>
            Skip
          </button>
        )}
      </div>
      {current === "skipped" && undo && (
        <div className="skip-why" role="group" aria-label="Why skip it?">
          <p>
            Skipped. Why? (optional){" "}
            <button
              type="button"
              className="link-button"
              onClick={() => {
                const back = undo;
                setUndo(null);
                change(back);
              }}
            >
              Undo
            </button>
          </p>
          <div className="reasons">
            {REASONS.map((r) => (
              <button key={r} type="button" className="reason-chip" aria-pressed={reason === r} onClick={() => skip(r)}>
                {label("skip_reason", r)}
              </button>
            ))}
          </div>
        </div>
      )}
      {error && (
        <p className="draft-error" role="alert">
          {error}
        </p>
      )}
      {/* a lead that never left `new` has no application yet: a note starts one (M6-03) */}
      {app === null && <NotesAndContacts leadId={lead.id} notes={[]} contacts={[]} reload={load} />}
      {app && (
        <>
          {app.snapshot && (
            <p className="score-note">
              Applied {app.applied_at && when(app.applied_at)} to: <ExternalLink href={app.snapshot.url}>{app.snapshot.title}</ExternalLink> (as it was then)
            </p>
          )}
          {app.documents.length > 0 && (
            <p className="score-note">
              Sent:{" "}
              {app.documents.map((d, i) => (
                <span key={d.id}>
                  {i > 0 && " · "}
                  <a href={`/api/documents/${d.id}/pdf`} download>
                    {d.kind === "cv" ? "CV" : "Cover letter"}
                  </a>{" "}
                  on {when(d.sent_at)}
                </span>
              ))}
            </p>
          )}
          <NotesAndContacts leadId={lead.id} notes={app.notes} contacts={app.contacts} reload={load} />
          <details className="history">
            <summary>History ({app.history.length})</summary>
            <ol>
              {app.history.map((h, i) => (
                <li key={i}>
                  {label("status", h.status)} · <time dateTime={h.at}>{when(h.at)}</time>
                  {h.auto && " · automatic"}
                  {h.note && ` · ${h.note}`}
                </li>
              ))}
            </ol>
          </details>
        </>
      )}
    </section>
  );
}
