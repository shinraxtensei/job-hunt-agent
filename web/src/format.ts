import type { Lead } from "./api";
import { label } from "./labels";

const DAY = 24 * 3600e3;
export const dateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" });
export const num = new Intl.NumberFormat();
export const one = new Intl.NumberFormat(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 });

// "New" is the API's meaning: first seen in the last 24 hours.
export const isNew = (l: Lead, now: number) => now - Date.parse(l.first_seen_at) < DAY;

// ago is a relative time for cards and rows: "now", "5 h ago", "3 d ago".
export function ago(iso: string, now: number): string {
  const h = Math.max(0, Math.floor((now - Date.parse(iso)) / 3600e3));
  if (h < 1) return "now";
  if (h < 48) return `${h} h ago`;
  return `${Math.floor(h / 24)} d ago`;
}

// Words a location uses for a workplace ("Hybrid", "On-site", "Remote - EMEA").
const workplaceWords: Record<string, string[]> = { remote: ["remote"], hybrid: ["hybrid"], onsite: ["onsite", "on site"] };
const words = (s: string) => ` ${s.toLowerCase().replace(/[^a-z0-9]+/g, " ").trim()} `;

// place is a lead's location: the one that matched the filters (else the first), how many
// more there are, and the workplace unless the location already says it.
export function place(l: Lead): string {
  const shown = l.matched_location || l.locations[0];
  const more = l.locations.length - (shown ? 1 : 0);
  const loc = shown ? shown + (more > 0 ? ` +${more}` : "") : "No location";
  const said = shown ? words(shown) : "";
  const wp = l.workplaces
    .filter((w) => w !== "unknown" && !(workplaceWords[w] ?? []).some((word) => said.includes(` ${word} `)))
    .map((w) => label("workplace", w))
    .join(", ");
  const where = wp ? `${loc} · ${wp}` : loc;
  return l.origin === "board" ? `${where} · via ${BOARDS[l.source] ?? l.source}` : where;
}

// Boards whose terms ask to be named as the source (ADR-0029).
const BOARDS: Record<string, string> = { remotive: "Remotive" };

// The semantic pastels (docs/design/brief.md): one meaning each, always with words.
export type Tone = "mint" | "sand" | "salmon" | "blue" | "lilac";
export const fitTone: Record<string, Tone> = { very_strong: "mint", strong: "mint", good: "sand", stretch: "salmon", no: "salmon" };
export const statusTone: Record<string, Tone> = {
  new: "lilac",
  shortlisted: "lilac",
  drafted: "sand",
  applied: "blue",
  replied: "blue",
  interviewing: "blue",
  offer: "mint",
  rejected: "salmon",
  closed: "salmon",
  skipped: "salmon",
};
export const draftTone: Record<string, Tone> = {
  requested: "lilac",
  ready_for_review: "sand",
  blocked: "salmon",
  approved: "mint",
  discarded: "salmon",
  failed: "salmon",
};

// leadHref and inApp make an in-app link to a lead: a real href (new tab works), and a plain click
// stays in the page.
export const leadHref = (id: number, tab?: string) => `?page=leads&lead=${id}${tab ? `&tab=${tab}` : ""}`;
export function inApp(go: () => void) {
  return (e: { metaKey: boolean; ctrlKey: boolean; shiftKey: boolean; button: number; preventDefault: () => void }) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    go();
  };
}
