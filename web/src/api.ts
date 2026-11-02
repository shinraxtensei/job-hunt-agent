// Types and fetchers for the M2-05 leads API. Every string in a Lead or Member is
// untrusted posting text: render it only as React text.

export type Lead = {
  id: number;
  company: { name: string; track: string };
  title: string;
  role_family: string;
  seniority: string;
  workplaces: string[];
  remote_scopes: string[];
  countries: string[];
  locations: string[];
  matched_location: string; // the location that passed the location rules, "" if none applied
  experience_min_years: number | null;
  closed: boolean;
  team: string;
  url: string;
  published_at: string | null;
  first_seen_at: string;
  members: number;
  members_url: string;
  hidden_by?: string[];
};

export type Page = { total: number; leads: Lead[]; next_cursor?: string };

export type Facets = { total: number; facets: Record<string, Record<string, number>> };

export type Preset = {
  name: string;
  role_family?: string[];
  seniority?: string[];
  experience_max_years?: number;
  closed?: string;
  location?: unknown[];
};

export type Presets = { presets: { id: string; preset?: Preset; error?: string }[] };

export type Member = {
  id: number;
  title: string;
  team: string;
  department: string;
  locations: string[];
  workplace: string;
  url: string;
  first_seen_at: string;
  closed: boolean;
};

export type Members = { lead_id: number; members: Member[] };

export class ApiError extends Error {}

export async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, { signal, headers: { Accept: "application/json" } });
  const body: unknown = await res.json().catch(() => null);
  if (!res.ok) {
    const err = (body as { error?: { parameter?: string; message?: string } } | null)?.error;
    throw new ApiError(err?.parameter ? `${err.parameter}: ${err.message}` : `HTTP ${res.status}`);
  }
  return body as T;
}

// The page URL holds the API's filter parameters plus the page's own `view` (leads |
// hidden) and `q` (search). apiQuery strips what the API doesn't know.
export function apiQuery(page: URLSearchParams, extra: Record<string, string> = {}): string {
  const q = new URLSearchParams(page);
  q.delete("view");
  q.delete("q");
  for (const [k, v] of Object.entries(extra)) q.set(k, v);
  const s = q.toString();
  return s ? `?${s}` : "";
}
