import { useState, type Ref } from "react";
import { get, type Lead, type Members } from "./api";
import { Icon } from "./icons";
import { ExternalLink } from "./safe";

const DAY = 24 * 3600e3;
const dateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" });
const num = new Intl.NumberFormat();

// "New" is the API's meaning: first seen in the last 24 hours.
export const isNew = (l: Lead, now: number) => now - Date.parse(l.first_seen_at) < DAY;

// age is a compact relative age ("3h", "2d") for the right-hand column.
export function age(iso: string, now: number): string {
  const h = Math.max(0, Math.floor((now - Date.parse(iso)) / 3600e3));
  if (h < 1) return "now";
  if (h < 48) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

export function place(l: Lead): string {
  const [first, ...rest] = l.locations;
  const loc = first ? first + (rest.length ? ` +${rest.length}` : "") : "No location";
  const wp = l.workplaces.filter((w) => w !== "unknown").join(", ");
  return wp ? `${loc} · ${wp}` : loc;
}

type Props = {
  leads: Lead[];
  hidden: boolean;
  now?: number;
  focus?: number;
  onFocus?: (i: number) => void;
  expanded?: Set<number>;
  onToggle?: (id: number) => void;
  rowRef?: (i: number) => Ref<HTMLTableRowElement>;
};

// LeadTable is a real table: one row per lead, grouped into new and earlier. Posting text
// is rendered only as React text.
export function LeadTable(props: Props) {
  const { leads, hidden, now = Date.now(), focus = -1, onFocus, onToggle, rowRef } = props;
  const [ownExpanded, setOwnExpanded] = useState<Set<number>>(new Set());
  const expanded = props.expanded ?? ownExpanded;
  const toggle =
    onToggle ??
    ((id: number) =>
      setOwnExpanded((s) => {
        const n = new Set(s);
        if (n.has(id)) n.delete(id);
        else n.add(id);
        return n;
      }));
  const fresh = leads.filter((l) => isNew(l, now));
  const groups: [string, Lead[]][] = [
    [`New · last 24 h · ${fresh.length}`, fresh],
    ["Earlier", leads.filter((l) => !isNew(l, now))],
  ];
  let index = -1;
  return (
    <table className="leads">
      <caption className="visually-hidden">{hidden ? "Leads hidden by the current filters" : "Leads"}</caption>
      <colgroup>
        <col className="c-dot" />
        <col className="c-company" />
        <col />
        <col className="c-place" />
        <col className="c-num" />
        <col className="c-num" />
        <col className="c-age" />
        <col className="c-more" />
      </colgroup>
      <thead>
        <tr>
          <th scope="col">
            <span className="visually-hidden">New</span>
          </th>
          <th scope="col">Company</th>
          <th scope="col">Title</th>
          <th scope="col">Location</th>
          <th scope="col" className="num">
            Postings
          </th>
          <th scope="col" className="num">
            Min years
          </th>
          <th scope="col" className="num">
            Age
          </th>
          <th scope="col">
            <span className="visually-hidden">Details</span>
          </th>
        </tr>
      </thead>
      {groups.map(([label, rows]) =>
        rows.length === 0 ? null : (
          <tbody key={label}>
            <tr>
              <th scope="rowgroup" colSpan={8} className="group">
                {label}
              </th>
            </tr>
            {rows.map((l) => {
              index++;
              const i = index;
              return (
                <LeadRow
                  key={l.id}
                  lead={l}
                  hidden={hidden}
                  now={now}
                  focused={i === focus}
                  open={expanded.has(l.id)}
                  onFocus={() => onFocus?.(i)}
                  onToggle={() => toggle(l.id)}
                  rowRef={rowRef?.(i)}
                />
              );
            })}
          </tbody>
        ),
      )}
    </table>
  );
}

function LeadRow(props: {
  lead: Lead;
  hidden: boolean;
  now: number;
  focused: boolean;
  open: boolean;
  onFocus: () => void;
  onToggle: () => void;
  rowRef?: Ref<HTMLTableRowElement>;
}) {
  const { lead: l, hidden, now, focused, open, onFocus, onToggle, rowRef } = props;
  const [members, setMembers] = useState<Members | null>(null);
  const [failed, setFailed] = useState(false);
  const detail = `detail-${l.id}`;
  const expand = () => {
    onToggle();
    // members_url comes from our own API; follow it only when it is one of its paths.
    if (!members && l.members > 1 && l.members_url.startsWith("/api/")) {
      get<Members>(l.members_url).then(setMembers, () => setFailed(true));
    }
  };
  return (
    <>
      <tr
        ref={rowRef}
        className={`row${focused ? " focused" : ""}${l.members > 1 ? "" : " single"}`}
        tabIndex={focused ? 0 : -1}
        onClick={onFocus}
        data-lead={l.id}
      >
        <td className="dot">
          {isNew(l, now) && (
            <span className="new-dot" title="New in the last 24 hours">
              <span className="visually-hidden">New</span>
            </span>
          )}
        </td>
        <td className="company" translate="no">
          {l.company.name}
        </td>
        <td className="title">
          <ExternalLink href={l.url}>{l.title}</ExternalLink>
          {l.closed && <span className="reasons">closed</span>}
          {hidden && l.hidden_by && <span className="reasons">hidden by {l.hidden_by.join(", ")}</span>}
        </td>
        <td className="place">{place(l)}</td>
        <td className="num">{l.members > 1 ? num.format(l.members) : ""}</td>
        <td className="num">{l.experience_min_years ?? "—"}</td>
        <td className="age">
          <time dateTime={l.first_seen_at} title={`First seen ${dateFmt.format(new Date(l.first_seen_at))}`}>
            {age(l.first_seen_at, now)}
          </time>
        </td>
        <td className="more">
          <button
            type="button"
            aria-expanded={open}
            aria-controls={detail}
            aria-label={l.members > 1 ? `Show ${l.members} postings` : "Show details"}
            onClick={(e) => {
              e.stopPropagation();
              onFocus();
              expand();
            }}
          >
            <Icon name="chevron" />
          </button>
        </td>
      </tr>
      {open && (
        <tr className="detail" id={detail}>
          <td colSpan={8}>
            <dl>
              <div className="pair">
                <dt>Postings</dt>
                <dd>{l.members}</dd>
              </div>
              <div className="pair">
                <dt>Min years</dt>
                <dd>{l.experience_min_years ?? "not stated"}</dd>
              </div>
              <div className="pair">
                <dt>Role</dt>
                <dd>
                  {l.role_family.replaceAll("_", " ")}, {l.seniority.replaceAll("_", " ")}
                </dd>
              </div>
              {l.published_at && (
                <div className="pair">
                  <dt>Published</dt>
                  <dd>{dateFmt.format(new Date(l.published_at))}</dd>
                </div>
              )}
            </dl>
            {l.members > 1 &&
              (members ? (
                <ul aria-label={`${l.members} postings of this role`}>
                  {members.members.map((m) => (
                    <li key={m.id}>
                      <ExternalLink href={m.url}>{m.title}</ExternalLink> · {m.locations.join("; ") || "No location"} ·{" "}
                      {[m.team, m.department].filter(Boolean).join(" / ") || "No team"} · first seen{" "}
                      {dateFmt.format(new Date(m.first_seen_at))}
                      {m.closed && " · closed"}
                    </li>
                  ))}
                </ul>
              ) : (
                <p role="status">{failed ? "Couldn’t load the postings. Collapse and expand to try again." : "Loading postings…"}</p>
              ))}
          </td>
        </tr>
      )}
    </>
  );
}
