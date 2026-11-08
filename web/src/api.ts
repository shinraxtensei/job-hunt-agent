// Types and fetchers for the M2-05 leads API. Every string in a Lead or Member is
// untrusted posting text or model output: render it only as React text.

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
  origin: "collected" | "manual"; // manual: added by link (shown in M6)
  members: number;
  members_url: string;
  hidden_by?: string[];
  // The score (M3-08): null when not scored. reason and gaps are model output.
  score: number | null;
  fit_level: FitLevel | null;
  not_enough_info: boolean;
  breakdown: Factor[] | null;
  reason: string | null;
  gaps: string[] | null;
  cv_variant: string | null;
  reason_current: boolean; // false: from older inputs, shown marked until regenerated
  score_status: "scored" | "pending" | "needs_review" | "not_queued";
};

export const FIT_LEVELS = ["very_strong", "strong", "good", "stretch", "no"] as const;
export type FitLevel = (typeof FIT_LEVELS)[number];

export type Factor = { factor: string; weight: number; value: number; points: number; detail: string };

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

// requestReason asks the API for a fresh reason (ADR-0022): a same-origin JSON POST.
export async function requestReason(id: number): Promise<boolean> {
  const res = await fetch(`/api/leads/${id}/reason`, { method: "POST", headers: { "Content-Type": "application/json" } });
  if (!res.ok) throw new ApiError(`HTTP ${res.status}`);
  return ((await res.json()) as { requested: boolean }).requested;
}

// M4-05: tailored CV drafts (M4-02). Every string is the owner's evidence, bank facts or
// model output derived from a posting: render it only as React text.
export type DraftBullet = { text: string; evidence_ids: string[] };
export type DraftSection = {
  id: string;
  title?: string;
  employer?: string;
  name?: string;
  stack?: string;
  location?: string;
  start: string | null;
  end: string | null;
  bullets: DraftBullet[];
};
export type DraftContent = {
  person: { name: string; headline: string; email: string; phone: string; location: string; links: string[] };
  summary: DraftBullet[];
  roles: DraftSection[];
  projects: DraftSection[];
  skills: string[];
  education: { id: string; school: string; degree: string; location: string; start: string; end: string }[];
  languages: { name: string; level: string }[];
};
export type Finding = { rule: string; where: string; sentence: string; closest: string | null; detail: string };
export type SkillCoverage = {
  name: string;
  kind: "required" | "optional";
  skill: string | null;
  level: string | null;
  status: "covered" | "listed" | "gap";
  why: "learning" | "missing" | "no_evidence" | null;
  lines: string[];
  suspicious: boolean;
};
export type Draft = {
  id: number;
  status: "requested" | "blocked" | "ready_for_review" | "approved" | "discarded" | "failed";
  variant: string | null;
  content: DraftContent | null;
  coverage: { variant: string; skills: SkillCoverage[] } | null;
  findings: { violations: Finding[]; warnings: Finding[] } | null;
  evidence_version: string | null;
  prompt_version: number | null;
  model: string | null;
  error: string | null;
  requested_at: string;
  made_at: string | null;
  decided_at: string | null;
};
export type Drafts = { drafts: Draft[]; evidence: Record<string, { text: string; retired: boolean }> };

// post is a same-origin JSON write (ADR-0022: the API refuses anything else).
export async function post<T>(path: string): Promise<T> {
  const res = await fetch(path, { method: "POST", headers: { "Content-Type": "application/json" } });
  const body: unknown = await res.json().catch(() => null);
  if (!res.ok) {
    const err = (body as { error?: { message?: string } } | null)?.error;
    throw new ApiError(err?.message ?? `HTTP ${res.status}`);
  }
  return body as T;
}
