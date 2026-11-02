import { useCallback, useEffect, useState } from "react";
import { apiQuery, get, type Facets, type Lead, type Members, type Page, type Preset, type Presets } from "./api";
import { ExternalLink } from "./safe";

// Facets shown as checkbox groups, in the order the API documents them.
const FACETS: [key: string, label: string][] = [
  ["role_family", "Role family"],
  ["seniority", "Seniority"],
  ["workplace", "Workplace"],
  ["remote_scope", "Remote scope"],
  ["country", "Country"],
  ["track", "Track"],
  ["company", "Company"],
];

const date = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" });
const fmt = (iso: string | null) => (iso ? date.format(new Date(iso)) : "—");

function current(): URLSearchParams {
  return new URLSearchParams(window.location.search);
}

type Data = { page: Page; facets: Facets; otherTotal: number };

export function App() {
  const [params, setParams] = useState(current);
  const [data, setData] = useState<Data | null>(null);
  const [more, setMore] = useState<Lead[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [error, setError] = useState<string | null>(null);
  const [presets, setPresets] = useState<Presets["presets"]>([]);
  const hidden = params.get("view") === "hidden";
  const key = params.toString();

  // Filter state lives in the URL only (AC2b): every change is a history entry.
  const navigate = useCallback((next: URLSearchParams) => {
    const s = next.toString();
    window.history.pushState(null, "", s ? `?${s}` : window.location.pathname);
    setParams(next);
  }, []);

  useEffect(() => {
    const back = () => setParams(current());
    window.addEventListener("popstate", back);
    return () => window.removeEventListener("popstate", back);
  }, []);

  useEffect(() => {
    get<Presets>("/api/presets").then((p) => setPresets(p.presets), () => setPresets([]));
  }, []);

  useEffect(() => {
    const ctl = new AbortController();
    const p = new URLSearchParams(key);
    const view = p.get("view") === "hidden";
    setData(null);
    setError(null);
    setMore([]);
    Promise.all([
      get<Page>(`/api/leads${view ? "/hidden" : ""}${apiQuery(p)}`, ctl.signal),
      get<Facets>(`/api/facets${apiQuery(p)}`, ctl.signal),
      get<Page>(`/api/leads${view ? "" : "/hidden"}${apiQuery(p, { limit: "1" })}`, ctl.signal),
    ]).then(
      ([page, facets, other]) => {
        setData({ page, facets, otherTotal: other.total });
        setCursor(page.next_cursor);
      },
      (e: unknown) => {
        if (!ctl.signal.aborted) setError(e instanceof Error ? e.message : String(e));
      },
    );
    return () => ctl.abort();
  }, [key]);

  const loadMore = () => {
    if (!cursor) return;
    get<Page>(`/api/leads${hidden ? "/hidden" : ""}${apiQuery(params, { cursor })}`).then(
      (p) => {
        setMore((m) => [...m, ...p.leads]);
        setCursor(p.next_cursor);
      },
      (e: unknown) => setError(e instanceof Error ? e.message : String(e)),
    );
  };

  const presetName = params.get("preset") ?? "default";
  const preset = presets.find((p) => p.id === presetName)?.preset;
  const set = (k: string, v: string | null) => {
    const next = new URLSearchParams(params);
    if (v === null || v === "") next.delete(k);
    else next.set(k, v);
    navigate(next);
  };

  return (
    <main>
      <h1>{hidden ? "Hidden leads" : "Leads"}</h1>
      <Filters params={params} preset={preset} presets={presets} facets={data?.facets} set={set} />
      <p>
        <button type="button" onClick={() => navigate(new URLSearchParams())}>
          Reset to my defaults
        </button>
      </p>
      {error && <p role="alert">Could not load leads: {error}</p>}
      {!data && !error && <p aria-live="polite">Loading…</p>}
      {data && (
        <>
          <p aria-live="polite">
            {hidden ? (
              <>
                {data.page.total} leads hidden by these filters.{" "}
                <a href={`?${withView(params, null)}`} onClick={follow(navigate, withView(params, null))}>
                  Show the {data.otherTotal} shown leads
                </a>
              </>
            ) : (
              <>
                {data.page.total} leads. These filters hide {data.otherTotal}.{" "}
                <a href={`?${withView(params, "hidden")}`} onClick={follow(navigate, withView(params, "hidden"))}>
                  See what is hidden, and why
                </a>
              </>
            )}
          </p>
          <LeadTable leads={[...data.page.leads, ...more]} hidden={hidden} />
          {cursor && (
            <button type="button" onClick={loadMore}>
              Load more
            </button>
          )}
        </>
      )}
    </main>
  );
}

function withView(params: URLSearchParams, view: string | null): URLSearchParams {
  const next = new URLSearchParams(params);
  if (view) next.set("view", view);
  else next.delete("view");
  return next;
}

// follow turns a same-page link into a pushState navigation, keeping the href for
// middle-click and copying.
function follow(navigate: (p: URLSearchParams) => void, to: URLSearchParams) {
  return (e: React.MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    navigate(to);
  };
}

function Filters(props: {
  params: URLSearchParams;
  preset: Preset | undefined;
  presets: Presets["presets"];
  facets: Facets | undefined;
  set: (k: string, v: string | null) => void;
}) {
  const { params, preset, presets, facets, set } = props;
  // A facet's selection is its URL parameter, or else what the preset sets for it.
  const selected = (k: string): string[] => {
    const v = params.get(k);
    if (v !== null) return v.split(",").filter(Boolean);
    if (k === "role_family") return preset?.role_family ?? [];
    if (k === "seniority") return preset?.seniority ?? [];
    return [];
  };
  return (
    <form aria-label="Filters" onSubmit={(e) => e.preventDefault()}>
      <label>
        Preset{" "}
        <select value={params.get("preset") ?? "default"} onChange={(e) => set("preset", e.target.value === "default" ? null : e.target.value)}>
          {presets.filter((p) => p.preset).map((p) => (
            <option key={p.id} value={p.id}>
              {p.id}
            </option>
          ))}
          <option value="none">none (no filters)</option>
        </select>
      </label>{" "}
      <label>
        Max experience (years){" "}
        <input
          type="number"
          min={0}
          max={20}
          inputMode="numeric"
          value={params.get("max_experience") ?? preset?.experience_max_years ?? ""}
          onChange={(e) => set("max_experience", e.target.value)}
        />
      </label>{" "}
      <label>
        Closed{" "}
        <select value={params.get("closed") ?? preset?.closed ?? ""} onChange={(e) => set("closed", e.target.value)}>
          <option value="">any</option>
          <option value="hide">hide</option>
          <option value="show">show</option>
          <option value="only">only</option>
        </select>
      </label>{" "}
      <label>
        <input type="checkbox" checked={params.get("new") === "true"} onChange={(e) => set("new", e.target.checked ? "true" : null)} /> New
        (last 24 h)
      </label>
      {FACETS.map(([k, label]) => {
        const sel = selected(k);
        const counts = facets?.facets[k] ?? {};
        const values = [...new Set([...Object.keys(counts), ...sel])].sort();
        return (
          <fieldset key={k}>
            <legend>{label}</legend>
            {values.map((v) => (
              <label key={v}>
                <input
                  type="checkbox"
                  checked={sel.includes(v)}
                  onChange={(e) => {
                    const next = e.target.checked ? [...sel, v] : sel.filter((s) => s !== v);
                    set(k, next.join(","));
                  }}
                />{" "}
                {v} ({counts[v] ?? 0}){" "}
              </label>
            ))}
          </fieldset>
        );
      })}
    </form>
  );
}

export function LeadTable({ leads, hidden }: { leads: Lead[]; hidden: boolean }) {
  if (leads.length === 0) return <p>No leads match these filters.</p>;
  return (
    <table>
      <thead>
        <tr>
          <th scope="col">Company</th>
          <th scope="col">Title</th>
          <th scope="col">Locations</th>
          <th scope="col">Workplace</th>
          <th scope="col">Published</th>
          <th scope="col">First seen</th>
          <th scope="col">Postings</th>
          {hidden && <th scope="col">Hidden by</th>}
        </tr>
      </thead>
      <tbody>
        {leads.map((l) => (
          <LeadRow key={l.id} lead={l} hidden={hidden} />
        ))}
      </tbody>
    </table>
  );
}

function LeadRow({ lead, hidden }: { lead: Lead; hidden: boolean }) {
  const [members, setMembers] = useState<Members | null>(null);
  const [open, setOpen] = useState(false);
  const toggle = () => {
    setOpen((o) => !o);
    // members_url comes from our own API; only follow it when it is one of its paths.
    if (!members && lead.members_url.startsWith("/api/")) get<Members>(lead.members_url).then(setMembers, () => setMembers(null));
  };
  const panel = `members-${lead.id}`;
  return (
    <>
      <tr>
        <td data-label="Company">{lead.company.name}</td>
        <td data-label="Title">
          <ExternalLink href={lead.url}>{lead.title}</ExternalLink>
          {lead.closed && " (closed)"}
        </td>
        <td data-label="Locations">{lead.locations.join("; ")}</td>
        <td data-label="Workplace">{lead.workplaces.join(", ")}</td>
        <td data-label="Published">{fmt(lead.published_at)}</td>
        <td data-label="First seen">{fmt(lead.first_seen_at)}</td>
        <td data-label="Postings">
          {lead.members > 1 && (
            <button type="button" aria-expanded={open} aria-controls={panel} onClick={toggle}>
              {lead.members} postings
            </button>
          )}
        </td>
        {hidden && <td data-label="Hidden by">{lead.hidden_by?.join(", ")}</td>}
      </tr>
      {open && (
        <tr id={panel}>
          <td colSpan={hidden ? 8 : 7}>
            {members ? (
              <ul>
                {members.members.map((m) => (
                  <li key={m.id}>
                    <ExternalLink href={m.url}>{m.title}</ExternalLink> · {[m.team, m.department].filter(Boolean).join(" / ") || "no team"} ·{" "}
                    {m.locations.join("; ") || "no location"} · {m.workplace} · first seen {fmt(m.first_seen_at)}
                    {m.closed && " · closed"}
                  </li>
                ))}
              </ul>
            ) : (
              <p>Loading postings…</p>
            )}
          </td>
        </tr>
      )}
    </>
  );
}
