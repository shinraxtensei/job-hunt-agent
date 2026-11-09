import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import { apiQuery, get, type Facets, type Lead, type Page, type Presets, type Status } from "./api";
import { Board } from "./Board";
import { DraftsPage } from "./Drafts";
import { FilterSheet } from "./FilterSheet";
import { applied } from "./filters";
import { FollowupsPage } from "./Followups";
import { isNew, num } from "./format";
import { Icon } from "./icons";
import { LeadDetail, TABS, type Tab } from "./LeadDetail";
import { LeadList } from "./LeadList";
import { readPref, writePref } from "./prefs";
import { safeHref } from "./safe";
import { Today } from "./Today";

export { LeadList } from "./LeadList";

// The shown view loads every lead (a few hundred) so search covers all of them; the
// hidden view (thousands) pages.
const SHOWN_PAGE = 200;
const SHOWN_MAX_PAGES = 5;
const HIDDEN_PAGE = 100;

// Pill navigation (M7-00): one row, the active pill filled.
const PAGES = [
  ["today", "Today"],
  ["leads", "Leads"],
  ["board", "Board"],
  ["followups", "Follow-ups"],
  ["drafts", "Drafts"],
] as const;
type PageId = (typeof PAGES)[number][0];

type Theme = "system" | "light" | "dark";
type Data = { leads: Lead[]; total: number; cursor?: string; facets: Facets; otherTotal: number };

const current = () => new URLSearchParams(window.location.search);
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));
// The page's own state in the URL; filters are everything else.
const UI_KEYS = ["q", "page", "lead", "tab"];

