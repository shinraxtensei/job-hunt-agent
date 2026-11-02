import { useCallback, useEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import { apiQuery, get, type Facets, type Lead, type Page, type Presets } from "./api";
import { FilterSheet } from "./FilterSheet";
import { applied } from "./filters";
import { Icon } from "./icons";
import { isNew, LeadTable } from "./LeadTable";
import { readPref, writePref } from "./prefs";
import { safeHref } from "./safe";

export { LeadTable } from "./LeadTable";

const num = new Intl.NumberFormat();
// The shown view loads every lead (a few hundred) so search covers all of them; the
// hidden view (thousands) pages.
const SHOWN_PAGE = 200;
const SHOWN_MAX_PAGES = 5;
const HIDDEN_PAGE = 100;

type Theme = "system" | "light" | "dark";
type Data = { leads: Lead[]; total: number; cursor?: string; facets: Facets; otherTotal: number };

const current = () => new URLSearchParams(window.location.search);
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

// useNarrow follows the phone layout breakpoint (styles/app.css, 640px).
function useNarrow(): boolean {
  const query = "(max-width: 640px)";
  const [narrow, setNarrow] = useState(() => window.matchMedia(query).matches);
  useEffect(() => {
    const mq = window.matchMedia(query);
    const on = () => setNarrow(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return narrow;
}

function withParam(params: URLSearchParams, k: string, v: string | null): URLSearchParams {
  const next = new URLSearchParams(params);
  if (v === null || v === "") next.delete(k);
  else next.set(k, v);
  return next;
}

export function App() {
  const [params, setParams] = useState(current);
  const [data, setData] = useState<Data | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [presets, setPresets] = useState<Presets["presets"]>([]);
  const [query, setQueryState] = useState(() => current().get("q") ?? "");
  // The search is in the URL too (q), replaced rather than pushed per keystroke.
  const setQuery = useCallback((q: string) => {
    setQueryState(q);
    const next = withParam(current(), "q", q || null);
    const s = next.toString();
    window.history.replaceState(null, "", s ? `?${s}` : window.location.pathname);
  }, []);
  const [focus, setFocus] = useState(-1);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [sheet, setSheet] = useState(false);
  const [help, setHelp] = useState(false);
  const [rail, setRail] = useState(() => {
    const saved = readPref("sidebar");
    return saved ? saved === "rail" : window.innerWidth < 1024;
  });
  const [theme, setTheme] = useState<Theme>(() => (readPref("theme") as Theme | null) ?? "system");
  const search = useRef<HTMLInputElement>(null);
  const narrow = useNarrow();
  const rows = useRef<(HTMLTableRowElement | null)[]>([]);
  const helpRef = useRef<HTMLDialogElement>(null);

  const hidden = params.get("view") === "hidden";
  const key = withParam(params, "q", null).toString(); // q is searched here, not by the API
  const now = useMemo(() => (data ? Date.now() : 0), [data]); // "now" is when the data arrived

  // Filter state lives in the URL only: every change is a history entry.
  const navigate = useCallback((to: URLSearchParams) => {
    const next = withParam(to, "q", current().get("q")); // filters keep the current search
    const s = next.toString();
    window.history.pushState(null, "", s ? `?${s}` : window.location.pathname);
    setParams(next);
  }, []);
  const set = (k: string, v: string | null) => navigate(withParam(params, k, v));
  const reset = () => navigate(new URLSearchParams(hidden ? "view=hidden" : ""));
  const follow = (to: URLSearchParams) => (e: MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    navigate(to);
  };

  useEffect(() => {
    const back = () => {
      setParams(current());
      setQueryState(current().get("q") ?? "");
    };
    window.addEventListener("popstate", back);
    return () => window.removeEventListener("popstate", back);
  }, []);

  useEffect(() => {
    get<Presets>("/api/presets").then(
      (p) => setPresets(p.presets),
      () => setPresets([]),
    );
  }, []);

  useEffect(() => {
    if (theme === "system") document.documentElement.removeAttribute("data-theme");
    else document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);

  useEffect(() => {
    const ctl = new AbortController();
    const p = new URLSearchParams(key);
    const view = p.get("view") === "hidden";
    setData(null);
    setError(null);
    setFocus(-1);
    setExpanded(new Set());
    (async () => {
      const path = `/api/leads${view ? "/hidden" : ""}`;
      const first = get<Page>(`${path}${apiQuery(p, { limit: String(view ? HIDDEN_PAGE : SHOWN_PAGE) })}`, ctl.signal);
      const [page, facets, other] = await Promise.all([
        first,
        get<Facets>(`/api/facets${apiQuery(p)}`, ctl.signal),
        get<Page>(`/api/leads${view ? "" : "/hidden"}${apiQuery(p, { limit: "1" })}`, ctl.signal),
      ]);
      let leads = page.leads;
      let cursor = page.next_cursor;
      for (let n = 1; !view && cursor && n < SHOWN_MAX_PAGES; n++) {
        const more = await get<Page>(`${path}${apiQuery(p, { limit: String(SHOWN_PAGE), cursor })}`, ctl.signal);
        leads = [...leads, ...more.leads];
        cursor = more.next_cursor;
      }
      setData({ leads, total: page.total, cursor, facets, otherTotal: other.total });
    })().catch((e: unknown) => {
      if (!ctl.signal.aborted) setError(message(e));
    });
    return () => ctl.abort();
  }, [key, attempt]);

  const loadMore = () => {
    if (!data?.cursor) return;
    const limit = String(hidden ? HIDDEN_PAGE : SHOWN_PAGE);
    get<Page>(`/api/leads${hidden ? "/hidden" : ""}${apiQuery(params, { limit, cursor: data.cursor })}`).then(
      (p) => setData((d) => d && { ...d, leads: [...d.leads, ...p.leads], cursor: p.next_cursor }),
      (e: unknown) => setError(message(e)),
    );
  };

  const presetName = params.get("preset") ?? "default";
  const preset = presets.find((p) => p.id === presetName)?.preset;
  const chips = applied(params, preset);

  const q = query.trim().toLowerCase();
  const visible = useMemo(() => {
    const leads = data?.leads ?? [];
    if (!q) return leads;
    return leads.filter((l) => [l.title, l.company.name, ...l.locations].some((s) => s.toLowerCase().includes(q)));
  }, [data, q]);
  // Rows render new first, then earlier: keyboard order follows the screen.
  const ordered = useMemo(() => [...visible.filter((l) => isNew(l, now)), ...visible.filter((l) => !isNew(l, now))], [visible, now]);

  const moveFocus = useCallback(
    (i: number) => {
      if (ordered.length === 0) return;
      const next = Math.max(0, Math.min(ordered.length - 1, i));
      setFocus(next);
      rows.current[next]?.focus();
      rows.current[next]?.scrollIntoView({ block: "nearest" });
    },
    [ordered.length],
  );
  const openLead = useCallback(
    (i: number) => {
      const href = ordered[i] && safeHref(ordered[i].url);
      if (href) window.open(href, "_blank", "noopener,noreferrer");
    },
    [ordered],
  );
  const toggle = useCallback((id: number) => {
    setExpanded((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  }, []);
  const toggleRail = useCallback(() => {
    setRail((r) => {
      writePref("sidebar", r ? "full" : "rail");
      return !r;
    });
  }, []);

  // Keyboard: j/k move, o or Enter opens, e expands, / searches, Esc clears, ? helps,
  // [ toggles the sidebar. Dialogs handle their own keys (Esc closes them).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || document.querySelector("dialog[open]")) return;
      const t = e.target as HTMLElement | null;
      const typing = !!t?.closest("input, select, textarea");
      if (e.key === "Escape") {
        if (t === search.current) {
          if (query) setQuery("");
          else search.current?.blur();
          e.preventDefault();
        } else if (focus >= 0 && ordered[focus] && expanded.has(ordered[focus].id)) {
          toggle(ordered[focus].id);
        }
        return;
      }
      if (typing) return;
      const onRowOrPage = !t || t === document.body || !!t.closest("tr.row");
      switch (e.key) {
        case "/":
          e.preventDefault();
          search.current?.focus();
          break;
        case "j":
        case "ArrowDown":
          if (e.key === "ArrowDown" && !onRowOrPage) return;
          e.preventDefault();
          moveFocus(focus + 1);
          break;
        case "k":
        case "ArrowUp":
          if (e.key === "ArrowUp" && !onRowOrPage) return;
          e.preventDefault();
          moveFocus(focus - 1);
          break;
        case "o":
        case "Enter":
          if (!onRowOrPage || focus < 0) return;
          e.preventDefault();
          openLead(focus);
          break;
        case "e":
          if (focus >= 0 && ordered[focus]) toggle(ordered[focus].id);
          break;
        case "?":
          setHelp(true);
          break;
        case "[":
          toggleRail();
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [focus, ordered, expanded, query, setQuery, moveFocus, openLead, toggle, toggleRail]);

  useEffect(() => {
    const d = helpRef.current;
    if (help && d && !d.open) d.showModal();
  }, [help]);

  const nextTheme: Record<Theme, Theme> = { system: "light", light: "dark", dark: "system" };
  const view = (v: string | null) => withParam(params, "view", v);
  const fresh = data && !hidden ? data.leads.filter((l) => isNew(l, now)).length : null;

  return (
    <div className={`app${rail ? " rail" : ""}`}>
      <a className="skip" href="#leads">
        Skip to leads
      </a>
      <nav className="sidebar" aria-label="Views">
        <div className="sidebar-head">
          <h1>Leads</h1>
          <button
            type="button"
            className="icon-button"
            aria-label={rail ? "Expand sidebar ([)" : "Collapse sidebar ([)"}
            aria-expanded={!rail}
            onClick={toggleRail}
          >
            <Icon name="rail" />
          </button>
        </div>
        <div className="nav">
          <a href={`?${view(null)}`} onClick={follow(view(null))} aria-current={!hidden ? "page" : undefined} aria-label={rail ? "Shown" : undefined}>
            <Icon name="inbox" />
            <span className="nav-label">Shown</span>
            <span className="nav-count">{data && !hidden ? num.format(data.total) : data ? num.format(data.otherTotal) : ""}</span>
          </a>
          <a
            href={`?${withParam(view(null), "new", "true")}`}
            onClick={follow(withParam(view(null), "new", "true"))}
            aria-label={rail ? "New in the last 24 hours" : undefined}
          >
            <Icon name="new" />
            <span className="nav-label">New</span>
            <span className="nav-count">{fresh ?? ""}</span>
          </a>
          <a
            href={`?${view("hidden")}`}
            onClick={follow(view("hidden"))}
            aria-current={hidden ? "page" : undefined}
            aria-label={rail ? "Hidden by filters" : undefined}
          >
            <Icon name="hidden" />
            <span className="nav-label">Hidden by filters</span>
            <span className="nav-count">{data ? num.format(hidden ? data.total : data.otherTotal) : ""}</span>
          </a>
          <div className="nav-section">Presets</div>
          {[...presets.filter((p) => p.preset).map((p) => p.id), "none"].map((id) => {
            const to = withParam(new URLSearchParams(hidden ? "view=hidden" : ""), "preset", id === "default" ? null : id);
            const label = id === "default" ? "My defaults" : id === "none" ? "No filters" : id;
            return (
              <a key={id} href={`?${to}`} onClick={follow(to)} aria-current={presetName === id ? "true" : undefined} aria-label={rail ? label : undefined}>
                <Icon name="preset" />
                <span className="nav-label">{label}</span>
              </a>
            );
          })}
        </div>
      </nav>

      <main id="leads">
        <div className="topbar">
          <div className="search">
            <label htmlFor="search" className="visually-hidden">
              Search {hidden ? "loaded hidden leads" : "leads"}
            </label>
            <input
              id="search"
              ref={search}
              type="search"
              name="search"
              autoComplete="off"
              spellCheck={false}
              placeholder={narrow ? "Search" : "Search titles, companies, places…"}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <kbd aria-hidden="true">/</kbd>
          </div>
          <button type="button" className="button" onClick={() => setSheet(true)} aria-haspopup="dialog">
            <Icon name="filter" /> Filters <span className="count">({chips.length})</span>
          </button>
          <button
            type="button"
            className="icon-button"
            onClick={() => setTheme(nextTheme[theme])}
            aria-label={`Theme: ${theme}. Switch to ${nextTheme[theme]}`}
          >
            <Icon name={theme === "dark" ? "moon" : theme === "light" ? "sun" : "system"} />
          </button>
          <button type="button" className="icon-button hide-narrow" onClick={() => setHelp(true)} aria-label="Keyboard shortcuts (?)">
            ?
          </button>
        </div>

        <div className="chips" role="group" aria-label="Applied filters">
          {chips.map((c) =>
            c.fromURL ? (
              <span className="chip" key={c.key}>
                {c.label} <b>{c.value}</b>
                <button type="button" aria-label={`Remove ${c.label} filter`} onClick={() => set(c.key, null)}>
                  <Icon name="close" />
                </button>
              </span>
            ) : (
              <span className="chip static" key={c.key}>
                {c.label} <b>{c.value}</b>
              </span>
            ),
          )}
          {params.toString() && params.toString() !== "view=hidden" && (
            <button type="button" className="link-button" onClick={reset}>
              Reset to my defaults
            </button>
          )}
        </div>

        <Body
          data={data}
          error={error}
          hidden={hidden}
          query={query}
          visible={ordered}
          now={now}
          focus={focus}
          setFocus={setFocus}
          expanded={expanded}
          toggle={toggle}
          rows={rows}
          retry={() => setAttempt((a) => a + 1)}
          reset={reset}
          showHidden={follow(view("hidden"))}
          showShown={follow(view(null))}
          hiddenHref={`?${view("hidden")}`}
          shownHref={`?${view(null)}`}
          clearSearch={() => setQuery("")}
          loadMore={loadMore}
        />
      </main>

      <FilterSheet
        open={sheet}
        onClose={() => setSheet(false)}
        params={params}
        preset={preset}
        presets={presets}
        facets={data?.facets}
        set={set}
        reset={reset}
      />
      <dialog ref={helpRef} className="shortcuts" aria-labelledby="help-title" onClose={() => setHelp(false)}>
        <h2 id="help-title">Keyboard shortcuts</h2>
        <dl>
          <dt><kbd>j</kbd> <kbd>k</kbd></dt>
          <dd>Next and previous lead</dd>
          <dt><kbd>o</kbd> <kbd>Enter</kbd></dt>
          <dd>Open the posting in a new tab</dd>
          <dt><kbd>e</kbd></dt>
          <dd>Show or hide the lead's details and postings</dd>
          <dt><kbd>/</kbd></dt>
          <dd>Search</dd>
          <dt><kbd>Esc</kbd></dt>
          <dd>Clear the search, close details or a panel</dd>
          <dt><kbd>[</kbd></dt>
          <dd>Collapse or expand the sidebar</dd>
          <dt><kbd>?</kbd></dt>
          <dd>This list</dd>
        </dl>
        <form method="dialog">
          <button type="submit" className="button">
            Close
          </button>
        </form>
      </dialog>
    </div>
  );
}

function Body(props: {
  data: Data | null;
  error: string | null;
  hidden: boolean;
  query: string;
  visible: Lead[];
  now: number;
  focus: number;
  setFocus: (i: number) => void;
  expanded: Set<number>;
  toggle: (id: number) => void;
  rows: React.RefObject<(HTMLTableRowElement | null)[]>;
  retry: () => void;
  reset: () => void;
  showHidden: (e: MouseEvent) => void;
  showShown: (e: MouseEvent) => void;
  hiddenHref: string;
  shownHref: string;
  clearSearch: () => void;
  loadMore: () => void;
}) {
  const { data, error, hidden, query, visible } = props;
  if (error) {
    return (
      <div className="notice" role="alert">
        <p>
          <strong>Couldn’t load leads.</strong> The API answered: {error}
        </p>
        <p>Check that the API is running (go run ./cmd/api), then try again.</p>
        <button type="button" className="button" onClick={props.retry}>
          Try again
        </button>
      </div>
    );
  }
  if (!data) {
    return (
      <p className="summary" role="status" aria-busy="true">
        Loading leads…
      </p>
    );
  }
  const summary = hidden ? (
    <p className="summary" role="status">
      {num.format(data.total)} hidden by these filters ·{" "}
      <a href={props.shownHref} onClick={props.showShown}>
        {num.format(data.otherTotal)} shown
      </a>
    </p>
  ) : (
    <p className="summary" role="status">
      {num.format(data.total)} shown ·{" "}
      <a href={props.hiddenHref} onClick={props.showHidden}>
        {num.format(data.otherTotal)} hidden by these filters
      </a>
    </p>
  );
  if (data.total === 0) {
    const everythingHidden = !hidden && data.otherTotal > 0;
    return (
      <>
        {summary}
        <div className="notice">
          {everythingHidden ? (
            <p>
              <strong>Your filters hide all {num.format(data.otherTotal)} leads.</strong>{" "}
              <a href={props.hiddenHref} onClick={props.showHidden}>
                See what they hide and why
              </a>
              , or widen a filter.
            </p>
          ) : hidden ? (
            <p>
              <strong>These filters hide nothing.</strong> Every lead is in the shown list.
            </p>
          ) : (
            <p>
              <strong>No leads yet.</strong> Run the collector (go run ./cmd/collector run) to fetch postings.
            </p>
          )}
          <button type="button" className="button" onClick={props.reset}>
            Reset to my defaults
          </button>
        </div>
      </>
    );
  }
  return (
    <>
      {summary}
      {visible.length === 0 ? (
        <div className="notice">
          <p>
            <strong>Nothing matches “{query}”</strong> in the {num.format(data.leads.length)} loaded leads.
          </p>
          <button type="button" className="button" onClick={props.clearSearch}>
            Clear search
          </button>
        </div>
      ) : (
        <LeadTable
          leads={visible}
          hidden={hidden}
          now={props.now}
          focus={props.focus}
          onFocus={props.setFocus}
          expanded={props.expanded}
          onToggle={props.toggle}
          rowRef={(i) => (el) => {
            props.rows.current[i] = el;
          }}
        />
      )}
      {data.cursor && (
        <button type="button" className="button load-more" onClick={props.loadMore}>
          Load {hidden ? "more hidden leads" : "more"} ({num.format(data.total - data.leads.length)} left)
        </button>
      )}
    </>
  );
}
