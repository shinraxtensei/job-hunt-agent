import { useCallback, useEffect, useState } from "react";
import { STATUSES, get, post, type Application, type Lead, type Previous, type Status } from "./api";
import { statusTone } from "./format";
import { label } from "./labels";
import { ExternalLink } from "./safe";

// OI-16's skip reasons, in the order offered.
const REASONS = ["location", "seniority", "stack", "salary", "company", "role_type", "visa_relocation", "language", "not_interested", "other"];

export const when = (iso: string) => new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

// useApplication loads a lead's application (M6-01): undefined while loading, null when
// the lead never left `new`. The detail panel's tabs share it.
export function useApplication(leadId: number) {
  const [app, setApp] = useState<Application | null | undefined>(undefined);
  const [previous, setPrevious] = useState<Previous[]>([]); // set aside (M9-05)
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(
    () =>
      get<{ application: Application | null; previous?: Previous[] }>(`/api/leads/${leadId}/application`)
        .then((r) => {
          setApp(r.application);
          setPrevious(r.previous ?? []);
        })
        .catch((e: unknown) => setError(message(e))),
    [leadId],
  );
  useEffect(() => {
    setApp(undefined);
    setPrevious([]);
    void load();
  }, [load]);
  return { app, previous, error, load };
}

// StatusControl: the lead's status (any may follow any, M6-01), one-click Skip with Undo
// and an optional reason (M6-05). Changes are ADR-0022 writes.
export function StatusControl(props: { lead: Lead; app: Application | null | undefined; reload: () => Promise<void>; onStatus?: (s: Status) => void }) {
  const { lead, app, reload, onStatus } = props;
  const [error, setError] = useState<string | null>(null);
  const [undo, setUndo] = useState<Status | null>(null); // the status before a skip
  const [reason, setReason] = useState<string | null>(null);
  const change = (s: Status) => {
    setError(null);
    post(`/api/leads/${lead.id}/status`, { status: s })
      .then(() => {
        onStatus?.(s);
        return reload();
      })
      .catch((e: unknown) => setError(message(e)));
  };
  const skip = (why?: string) => {
    setError(null);
    post<{ previous: Status }>(`/api/leads/${lead.id}/skip`, why ? { reason: why, note: "" } : {})
      .then((r) => {
        if (!why) setUndo(r.previous);
        else setReason(why);
        onStatus?.("skipped");
        return reload();
      })
      .catch((e: unknown) => setError(message(e)));
  };
  // M9-05: an application that ended can be set aside; the next status starts a new one.
  const again = () => {
    setError(null);
    post(`/api/leads/${lead.id}/reapply`, {})
      .then(() => {
        onStatus?.("new");
        return reload();
      })
      .catch((e: unknown) => setError(message(e)));
  };
  const current = app === null ? "new" : (app?.status ?? lead.status);
  const id = `status-${lead.id}`;
  return (
    <section className={`tile status-tile ${statusTone[current] ?? "lilac"}`} aria-label="Application">
      <label htmlFor={id} className="tile-label">
        Status
      </label>
      <div className="status-row">
        <select id={id} name="status" value={current} onChange={(e) => change(e.target.value as Status)}>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {label("status", s)}
            </option>
          ))}
        </select>
        {current !== "skipped" && current !== "rejected" && current !== "closed" && (
          <button type="button" className="button on-pastel" onClick={() => skip()}>
            Skip
          </button>
        )}
        {(current === "rejected" || current === "closed") && (
          <button type="button" className="button on-pastel" onClick={again}>
            Apply again
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
    </section>
  );
}

// HistoryPanel: the posting as applied to, the documents sent (M5-04) and every status
// change, newest first (M6-01). Snapshot text is the posting's: text only.
export function HistoryPanel({ app, previous = [] }: { app: Application | null | undefined; previous?: Previous[] }) {
  if (app === undefined) return <p role="status">Loading…</p>;
  const earlier = previous.map((p) => (
    <p key={p.id} className="quiet-note">
      Earlier application: {label("status", p.status)} on {when(p.status_at)}
      {p.applied_at && <>, applied {when(p.applied_at)}</>}
    </p>
  ));
  if (app === null)
    return (
      <div className="history">
        <p className="quiet-note">{previous.length ? "A new application starts with the next status." : "No history yet: set a status to start one."}</p>
        {earlier}
      </div>
    );
  return (
    <div className="history">
      {earlier}
      {app.snapshot && (
        <p>
          Applied {app.applied_at && when(app.applied_at)} to: <ExternalLink href={app.snapshot.url}>{app.snapshot.title}</ExternalLink> (as it was then)
        </p>
      )}
      {app.documents.length > 0 && (
        <p>
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
      <ol className="timeline" aria-label={`History (${app.history.length})`}>
        {app.history.map((h, i) => (
          <li key={i}>
            <span className={`tag ${statusTone[h.status] ?? "lilac"}`}>{label("status", h.status)}</span>
            <time dateTime={h.at}>{when(h.at)}</time>
            {h.auto && <span className="quiet-note"> · automatic</span>}
            {h.note && <span className="quiet-note"> · {h.note}</span>}
          </li>
        ))}
      </ol>
    </div>
  );
}
