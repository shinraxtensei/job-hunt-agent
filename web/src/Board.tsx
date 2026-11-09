import { useCallback, useEffect, useState, type DragEvent } from "react";
import { STATUSES, get, post, type Status } from "./api";
import { fitTone, inApp, leadHref, statusTone } from "./format";
import { label } from "./labels";

type Card = { lead_id: number; company: string; title: string; fit_level: string | null; status: Status; status_at: string; followup_due: boolean };

// OI-13: one column per active status; rejected, closed and skipped share "Done". New
// leads stay in the table.
const COLUMNS: { id: string; title: string; statuses: Status[]; drop: Status }[] = [
  ...(["shortlisted", "drafted", "applied", "replied", "interviewing", "offer"] as const).map((s) => ({ id: s, title: label("status", s), statuses: [s], drop: s })),
  { id: "done", title: "Done", statuses: ["rejected", "closed", "skipped"], drop: "closed" },
];
const days = (iso: string, now: number) => Math.max(0, Math.floor((now - new Date(iso).getTime()) / 86_400_000));

// Board: applications by status (M6-02). A card moves by drag and drop, or with its
// "Move to" menu (keyboard and touch); both go through the M6-01 status endpoint.
export function Board({ onOpen }: { onOpen?: (lead: number) => void }) {
  const [cards, setCards] = useState<Card[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [over, setOver] = useState<string | null>(null);
  const load = useCallback(
    () =>
      get<{ cards: Card[] }>("/api/board")
        .then((r) => setCards(r.cards))
        .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e))),
    [],
  );
  useEffect(() => {
    void load();
  }, [load]);
  const move = (lead: number, to: Status) => {
    setError(null);
    post(`/api/leads/${lead}/status`, { status: to })
      .then(() => load())
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };
  const drop = (col: (typeof COLUMNS)[number]) => (e: DragEvent) => {
    e.preventDefault();
    setOver(null);
    const lead = Number(e.dataTransfer.getData("text/plain"));
    const card = cards?.find((c) => c.lead_id === lead);
    if (card && !col.statuses.includes(card.status)) move(lead, col.drop);
  };
  if (!cards) return <p role="status">{error ?? "Loading the board…"}</p>;
  const now = Date.now();
  return (
    <div className="page board-wrap">
      <h1>Board</h1>
      <p className="page-sub">Your applications by status. Drag a card, or use its menu, to move it.</p>
      {error && (
        <p className="draft-error" role="alert">
          {error}
        </p>
      )}
      {cards.length === 0 && <p className="empty">No applications yet. Set a lead’s status in its details to put it here.</p>}
      <div className="board">
        {COLUMNS.map((col) => {
          const mine = cards.filter((c) => col.statuses.includes(c.status));
          return (
            <section
              key={col.id}
              className={`column${over === col.id ? " over" : ""}`}
              aria-labelledby={`col-${col.id}`}
              onDragOver={(e) => {
                e.preventDefault();
                setOver(col.id);
              }}
              onDragLeave={() => setOver(null)}
              onDrop={drop(col)}
            >
              <h2 id={`col-${col.id}`}>
                {col.title} <span className="count">{mine.length}</span>
              </h2>
              <ul className="plain">
                {mine.map((c) => (
                  <li key={c.lead_id} className="card" draggable onDragStart={(e) => e.dataTransfer.setData("text/plain", String(c.lead_id))}>
                    <p className="card-title">
                      <a href={leadHref(c.lead_id)} onClick={inApp(() => onOpen?.(c.lead_id))}>
                        {c.company} · {c.title}
                      </a>
                    </p>
                    <p className="card-tags">
                      {c.fit_level && <span className={`tag ${fitTone[c.fit_level] ?? "lilac"}`}>{label("fit", c.fit_level)} fit</span>}
                      {col.id === "done" && <span className={`tag ${statusTone[c.status] ?? "lilac"}`}>{label("status", c.status)}</span>}
                      {c.followup_due && <span className="tag sand due">Follow-up due</span>}
                    </p>
                    <p className="when">{days(c.status_at, now)} days here</p>
                    <label className="visually-hidden" htmlFor={`move-${c.lead_id}`}>
                      Move {c.company} {c.title} to
                    </label>
                    <select id={`move-${c.lead_id}`} value={c.status} onChange={(e) => move(c.lead_id, e.target.value as Status)}>
                      {STATUSES.map((s) => (
                        <option key={s} value={s}>
                          {label("status", s)}
                        </option>
                      ))}
                    </select>
                  </li>
                ))}
              </ul>
            </section>
          );
        })}
      </div>
    </div>
  );
}
