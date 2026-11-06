import { useEffect, useRef } from "react";
import { FIT_LEVELS, type Facets, type Preset, type Presets } from "./api";

const FIT_ORDER: string[] = [...FIT_LEVELS, "unscored"];
import { applied, FACETS, selected } from "./filters";
import { label as display } from "./labels";
import { Icon } from "./icons";

type Props = {
  open: boolean;
  onClose: () => void;
  params: URLSearchParams;
  preset: Preset | undefined;
  presets: Presets["presets"];
  facets: Facets | undefined;
  set: (k: string, v: string | null) => void;
  reset: () => void;
};

// FilterSheet is a native modal <dialog>: focus is trapped, Esc closes it, and focus
// returns to the button that opened it. Changes apply at once and go to the URL.
export function FilterSheet(props: Props) {
  const { open, onClose, params, preset, presets, facets, set, reset } = props;
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);

  const chips = applied(params, preset);
  return (
    <dialog ref={ref} className="sheet" aria-labelledby="filters-title" onClose={onClose}>
      <div className="sheet-head">
        <h2 id="filters-title">Filters</h2>
        <button type="button" className="icon-button" aria-label="Close filters" onClick={onClose}>
          <Icon name="close" />
        </button>
      </div>
      <div className="sheet-body">
        <section aria-labelledby="applied-title">
          <h3 id="applied-title">Applied</h3>
          <ul className="applied">
            {chips.map((c) => (
              <li key={c.key}>
                <span>
                  {c.label}: <b>{c.value}</b>
                </span>
                {c.fromURL && (
                  <button type="button" className="link-button" onClick={() => set(c.key, null)}>
                    Remove
                  </button>
                )}
              </li>
            ))}
          </ul>
          <button type="button" className="button" onClick={reset}>
            Reset to my defaults
          </button>
        </section>
        <section aria-label="Preset and limits">
          <div className="field">
            <label htmlFor="f-preset">Preset</label>
            <select
              id="f-preset"
              name="preset"
              value={params.get("preset") ?? "default"}
              onChange={(e) => set("preset", e.target.value === "default" ? null : e.target.value)}
            >
              {presets
                .filter((p) => p.preset)
                .map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.id === "default" ? "My defaults" : p.id}
                  </option>
                ))}
              <option value="none">No filters</option>
            </select>
          </div>
          <div className="field">
            <label htmlFor="f-sort">Sort</label>
            <select id="f-sort" name="sort" value={params.get("sort") ?? "score"} onChange={(e) => set("sort", e.target.value === "score" ? null : e.target.value)}>
              <option value="score">Best fit first</option>
              <option value="date">Newest first</option>
            </select>
          </div>
          <div className="field">
            <label htmlFor="f-exp">Max years required</label>
            <input
              id="f-exp"
              name="max_experience"
              type="number"
              inputMode="numeric"
              autoComplete="off"
              min={0}
              max={20}
              value={params.get("max_experience") ?? preset?.experience_max_years ?? ""}
              onChange={(e) => set("max_experience", e.target.value)}
            />
          </div>
          <div className="field">
            <label htmlFor="f-closed">Closed postings</label>
            <select id="f-closed" name="closed" value={params.get("closed") ?? preset?.closed ?? ""} onChange={(e) => set("closed", e.target.value)}>
              <option value="">Any</option>
              <option value="hide">Hide</option>
              <option value="show">Show</option>
              <option value="only">Only closed</option>
            </select>
          </div>
          <label className="option">
            <input type="checkbox" checked={params.get("new") === "true"} onChange={(e) => set("new", e.target.checked ? "true" : null)} />
            <span className="name">New in the last 24 hours</span>
          </label>
        </section>
        {FACETS.map(([k, label]) => {
          const sel = selected(params, preset, k);
          const counts = facets?.facets[k] ?? {};
          const values = [...new Set([...Object.keys(counts), ...sel])].sort(
            k === "fit" ? (a, b) => FIT_ORDER.indexOf(a) - FIT_ORDER.indexOf(b) : (a, b) => display(k, a).localeCompare(display(k, b)),
          );
          return (
            <fieldset key={k}>
              <legend>{label}</legend>
              <div className="options">
                {values.map((v) => (
                  <label key={v} className="option">
                    <input
                      type="checkbox"
                      checked={sel.includes(v)}
                      onChange={(e) => {
                        const next = e.target.checked ? [...sel, v] : sel.filter((s) => s !== v);
                        set(k, next.join(","));
                      }}
                    />
                    <span className="name">{display(k, v)}</span>
                    <span className="count" aria-label={`${counts[v] ?? 0} leads`}>
                      {counts[v] ?? 0}
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>
          );
        })}
      </div>
    </dialog>
  );
}
