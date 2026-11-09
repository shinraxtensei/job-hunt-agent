import { useEffect, useState, type MouseEvent } from "react";
import { get, type Lead, type Page } from "./api";
import type { DraftSummary } from "./Drafts";
import { FollowupCards, useFollowups } from "./Followups";
import { ago, inApp, isNew, leadHref, num, place } from "./format";
import { FitTag } from "./LeadList";
import { ExternalLink } from "./safe";

type Card = { lead_id: number; status: string; status_at: string };
const WEEK = 7 * 24 * 3600e3;
const SHOW = 8;

// Today (M7-00, M7-01): what to do now. Stat tiles, then new leads (best fit first),
// follow-ups due, and closed postings of leads you acted on.
export function Today(props: { now: number; onOpen: (lead: number) => void; go: (page: string) => (e: MouseEvent) => void }) {
  const { now, onOpen, go } = props;
  const [leads, setLeads] = useState<Lead[] | null>(null);
  const [closed, setClosed] = useState<Lead[]>([]);
  const [cards, setCards] = useState<Card[]>([]);
  const [drafts, setDrafts] = useState<DraftSummary[]>([]);
  const [error, setError] = useState<string | null>(null);
  const followups = useFollowups();

  useEffect(() => {
    get<Page>("/api/leads?limit=200").then(
      (p) => setLeads(p.leads),
      (e: unknown) => setError(e instanceof Error ? e.message : String(e)),
    );
    get<Page>("/api/leads?preset=none&closed=only&sort=date&limit=50").then(
      (p) => setClosed(p.leads.filter((l) => l.status !== "new" && l.status !== "skipped")),
      () => setClosed([]),
    );
    get<{ cards: Card[] }>("/api/board").then(
      (r) => setCards(r.cards),
      () => setCards([]),
    );
    get<{ drafts: DraftSummary[] }>("/api/drafts").then(
      (r) => setDrafts(r.drafts),
      () => setDrafts([]),
    );
  }, []);

  const fresh = (leads ?? []).filter((l) => isNew(l, now));
  const applied = cards.filter((c) => c.status === "applied" && now - Date.parse(c.status_at) < WEEK).length;
  const ready = drafts.filter((d) => d.status === "ready_for_review").length;
  const due = followups.items?.length ?? 0;
  const tiles: [string, number, string, string, string][] = [
    ["New in the last 24 hours", fresh.length, "shown by your filters", "mint", "leads"],
    ["Follow-ups due", due, "7+ days after applying", "sand", "followups"],
    ["Applied this week", applied, "in the last 7 days", "blue", "board"],
    ["Drafts ready", ready, "waiting for your review", "lilac", "drafts"],
  ];

  return (
    <div className="page today">
      <h1>Today</h1>
      <p className="page-sub">New leads in the last 24 hours, follow-ups due, and postings that closed.</p>
      {error && (
        <p className="draft-error" role="alert">
          Couldn’t load leads. The API answered: {error}
        </p>
      )}
      <ul className="tiles" aria-label="At a glance">
        {tiles.map(([title, n, sub, tone, page]) => (
          <li key={title}>
            <a className={`tile ${tone}`} href={`?page=${page}`} onClick={go(page)}>
              <span className="tile-label">{title}</span>
              <span className="tile-number">{leads || page !== "leads" ? num.format(n) : "…"}</span>
              <span className="tile-note">{sub}</span>
            </a>
          </li>
        ))}
      </ul>
      <div className="today-cols">
        <section aria-labelledby="today-new">
          <h2 id="today-new">
            New leads <span className="count">{fresh.length}</span>
          </h2>
          {leads === null ? (
            <p role="status">Loading…</p>
          ) : fresh.length === 0 ? (
            <p className="empty">Nothing new in the last 24 hours. Run make collect to look for more.</p>
          ) : (
            <>
              <ul className="cards">
                {fresh.slice(0, SHOW).map((l) => (
                  <LeadCard key={l.id} lead={l} now={now} onOpen={onOpen} />
                ))}
              </ul>
              {fresh.length > SHOW && (
                <p className="more">
                  <a href="?page=leads&new=true" onClick={go("leads&new=true")}>
                    See all {num.format(fresh.length)} new leads
                  </a>
                </p>
              )}
            </>
          )}
        </section>
        <div>
          <section aria-labelledby="today-followups">
            <h2 id="today-followups">
              Follow-ups due <span className="count">{due}</span>
            </h2>
            {followups.items === null ? (
              <p role="status">Loading…</p>
            ) : due === 0 ? (
              <p className="empty">Nothing due.</p>
            ) : (
              <FollowupCards items={followups.items} reload={followups.load} />
            )}
          </section>
          <section aria-labelledby="today-closed">
            <h2 id="today-closed">
              Closed <span className="count">{closed.length}</span>
            </h2>
            {closed.length === 0 ? (
              <p className="empty">None of your leads closed.</p>
            ) : (
              <ul className="cards">
                {closed.slice(0, SHOW).map((l) => (
                  <LeadCard key={l.id} lead={l} now={now} onOpen={onOpen} closed />
                ))}
              </ul>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}

// LeadCard is a lead as a rounded card: the card opens the lead; the title is the posting.
function LeadCard({ lead: l, now, onOpen, closed = false }: { lead: Lead; now: number; onOpen: (lead: number) => void; closed?: boolean }) {
  return (
    <li className="card lead-card">
      <div className="card-main">
        <div>
          <p className="card-title">
            <a href={leadHref(l.id)} onClick={inApp(() => onOpen(l.id))} translate="no">
              {l.company.name}
            </a>
          </p>
          <p className="card-sub">
            <ExternalLink href={l.url}>{l.title}</ExternalLink>
          </p>
          <p className="quiet-note">{place(l)}</p>
        </div>
        <div className="side">
          {closed ? <span className="tag salmon">Closed</span> : <FitTag lead={l} />}
          <p className="when">{ago(l.first_seen_at, now)}</p>
        </div>
      </div>
    </li>
  );
}
