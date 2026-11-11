import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { get, type Lead, type Members, type Status } from "./api";
import { HistoryPanel, StatusControl, useApplication } from "./Application";
import { DraftPanel } from "./Drafts";
import { ago, dateFmt, num, place } from "./format";
import { Icon } from "./icons";
import { label } from "./labels";
import { ContactsPanel, NotesPanel } from "./Notes";
import { ExternalLink } from "./safe";
import { FitTile, ScoreDetail, useFreshReason } from "./Score";

export const TABS = [
  ["overview", "Overview"],
  ["cv", "CV & letter"],
  ["notes", "Notes"],
  ["contacts", "Contacts"],
  ["history", "History"],
] as const;
export type Tab = (typeof TABS)[number][0];

// LeadDetail is the selected lead beside the list (M7-00): a header whose title is the
// posting's link, and tabs. On the phone it is a full-screen sheet with Back.
export function LeadDetail(props: { lead: Lead; now: number; tab: Tab; onTab: (t: Tab) => void; onBack: () => void; onStatus?: (s: Status) => void }) {
  const { now, tab, onTab, onBack, onStatus } = props;
  const [l, updating] = useFreshReason(props.lead, true);
  const { app, previous, load } = useApplication(l.id);
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const heading = useRef<HTMLHeadingElement>(null);

  // A newly selected lead's heading takes focus on the phone sheet (where it replaces the
  // list), so a screen reader starts at the top; on wide screens focus stays in the list.
  useEffect(() => {
    if (window.matchMedia("(max-width: 767px)").matches) heading.current?.focus();
  }, [l.id]);

  // Tabs follow the ARIA pattern: arrows move between tabs, Home/End jump.
  const onTabKey = (e: KeyboardEvent, i: number) => {
    const n = TABS.length;
    const to = e.key === "ArrowRight" ? (i + 1) % n : e.key === "ArrowLeft" ? (i - 1 + n) % n : e.key === "Home" ? 0 : e.key === "End" ? n - 1 : -1;
    if (to < 0) return;
    e.preventDefault();
    onTab(TABS[to]![0]);
    tabRefs.current[to]?.focus();
  };

  return (
    <article className="detail" aria-labelledby={`lead-title-${l.id}`}>
      <header className="detail-head">
        <button type="button" className="icon-button back" aria-label="Back to leads" onClick={onBack}>
          <Icon name="back" />
        </button>
        <div className="detail-titles">
          <p className="detail-company" translate="no">
            {l.company.name}
          </p>
          <h2 id={`lead-title-${l.id}`} ref={heading} tabIndex={-1}>
            <ExternalLink href={l.url}>{l.title}</ExternalLink>
          </h2>
          <p className="detail-meta">
            {place(l)} · first seen {ago(l.first_seen_at, now)}
            {l.closed && " · closed"}
          </p>
        </div>
      </header>
      <div className="tabs" role="tablist" aria-label="Lead sections">
        {TABS.map(([id, text], i) => (
          <button
            key={id}
            ref={(el) => {
              tabRefs.current[i] = el;
            }}
            type="button"
            role="tab"
            id={`tab-${id}`}
            aria-selected={tab === id}
            aria-controls={`panel-${id}`}
            tabIndex={tab === id ? 0 : -1}
            className="tab"
            onClick={() => onTab(id)}
            onKeyDown={(e) => onTabKey(e, i)}
          >
            {text}
          </button>
        ))}
      </div>
      <div className="tab-panel" role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`} tabIndex={0}>
        {tab === "overview" && (
          <>
            <div className="tile-row">
              <FitTile lead={l} />
              <StatusControl lead={l} app={app} reload={load} onStatus={onStatus} />
            </div>
            <ScoreDetail lead={l} updating={updating} />
            <Facts lead={l} />
          </>
        )}
        {tab === "cv" && <DraftPanel lead={l} />}
        {tab === "notes" && (app === undefined ? <p role="status">Loading…</p> : <NotesPanel leadId={l.id} notes={app?.notes ?? []} reload={load} />)}
        {tab === "contacts" && (app === undefined ? <p role="status">Loading…</p> : <ContactsPanel leadId={l.id} contacts={app?.contacts ?? []} reload={load} />)}
        {tab === "history" && <HistoryPanel app={app} previous={previous} />}
      </div>
    </article>
  );
}

// Facts: what the posting says beyond the fit, and every posting of the role.
function Facts({ lead: l }: { lead: Lead }) {
  const [members, setMembers] = useState<Members | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    setMembers(null);
    setFailed(false);
    // members_url comes from our own API; follow it only when it is one of its paths.
    if (l.members > 1 && l.members_url.startsWith("/api/")) get<Members>(l.members_url).then(setMembers, () => setFailed(true));
  }, [l.id, l.members, l.members_url]);
  return (
    <section className="facts" aria-label="About the role">
      <dl>
        <div>
          <dt>Role</dt>
          <dd>
            {label("role_family", l.role_family)}, {label("seniority", l.seniority)}
          </dd>
        </div>
        <div>
          <dt>Min years</dt>
          <dd>{l.experience_min_years ?? "not stated"}</dd>
        </div>
        <div>
          <dt>Postings</dt>
          <dd>{num.format(l.members)}</dd>
        </div>
        {l.published_at && (
          <div>
            <dt>Published</dt>
            <dd>{dateFmt.format(new Date(l.published_at))}</dd>
          </div>
        )}
      </dl>
      {l.members > 1 &&
        (members ? (
          <ul className="members" aria-label={`${l.members} postings of this role`}>
            {members.members.map((m) => (
              <li key={m.id}>
                <ExternalLink href={m.url}>{m.title}</ExternalLink> · {m.locations.join("; ") || "No location"} · {[m.team, m.department].filter(Boolean).join(" / ") || "No team"} · first seen{" "}
                {dateFmt.format(new Date(m.first_seen_at))}
                {m.closed && " · closed"}
              </li>
            ))}
          </ul>
        ) : (
          <p role="status" className="quiet-note">
            {failed ? "Couldn’t load the postings." : "Loading postings…"}
          </p>
        ))}
    </section>
  );
}