// useNarrow follows the phone layout breakpoint (styles/app.css, 767px): the detail is a
// full-screen sheet there.
function useNarrow(): boolean {
  const query = "(max-width: 767px)";
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

function pageOf(params: URLSearchParams): PageId {
  if (params.get("view") === "board") return "board"; // M6-02 links
  const p = params.get("page");
  if (p && PAGES.some(([id]) => id === p)) return p as PageId;
  // a link with filters or a lead (from before M7-00) means the leads page
  return [...params.keys()].some((k) => k !== "page") ? "leads" : "today";
}

export function App() {
  const [params, setParams] = useState(current);
  const [data, setData] = useState<Data | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [presets, setPresets] = useState<Presets["presets"]>([]);
  const [query, setQueryState] = useState(() => current().get("q") ?? "");
  const [sheet, setSheet] = useState(false);
  const [help, setHelp] = useState(false);
  const [theme, setTheme] = useState<Theme>(() => (readPref("theme") as Theme | null) ?? "system");
  const search = useRef<HTMLInputElement>(null);
  const narrow = useNarrow();
  const rows = useRef<(HTMLLIElement | null)[]>([]);
  const helpRef = useRef<HTMLDialogElement>(null);
  const [loadedAt, setLoadedAt] = useState(() => Date.now());

  const page = pageOf(params);
  const hidden = params.get("view") === "hidden";
  const selectedId = Number(params.get("lead")) || null;
  const tab = (TABS.some(([id]) => id === params.get("tab")) ? params.get("tab") : "overview") as Tab;
  // the filters only: what the leads API is asked for
  const key = useMemo(() => {
    const p = new URLSearchParams(params);
    for (const k of UI_KEYS) p.delete(k);
    return p.toString();
  }, [params]);
  const now = useMemo(() => (data ? Date.now() : loadedAt), [data, loadedAt]); // when the data arrived

  // The search is in the URL too (q), replaced rather than pushed per keystroke.
  const setQuery = useCallback((q: string) => {
    setQueryState(q);
    const next = withParam(current(), "q", q || null);
    const s = next.toString();
    window.history.replaceState(null, "", s ? `?${s}` : window.location.pathname);
  }, []);

  // Navigation state lives in the URL only: a page or filter change is a history entry;
  // moving the selection replaces the entry (j/k shouldn't fill the history).
  const navigate = useCallback((to: URLSearchParams, replace = false) => {
    const next = withParam(to, "q", current().get("q")); // keep the current search
    const s = next.toString();
    window.history[replace ? "replaceState" : "pushState"](null, "", s ? `?${s}` : window.location.pathname);
    setParams(next);
  }, []);
  const set = (k: string, v: string | null) => navigate(withParam(withParam(params, k, v), "lead", null));
  const reset = () => navigate(new URLSearchParams(hidden ? "page=leads&view=hidden" : "page=leads"));
  const follow = (to: URLSearchParams) => (e: MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    navigate(to);
  };
  const goPage = (p: string) => follow(new URLSearchParams(`page=${p}`));
  const openLead = useCallback((id: number) => navigate(new URLSearchParams(`page=leads&lead=${id}`)), [navigate]);

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
    if (page !== "leads") return;
    const ctl = new AbortController();
    const p = new URLSearchParams(key);
    const view = p.get("view") === "hidden";
    setData(null);
    setError(null);
    (async () => {
      const path = `/api/leads${view ? "/hidden" : ""}`;
      const first = get<Page>(`${path}${apiQuery(p, { limit: String(view ? HIDDEN_PAGE : SHOWN_PAGE) })}`, ctl.signal);
      const [pg, facets, other] = await Promise.all([
        first,
        get<Facets>(`/api/facets${apiQuery(p)}`, ctl.signal),
        get<Page>(`/api/leads${view ? "" : "/hidden"}${apiQuery(p, { limit: "1" })}`, ctl.signal),
      ]);
      let leads = pg.leads;
      let cursor = pg.next_cursor;
      for (let n = 1; !view && cursor && n < SHOWN_MAX_PAGES; n++) {
        const more = await get<Page>(`${path}${apiQuery(p, { limit: String(SHOWN_PAGE), cursor })}`, ctl.signal);
        leads = [...leads, ...more.leads];
        cursor = more.next_cursor;
      }
      setData({ leads, total: pg.total, cursor, facets, otherTotal: other.total });
      setLoadedAt(Date.now());
    })().catch((e: unknown) => {
      if (!ctl.signal.aborted) setError(message(e));
    });
    return () => ctl.abort();
  }, [key, attempt, page]);

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
  // Rows render best first (the API's order), or new then earlier when sorted by date:
  // keyboard order follows the screen.
  const byScore = params.get("sort") !== "date";
  const ordered = useMemo(
    () => (byScore ? visible : [...visible.filter((l) => isNew(l, now)), ...visible.filter((l) => !isNew(l, now))]),
    [visible, now, byScore],
  );
  const selected = selectedId === null ? -1 : ordered.findIndex((l) => l.id === selectedId);
  const lead = selected >= 0 ? ordered[selected] : undefined;
  const sheetOpen = narrow && !!lead;

  // Wide screens always show a lead beside the list: the first one until you pick.
  useEffect(() => {
    if (page === "leads" && !narrow && ordered.length > 0 && selected < 0) {
      navigate(withParam(current(), "lead", String(ordered[0]!.id)), true);
    }
  }, [page, narrow, ordered, selected, navigate]);

  const select = useCallback(
    (i: number, focusRow = true) => {
      if (ordered.length === 0) return;
      const next = Math.max(0, Math.min(ordered.length - 1, i));
      // on the phone, opening a lead is a history entry so the system Back closes the sheet
      navigate(withParam(current(), "lead", String(ordered[next]!.id)), !narrow);
      if (focusRow && !narrow) {
        rows.current[next]?.focus();
        rows.current[next]?.scrollIntoView({ block: "nearest" });
      }
    },
    [ordered, navigate, narrow],
  );
  const closeSheet = useCallback(() => {
    const id = selectedId;
    navigate(withParam(current(), "lead", null), true);
    // back where you were in the list
    requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-lead="${id}"]`)?.focus());
  }, [navigate, selectedId]);
  const setTab = (t: Tab) => navigate(withParam(current(), "tab", t === "overview" ? null : t), true);
  const updateStatus = (s: Status) =>
    setData((d) => d && { ...d, leads: d.leads.map((l) => (l.id === selectedId ? { ...l, status: s } : l)) });

  // Keyboard: j/k move (the panel follows), o or Enter opens the posting, / searches,
  // Esc clears or closes the sheet, ? helps. Dialogs handle their own keys.
  // One listener for the page's life that always runs the latest handler.
  const onKeyRef = useRef<(e: KeyboardEvent) => void>(() => {});
  useEffect(() => {
    const listener = (e: KeyboardEvent) => onKeyRef.current(e);
    window.addEventListener("keydown", listener);
    return () => window.removeEventListener("keydown", listener);
  }, []);
  useLayoutEffect(() => {
    onKeyRef.current = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || document.querySelector("dialog[open]")) return;
      const t = e.target as HTMLElement | null;
      const typing = !!t?.closest("input, select, textarea");
      if (e.key === "Escape") {
        if (t === search.current) {
          if (query) setQuery("");
          else search.current?.blur();
          e.preventDefault();
        } else if (sheetOpen) {
          e.preventDefault();
          closeSheet();
        }
        return;
      }
      if (typing) return;
      if (e.key === "?") {
        setHelp(true);
        return;
      }
      if (page !== "leads") return;
      const inList = !t || t === document.body || !!t.closest(".lead-row");
      switch (e.key) {
        case "/":
          e.preventDefault();
          search.current?.focus();
          break;
        case "j":
        case "ArrowDown":
          if (e.key === "ArrowDown" && !inList) return;
          if (sheetOpen) return;
          e.preventDefault();
          select(selected + 1);
          break;
        case "k":
        case "ArrowUp":
          if (e.key === "ArrowUp" && !inList) return;
          if (sheetOpen) return;
          e.preventDefault();
          select(selected - 1);
          break;
        case "o":
        case "Enter": {
          if (!inList || !lead) return;
          if (narrow && !sheetOpen && t?.closest(".lead-row")) {
            e.preventDefault();
            select(selected, false);
            return;
          }
          const href = safeHref(lead.url);
          e.preventDefault();
          if (href) window.open(href, "_blank", "noopener,noreferrer");
          break;
        }
      }
    };
  });

  useEffect(() => {
    const d = helpRef.current;
    if (help && d && !d.open) d.showModal();
  }, [help]);

  const nextTheme: Record<Theme, Theme> = { system: "light", light: "dark", dark: "system" };
  const leadsView = (v: string | null) => withParam(withParam(params, "view", v), "lead", null);

  return (
    <div className={`app${sheetOpen ? " sheet-open" : ""}`}>
      <a className="skip" href="#main">
        Skip to content
      </a>
      <header className="topbar">
        <nav className="pills" aria-label="Pages">
          {PAGES.map(([id, text]) => (
            <a key={id} className="pill" href={`?page=${id}`} onClick={goPage(id)} aria-current={page === id ? "page" : undefined}>
              {text}
            </a>
          ))}
        </nav>
        <div className="tools">
          {page === "leads" && (
            <>
              <div className="search">
                <Icon name="search" />
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
            </>
          )}
          <button
            type="button"
            className="icon-button"
            onClick={() =>
              setTheme((t) => {
                writePref("theme", nextTheme[t]);
                return nextTheme[t];
              })
            }
            aria-label={`Theme: ${theme}. Switch to ${nextTheme[theme]}`}
          >
            <Icon name={theme === "dark" ? "moon" : theme === "light" ? "sun" : "system"} />
          </button>
          <button type="button" className="icon-button hide-narrow" onClick={() => setHelp(true)} aria-label="Keyboard shortcuts (?)">
            ?
          </button>
        </div>
      </header>

      <main id="main">
        {page === "today" && <Today now={loadedAt} onOpen={openLead} go={goPage} />}
        {page === "board" && <Board onOpen={openLead} />}
        {page === "followups" && <FollowupsPage />}
        {page === "drafts" && <DraftsPage onOpen={(id) => navigate(new URLSearchParams(`page=leads&lead=${id}&tab=cv`))} now={loadedAt} />}
        {page === "leads" && (
          <div className="split">
            <section className="list-pane" aria-labelledby="leads-title">
              <div className="list-head">
                <h1 id="leads-title">{hidden ? "Hidden leads" : "Leads"}</h1>
                <div className="segmented" role="group" aria-label="Which leads">
                  <a href={`?${leadsView(null)}`} onClick={follow(withParam(leadsView(null), "new", null))} aria-current={!hidden && !params.get("new") ? "true" : undefined}>
                    Shown{data && <span className="count"> {num.format(hidden ? data.otherTotal : data.total)}</span>}
                  </a>
                  <a href={`?${withParam(leadsView(null), "new", "true")}`} onClick={follow(withParam(leadsView(null), "new", "true"))} aria-current={params.get("new") ? "true" : undefined}>
                    New
                  </a>
                  <a href={`?${leadsView("hidden")}`} onClick={follow(leadsView("hidden"))} aria-current={hidden ? "true" : undefined}>
                    Hidden{data && <span className="count"> {num.format(hidden ? data.total : data.otherTotal)}</span>}
                  </a>
                </div>
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
                {key && key !== "view=hidden" && (
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
                byScore={byScore}
                now={now}
                selected={selected}
                onSelect={(i) => select(i, false)}
                rows={rows}
                retry={() => setAttempt((a) => a + 1)}
                reset={reset}
                showHidden={follow(leadsView("hidden"))}
                showShown={follow(leadsView(null))}
                hiddenHref={`?${leadsView("hidden")}`}
                shownHref={`?${leadsView(null)}`}
                clearSearch={() => setQuery("")}
                loadMore={loadMore}
              />
            </section>
            <div className="detail-pane">
              {lead ? (
                <LeadDetail key={lead.id} lead={lead} now={now} tab={tab} onTab={setTab} onBack={closeSheet} onStatus={updateStatus} />
              ) : (
                data && ordered.length > 0 && <p className="empty">Pick a lead to see it here.</p>
              )}
            </div>
          </div>
        )}
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
          <dt>
            <kbd>j</kbd> <kbd>k</kbd>
          </dt>
          <dd>Next and previous lead; its details follow</dd>
          <dt>
            <kbd>o</kbd> <kbd>Enter</kbd>
          </dt>
          <dd>Open the posting in a new tab</dd>
          <dt>
            <kbd>/</kbd>
          </dt>
          <dd>Search</dd>
          <dt>
            <kbd>Esc</kbd>
          </dt>
          <dd>Clear the search, or close the lead on the phone</dd>
          <dt>
            <kbd>?</kbd>
          </dt>
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
  byScore: boolean;
  now: number;
  selected: number;
  onSelect: (i: number) => void;
  rows: React.RefObject<(HTMLLIElement | null)[]>;
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
              <strong>No leads yet.</strong> Run make collect to fetch postings.
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
        <LeadList
          leads={visible}
          hidden={hidden}
          byScore={props.byScore}
          now={props.now}
          selected={props.selected}
          onSelect={props.onSelect}
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
