import { useEffect, useState } from "react";
import { get, requestReason, type Lead } from "./api";
import { label } from "./labels";

const one = new Intl.NumberFormat(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 });
const SEGMENTS: Record<string, number> = { very_strong: 4, strong: 3, good: 2, stretch: 1, no: 0 };

// FitCell is the row's fit: four segments filled by level (a shape, not a colour), the
// score, and the level in words for screen readers and on hover. Unscored leads say why.
export function FitCell({ lead: l }: { lead: Lead }) {
  if (l.score === null || l.fit_level === null) {
    if (l.score_status === "needs_review") return <span className="fit-status">Review</span>;
    if (l.score_status === "pending")
      return (
        <span className="fit-status" title="Being scored">
          …<span className="visually-hidden">Being scored</span>
        </span>
      );
    return (
      <span className="fit-status" title="Not scored">
        —<span className="visually-hidden">Not scored</span>
      </span>
    );
  }
  const filled = SEGMENTS[l.fit_level] ?? 0;
  const level = label("fit", l.fit_level);
  return (
    <span className="fit" title={`${level} fit`}>
      <span className="fit-mark" aria-hidden="true">
        {[0, 1, 2, 3].map((i) => (
          <i key={i} className={i < filled ? "on" : undefined} />
        ))}
      </span>
      <span className="visually-hidden">{level} fit, score </span>
      {one.format(l.score)}
    </span>
  );
}

const POLL_MS = 4000;
const POLL_TRIES = 15;

// useFreshReason asks once for a new reason when an opened lead's reason is from older
// inputs (ADR-0021, ADR-0022), then polls the lead until it is current. It returns the
// lead to show and whether an update is under way.
export function useFreshReason(lead: Lead, open: boolean): [Lead, "idle" | "updating" | "failed"] {
  const [live, setLive] = useState<Lead | null>(null);
  const [state, setState] = useState<"idle" | "updating" | "failed">("idle");
  const shown = live ?? lead;
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

// ScoreDetail is the expanded row's score: the level, the reason, the gaps and the factor
// breakdown. Reason and gaps are model output: React text only.
export function ScoreDetail({ lead: l, updating }: { lead: Lead; updating: "idle" | "updating" | "failed" }) {
  if (l.score_status === "needs_review" && l.score === null) {
    return <p className="score-note">Needs review: the posting couldn’t be read into facts after two tries. It isn’t scored.</p>;
  }
  if (l.score === null || l.fit_level === null || !l.breakdown) {
    return <p className="score-note">{l.score_status === "pending" ? "Being scored." : "Not scored: your default filters don’t show this lead."}</p>;
  }
  return (
    <section className="score" aria-label="Fit">
      <p className="score-head">
        <b>{label("fit", l.fit_level)} fit</b> · {one.format(l.score)} of 100
        {l.cv_variant && <> · CV: {label("cv_variant", l.cv_variant)}</>}
      </p>
      {l.not_enough_info && <p className="score-note">Not enough info: the posting names no required skills, so they count as none.</p>}
      {l.reason ? <p className="reason">{l.reason}</p> : <p className="score-note">The reason is being written.</p>}
      {l.reason && !l.reason_current && (
        <p className="score-note" role="status">
          {updating === "updating"
            ? "This reason is from older inputs. Updating…"
            : updating === "failed"
              ? "This reason is from older inputs; the update didn’t arrive yet. Open the lead later to see it."
              : "This reason is from older inputs."}
        </p>
      )}
      {l.score_status === "needs_review" && l.reason === null && (
        <p className="score-note">Needs review: the reason couldn’t be written after two tries.</p>
      )}
      {l.gaps && l.gaps.length > 0 && (
        <div className="gaps">
          <span className="gaps-label" id={`gaps-${l.id}`}>
            Gaps
          </span>
          <ul aria-labelledby={`gaps-${l.id}`}>
            {l.gaps.map((g) => (
              <li key={g} translate="no">
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
