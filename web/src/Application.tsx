import { useCallback, useEffect, useState } from "react";
import { STATUSES, get, post, type Application, type Lead, type Status } from "./api";
import { label } from "./labels";
import { ExternalLink } from "./safe";

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
      </div>
      {error && (
        <p className="draft-error" role="alert">
          {error}
        </p>
      )}
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
          <details className="history">
            <summary>History ({app.history.length})</summary>
            <ol>
              {app.history.map((h, i) => (
                <li key={i}>
                  {label("status", h.status)} · <time dateTime={h.at}>{when(h.at)}</time>
                  {h.auto && " · automatic"}
                </li>
              ))}
            </ol>
          </details>
        </>
      )}
    </section>
  );
}
