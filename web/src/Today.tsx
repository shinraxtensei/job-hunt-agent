import { useCallback, useEffect, useState, type MouseEvent } from "react";
import { get, post, type Lead } from "./api";
import type { DraftSummary } from "./Drafts";
import { FollowupCards, useFollowups } from "./Followups";
import { ago, inApp, leadHref, num, place } from "./format";
import { FitTag } from "./LeadList";
import { ExternalLink } from "./safe";

type Card = { lead_id: number; status: string; status_at: string };
type TodayData = { since: string; new_total: number; new: Lead[]; closed: (Lead & { closed_at: string })[] };
const sinceFmt = new Intl.DateTimeFormat(undefined, { weekday: "long", hour: "2-digit", minute: "2-digit" });
const WEEK = 7 * 24 * 3600e3;
const SHOW = 8;

// Today (M7-00, M7-01): what changed since your last visit (OI-20). Opening it records the
// visit (an ADR-0022 write), then it shows stat tiles, the new leads (best fit first),
// follow-ups due and closed postings of leads you acted on.
export function Today(props: { now: number; onOpen: (lead: number) => void; go: (page: string) => (e: MouseEvent) => void }) {
  const { now, onOpen, go } = props;
  const [data, setData] = useState<TodayData | null>(null);
  const [cards, setCards] = useState<Card[]>([]);
  const [drafts, setDrafts] = useState<DraftSummary[]>([]);
  const [error, setError] = useState<string | null>(null);
  const followups = useFollowups();

  const load = useCallback(
    () =>
      get<TodayData>("/api/today").then(setData, (e: unknown) => setError(e instanceof Error ? e.message : String(e))),
    [],
  );
  useEffect(() => {
    // record the visit first, so "since" is the visit before this one
    post("/api/visits").then(load, load);
    get<{ cards: Card[] }>("/api/board").then(
      (r) => setCards(r.cards),
      () => setCards([]),
    );
    get<{ drafts: DraftSummary[] }>("/api/drafts").then(
      (r) => setDrafts(r.drafts),
      () => setDrafts([]),
    );
  }, [load]);
  const markSeen = () => {
    post("/api/visits/seen").then(load, (e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };

  const fresh = data?.new ?? [];
  const closed = data?.closed ?? [];
  const newTotal = data?.new_total ?? 0;
  const applied = cards.filter((c) => c.status === "applied" && now - Date.parse(c.status_at) < WEEK).length;
  const ready = drafts.filter((d) => d.status === "ready_for_review").length;
  const due = followups.items?.length ?? 0;
  const tiles: [string, number, string, string, string][] = [
    ["New since your last visit", newTotal, "shown by your filters", "mint", "leads"],
    ["Follow-ups due", due, "7+ days after applying", "sand", "followups"],
    ["Applied this week", applied, "in the last 7 days", "blue", "board"],
    ["Drafts ready", ready, "waiting for your review", "lilac", "drafts"],
  ];

  return (
    <div className="page today">
      <h1>Today</h1>
      <p className="page-sub">
        {data ? <>Since {sinceFmt.format(new Date(data.since))} · </> : "Loading… "}
        {data && (
          <button type="button" className="link-button" onClick={markSeen}>
            Mark all seen
          </button>
        )}
      </p>
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
              <span className="tile-number">{data || page !== "leads" ? num.format(n) : "…"}</span>
              <span className="tile-note">{sub}</span>
            </a>
          </li>
        ))}
      </ul>
      <div className="today-cols">
        <section aria-labelledby="today-new">
          <h2 id="today-new">
            New leads <span className="count">{newTotal}</span>
          </h2>
          {data === null ? (
            <p role="status">Loading…</p>
          ) : fresh.length === 0 ? (
            <p className="empty">Nothing new since your last visit. Run make collect to look for more.</p>
          ) : (
            <>
              <ul className="cards">
                {fresh.slice(0, SHOW).map((l) => (
                  <LeadCard key={l.id} lead={l} now={now} onOpen={onOpen} />
                ))}
              </ul>
              {newTotal > SHOW && (
                <p className="more">
                  <a href="?page=leads&sort=date" onClick={go("leads&sort=date")}>
                    See all {num.format(newTotal)} newest first
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
              Closed since your last visit <span className="count">{closed.length}</span>
            </h2>
            {closed.length === 0 ? (
              <p className="empty">None of the leads you acted on closed.</p>
            ) : (
              <ul className="cards">
                {closed.slice(0, SHOW).map((l) => (
                  <LeadCard key={l.id} lead={l} now={now} onOpen={onOpen} closedAt={l.closed_at} />
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
function LeadCard({ lead: l, now, onOpen, closedAt }: { lead: Lead; now: number; onOpen: (lead: number) => void; closedAt?: string }) {
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
          {closedAt ? <span className="tag salmon">Closed</span> : <FitTag lead={l} />}
          <p className="when">{closedAt ? `closed ${ago(closedAt, now)}` : ago(l.first_seen_at, now)}</p>
        </div>
      </div>
    </li>
  );
}
