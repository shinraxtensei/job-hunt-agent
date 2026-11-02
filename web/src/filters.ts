import type { Preset } from "./api";

// The facets the filter sheet offers, in the API's order.
export const FACETS: [key: string, label: string][] = [
  ["role_family", "Role"],
  ["seniority", "Seniority"],
  ["workplace", "Workplace"],
  ["remote_scope", "Remote scope"],
  ["country", "Country"],
  ["track", "Track"],
  ["company", "Company"],
];

export const LOCATION_PARAMS = ["workplace", "remote_scope", "country", "city", "track"];

// A readable form of a facet value ("ai_ml" → "ai ml", "country:MA" → "MA").
export const pretty = (v: string) => v.replace(/^country:/, "").replaceAll("_", " ");

export type Applied = { key: string; label: string; value: string; fromURL: boolean };

const list = (v: string | null) => (v === null ? null : v.split(",").filter(Boolean));

// applied lists every filter in effect: each URL parameter, else what the preset sets.
// fromURL ones can be removed (falling back to the preset).
export function applied(params: URLSearchParams, preset: Preset | undefined): Applied[] {
  const out: Applied[] = [];
  const presetName = params.get("preset");
  if (presetName) out.push({ key: "preset", label: "Preset", value: presetName === "none" ? "No filters" : presetName, fromURL: true });
  const facet = (key: string, label: string, fallback: string[] | undefined) => {
    const v = list(params.get(key));
    const values = v ?? fallback;
    if (values?.length) out.push({ key, label, value: values.map(pretty).join(", "), fromURL: v !== null });
  };
  facet("role_family", "Role", preset?.role_family);
  facet("seniority", "Seniority", preset?.seniority);
  const exp = params.get("max_experience") ?? preset?.experience_max_years?.toString();
  if (exp) out.push({ key: "max_experience", label: "Experience", value: `≤ ${exp} years`, fromURL: params.has("max_experience") });
  const loc = LOCATION_PARAMS.filter((k) => params.has(k));
  if (loc.length) for (const k of loc) facet(k, k === "remote_scope" ? "Remote scope" : k[0]!.toUpperCase() + k.slice(1), undefined);
  else if (preset?.location?.length) out.push({ key: "location", label: "Location", value: "preset rules", fromURL: false });
  facet("company", "Company", undefined);
  const closed = params.get("closed") ?? preset?.closed;
  if (closed) out.push({ key: "closed", label: "Closed", value: { hide: "hidden", show: "shown", only: "only closed" }[closed] ?? closed, fromURL: params.has("closed") });
  if (params.get("new") === "true") out.push({ key: "new", label: "New", value: "last 24 h", fromURL: true });
  if (params.get("since")) out.push({ key: "since", label: "Since", value: params.get("since")!, fromURL: true });
  return out;
}

// selected is a facet's current selection: its URL parameter, else the preset's value.
export function selected(params: URLSearchParams, preset: Preset | undefined, key: string): string[] {
  const v = list(params.get(key));
  if (v) return v;
  if (key === "role_family") return preset?.role_family ?? [];
  if (key === "seniority") return preset?.seniority ?? [];
  return [];
}
