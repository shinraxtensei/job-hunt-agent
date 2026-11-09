import { useEffect, useState } from "react";
import { get, requestReason, type Lead } from "./api";
import { fitTone, one } from "./format";
import { label } from "./labels";

const POLL_MS = 4000;
const POLL_TRIES = 15;

// useFreshReason asks once for a new reason when an opened lead's reason is from older
// inputs (ADR-0021, ADR-0022), then polls the lead until it is current. It returns the
// lead to show and whether an update is under way.
export function useFreshReason(lead: Lead, open: boolean): [Lead, "idle" | "updating" | "failed"] {
  const [live, setLive] = useState<Lead | null>(null);
  const [state, setState] = useState<"idle" | "updating" | "failed">("idle");
  const shown = live && live.id === lead.id ? live : lead;
  const stale = shown.score !== null && !shown.reason_current && shown.score_status !== "needs_review";
  useEffect(() => {
    if (!open || !stale) return;
    let stop = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    setState("updating");
    const poll = (n: number) => {
      timer = setTimeout(() => {
        get<Lead>(`/api/leads/${lead.id}`).then(
          (l) => {
            if (stop) return;
            if (l.reason_current || l.score_status === "needs_review") {
              setLive(l);
              setState("idle");
            } else if (n < POLL_TRIES) poll(n + 1);
            else setState("failed");
          },
          () => !stop && setState("failed"),
        );
      }, POLL_MS);
    };
    requestReason(lead.id).then(
      () => !stop && poll(1),
      () => !stop && setState("failed"),
    );
    return () => {
      stop = true;
      clearTimeout(timer);
    };
  }, [open, stale, lead.id]);
  return [shown, state];
}

// FitTile is the Overview tab's fit (M3-08, M7-00): the level as a filled pastel tile with
// the score; unscored leads say why.
export function FitTile({ lead: l }: { lead: Lead }) {
  if (l.score === null || l.fit_level === null) {
    const why =
      l.score_status === "needs_review"
        ? "Needs review: the posting couldn’t be read into facts after two tries."
        : l.score_status === "pending"
          ? "Being scored."
          : "Not scored: your default filters don’t show this lead.";
    return (
      <section className="tile quiet" aria-label="Fit">
        <p className="tile-label">Fit</p>
        <p className="tile-note">{why}</p>
      </section>
    );
  }
  return (
    <section className={`tile ${fitTone[l.fit_level] ?? "lilac"}`} aria-label="Fit">
      <p className="tile-label">Fit</p>
      <p className="tile-number">
        {one.format(l.score)}
        <span className="of"> / 100</span>
      </p>
      <p className="tile-note">
        {label("fit", l.fit_level)} fit{l.cv_variant && <> · CV: {label("cv_variant", l.cv_variant)}</>}
      </p>
    </section>
  );
}

// ScoreDetail is the reason, the gaps and the factor breakdown. Reason and gaps are model
// output: React text only.
export function ScoreDetail({ lead: l, updating }: { lead: Lead; updating: "idle" | "updating" | "failed" }) {
  if (l.score === null || l.fit_level === null || !l.breakdown) return null;
  return (
    <section className="score" aria-label="Why this fit">
      {l.not_enough_info && <p className="quiet-note">Not enough info: the posting names no required skills, so they count as none.</p>}
      {l.reason ? <p className="reason">{l.reason}</p> : <p className="quiet-note">The reason is being written.</p>}
      {l.reason && !l.reason_current && (
        <p className="quiet-note" role="status">
          {updating === "updating"
            ? "This reason is from older inputs. Updating…"
            : updating === "failed"
              ? "This reason is from older inputs; the update didn’t arrive yet. Open the lead later to see it."
              : "This reason is from older inputs."}
        </p>
      )}
      {l.score_status === "needs_review" && l.reason === null && <p className="quiet-note">Needs review: the reason couldn’t be written after two tries.</p>}
      {l.gaps && l.gaps.length > 0 && (
        <div className="gaps">
          <span className="gaps-label" id={`gaps-${l.id}`}>
            Gaps
          </span>
          <ul aria-labelledby={`gaps-${l.id}`}>
            {l.gaps.map((g) => (
              <li key={g} className="tag salmon" translate="no">
                {g}
              </li>
            ))}
          </ul>
        </div>
      )}
      <table className="breakdown">
        <caption className="visually-hidden">Score breakdown: points by factor</caption>
        <tbody>
          {l.breakdown.map((f) => (
            <tr key={f.factor}>
              <th scope="row">{label("factor", f.factor)}</th>
              <td className="bar" aria-hidden="true">
                <span style={{ width: `${Math.round(Math.max(0, Math.min(1, f.value)) * 100)}%` }} />
              </td>
              <td className="pts">
                {one.format(f.points)} <span className="of">of {f.weight}</span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
