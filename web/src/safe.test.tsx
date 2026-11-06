import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";
import type { Lead } from "./api";
import { LeadTable } from "./App";
import { safeHref } from "./safe";

// React's act() needs this flag outside a test framework integration.
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const lead = (over: Partial<Lead>): Lead => ({
  id: 1,
  company: { name: "Acme", track: "remote" },
  title: "Engineer",
  role_family: "backend",
  seniority: "senior",
  workplaces: ["remote"],
  remote_scopes: [],
  countries: [],
  locations: ["Remote"],
  matched_location: "",
  experience_min_years: null,
  closed: false,
  team: "",
  url: "https://example.com/job",
  published_at: null,
  first_seen_at: "2026-11-02T08:00:00Z",
  origin: "collected",
  members: 1,
  members_url: "/api/leads/1/members",
  score: null,
  fit_level: null,
  not_enough_info: false,
  breakdown: null,
  reason: null,
  gaps: null,
  cv_variant: null,
  reason_current: false,
  score_status: "not_queued",
  ...over,
});

function render(leads: Lead[]): HTMLElement {
  const el = document.createElement("div");
  document.body.append(el);
  act(() => createRoot(el).render(<LeadTable leads={leads} hidden={false} />));
  return el;
}

describe("untrusted posting text (AC4)", () => {
  it("shows an HTML title as text, never as markup", () => {
    const payload = `<script>window.pwned=1</script><img src=x onerror="window.pwned=2">Backend`;
    const el = render([lead({ title: payload, locations: [payload] })]);
    expect(el.querySelector("script")).toBeNull();
    expect(el.querySelector("img")).toBeNull();
    expect(el.textContent).toContain(payload);
    expect((window as { pwned?: number }).pwned).toBeUndefined();
  });

  it("renders a javascript: URL as plain text, not a link", () => {
    const el = render([lead({ title: "Click me", url: "javascript:alert(1)" })]);
    expect(el.querySelector("a")).toBeNull();
    expect(el.textContent).toContain("Click me");
  });

  it("links http(s) postings in a new tab without opener or referrer", () => {
    const a = render([lead({})]).querySelector("a");
    expect(a?.getAttribute("href")).toBe("https://example.com/job");
    expect(a?.getAttribute("target")).toBe("_blank");
    expect(a?.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it.each([
    ["javascript:alert(1)", null],
    ["JaVaScRiPt:alert(1)", null],
    [" javascript:alert(1)", null],
    ["data:text/html,<script>alert(1)</script>", null],
    ["vbscript:msgbox", null],
    ["/relative/path", null],
    ["", null],
    ["https://jobs.lever.co/x/1", "https://jobs.lever.co/x/1"],
    ["http://example.com/a?b=c", "http://example.com/a?b=c"],
  ])("safeHref(%j) = %j", (url, want) => {
    expect(safeHref(url)).toBe(want);
  });
});

describe("duplicate groups (AC2c)", () => {
  it("counts postings only when a lead folds several, and offers to show them", () => {
    const el = render([lead({ id: 1, members: 3 }), lead({ id: 2, members: 1 })]);
    const counts = [...el.querySelectorAll("tr.row td.num:nth-of-type(6)")].map((td) => td.textContent);
    expect(counts).toEqual(["3", ""]);
    const labels = [...el.querySelectorAll("tr.row button")].map((b) => b.getAttribute("aria-label"));
    expect(labels).toEqual(["Show details and 3 postings", "Show details"]);
  });
});

const scored = (over: Partial<Lead>): Lead =>
  lead({
    score: 61.04,
    fit_level: "strong",
    breakdown: [
      { factor: "required", weight: 35, value: 0.5, points: 17.5, detail: "" },
      { factor: "freshness", weight: 5, value: 1, points: 5, detail: "" },
    ],
    reason: "Your backend stack fits. Rust is missing.",
    gaps: ["Rust"],
    cv_variant: "BE",
    reason_current: true,
    score_status: "scored",
    ...over,
  });

describe("scores (M3-08)", () => {
  it("shows the fit level in words and the score, never colour alone", () => {
    const cell = render([scored({})]).querySelector("td.fit-cell");
    expect(cell?.textContent).toBe("Strong fit, score 61.0");
    expect(cell?.querySelectorAll(".fit-mark i.on")).toHaveLength(3);
  });

  it("says why a lead has no score", () => {
    const el = render([lead({ id: 1, score_status: "pending" }), lead({ id: 2, score_status: "needs_review" })]);
    expect([...el.querySelectorAll("td.fit-cell")].map((td) => td.textContent)).toEqual(["…Being scored", "Review"]);
  });

  // AC6: reason and gaps are model output derived from postings: React text only.
  it("renders an HTML or javascript: payload in a reason or gap as text", () => {
    const payload = `<img src=x onerror="window.pwned=3"><a href="javascript:alert(1)">x</a>`;
    const el = render([scored({ reason: payload, gaps: [payload, "javascript:alert(1)"] })]);
    act(() => el.querySelector<HTMLButtonElement>("td.more button")!.click());
    expect(el.querySelector(".detail img, .detail a")).toBeNull();
    expect(el.querySelector(".detail .reason")?.textContent).toBe(payload);
    expect([...el.querySelectorAll(".detail .gaps li")].map((li) => li.textContent)).toEqual([payload, "javascript:alert(1)"]);
    expect((window as { pwned?: number }).pwned).toBeUndefined();
  });

  it("shows the breakdown as rows with numbers, the bars hidden from screen readers", () => {
    const el = render([scored({})]);
    act(() => el.querySelector<HTMLButtonElement>("td.more button")!.click());
    const rows = [...el.querySelectorAll(".breakdown tr")].map((tr) => [tr.querySelector("th")?.textContent, tr.querySelector(".pts")?.textContent]);
    expect(rows).toEqual([
      ["Required skills", "17.5 of 35"],
      ["Freshness", "5.0 of 5"],
    ]);
    expect(el.querySelector(".breakdown td.bar")?.getAttribute("aria-hidden")).toBe("true");
  });
});
