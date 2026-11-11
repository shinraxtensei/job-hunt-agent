import type { Ref } from "react";
import type { Lead } from "./api";
import { ago, dateFmt, fitTone, isNew, num, one, place } from "./format";
import { label, reason } from "./labels";
import { ExternalLink } from "./safe";

// FitTag is a lead's fit as a filled pastel tag: the level and the score in words and
// numbers (the colour only repeats them). Unscored leads say why.
export function FitTag({ lead: l }: { lead: Lead }) {
  if (l.score === null || l.fit_level === null) {
    const why = l.score_status === "needs_review" ? "Review" : l.score_status === "pending" ? "Scoring…" : "Not scored";
    return <span className="tag quiet">{why}</span>;
  }
  const level = label("fit", l.fit_level);
  return (
    <>
      <span className={`tag ${fitTone[l.fit_level] ?? "lilac"}`} title={`${level} fit`}>
        <span className="visually-hidden">{level} fit, score </span>
        <span aria-hidden="true">{level} · </span>
        {one.format(l.score)}
      </span>
      {l.stretch && <StretchTag />}
    </>
  );
}

// StretchTag (M9-03): the job asks for more than the profile, whatever its fit.
export function StretchTag() {
  return (
    <span className="tag quiet" title={STRETCH}>
      Stretch
    </span>
  );
}
export const STRETCH = "A step up: the job asks for a higher level or more years than your profile.";

type Props = {
  leads: Lead[];
  hidden: boolean;
  now?: number;
  selected?: number; // index into leads, in screen order
  onSelect?: (i: number) => void;
  rowRef?: (i: number) => Ref<HTMLLIElement>;
  byScore?: boolean; // best fit first in one group; otherwise new, then earlier
};

// LeadList is the leads as one rounded list with thin dividers (M7-00): the whole row
// selects the lead, the title is a real link to the posting (it doesn't select).
// Posting text and model output are rendered only as React text.
export function LeadList(props: Props) {
  const { leads, hidden, now = Date.now(), selected = -1, onSelect, rowRef, byScore = false } = props;
  const fresh = leads.filter((l) => isNew(l, now));
  const groups: [string, Lead[]][] = byScore
    ? [["Best fit first", leads]]
    : [
        [`New · last 24 h · ${fresh.length}`, fresh],
        ["Earlier", leads.filter((l) => !isNew(l, now))],
      ];
  let index = -1;
  return (
    <div className="lead-list">
      {groups.map(([title, rows]) =>
        rows.length === 0 ? null : (
          <section key={title} aria-label={title}>
            <h2 className="group">{title}</h2>
            <ul className="rows" aria-label={hidden ? "Leads hidden by the current filters" : "Leads"}>
              {rows.map((l) => {
                index++;
                const i = index;
                const on = i === selected;
                return (
                  <li
                    key={l.id}
                    ref={rowRef?.(i)}
                    className={`lead-row${on ? " selected" : ""}`}
                    tabIndex={on || (selected < 0 && i === 0) ? 0 : -1}
                    aria-current={on ? "true" : undefined}
                    data-lead={l.id}
                    onClick={() => onSelect?.(i)}
                  >
                    <div className="who">
                      <p className="company" translate="no">
                        {isNew(l, now) && (
                          <span className="new-dot" title="New in the last 24 hours">
                            <span className="visually-hidden">New: </span>
                          </span>
                        )}
                        {l.company.name}
                      </p>
                      <p className="title" onClick={(e) => e.stopPropagation()}>
                        <ExternalLink href={l.url}>{l.title}</ExternalLink>
                      </p>
                      <p className="meta">
                        <span className="place">{place(l)}</span>
                        {l.members > 1 && <> · {num.format(l.members)} postings</>}
                        {l.experience_min_years !== null && <> · {l.experience_min_years}+ years</>}
                        {l.closed && <span className="reasons"> · closed</span>}
                        {hidden && l.hidden_by && <span className="reasons"> · hidden by {l.hidden_by.map(reason).join(", ")}</span>}
                      </p>
                    </div>
                    <div className="side">
                      <FitTag lead={l} />
                      <time className="when" dateTime={l.first_seen_at} title={`First seen ${dateFmt.format(new Date(l.first_seen_at))}`}>
                        {ago(l.first_seen_at, now)}
                      </time>
                    </div>
                  </li>
                );
              })}
            </ul>
          </section>
        ),
      )}
    </div>
  );
}
