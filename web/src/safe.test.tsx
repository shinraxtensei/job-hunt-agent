import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";
import type { Lead } from "./api";
import { LeadList } from "./App";
import { safeHref } from "./safe";
import { ScoreDetail } from "./Score";

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
  source: "lever",
  members: 1,
  members_url: "/api/leads/1/members",
  score: null,
  fit_level: null,
  not_enough_info: false,
  stretch: false,
  breakdown: null,
  reason: null,
  gaps: null,
  cv_variant: null,
  reason_current: false,
  score_status: "not_queued",
  status: "new",
  status_at: null,
  ...over,
});

function render(leads: Lead[]): HTMLElement {
  const el = document.createElement("div");
  document.body.append(el);
  act(() => createRoot(el).render(<LeadList leads={leads} hidden={false} />));
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
  it("counts postings only when a lead folds several", () => {
    const el = render([lead({ id: 1, members: 3 }), lead({ id: 2, members: 1 })]);
    const metas = [...el.querySelectorAll(".lead-row .meta")].map((p) => p.textContent);
    expect(metas[0]).toContain("3 postings");
    expect(metas[1]).not.toContain("postings");
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

function renderScore(l: Lead): HTMLElement {
  const el = document.createElement("div");
  document.body.append(el);
  act(() => createRoot(el).render(<ScoreDetail lead={l} updating="idle" />));
  return el;
}

describe("scores (M3-08)", () => {
  it("shows the fit level in words and the score, never colour alone", () => {
    const tag = render([scored({})]).querySelector(".lead-row .tag");
    expect(tag?.textContent).toBe("Strong fit, score Strong · 61.0");
    expect(tag?.querySelector(".visually-hidden")?.textContent).toBe("Strong fit, score ");
    expect(tag?.querySelector("[aria-hidden]")?.textContent).toBe("Strong · ");
  });

  it("says why a lead has no score", () => {
    const el = render([lead({ id: 1, score_status: "pending" }), lead({ id: 2, score_status: "needs_review" })]);
    expect([...el.querySelectorAll(".lead-row .tag")].map((t) => t.textContent)).toEqual(["Scoring…", "Review"]);
  });

  // AC6: reason and gaps are model output derived from postings: React text only.
  it("renders an HTML or javascript: payload in a reason or gap as text", () => {
    const payload = `<img src=x onerror="window.pwned=3"><a href="javascript:alert(1)">x</a>`;
    const el = renderScore(scored({ reason: payload, gaps: [payload, "javascript:alert(1)"] }));
    expect(el.querySelector("img, a")).toBeNull();
    expect(el.querySelector(".reason")?.textContent).toBe(payload);
    expect([...el.querySelectorAll(".gaps li")].map((li) => li.textContent)).toEqual([payload, "javascript:alert(1)"]);
    expect((window as { pwned?: number }).pwned).toBeUndefined();
  });

  it("shows the breakdown as rows with numbers, the bars hidden from screen readers", () => {
    const el = renderScore(scored({}));
    const rows = [...el.querySelectorAll(".breakdown tr")].map((tr) => [tr.querySelector("th")?.textContent, tr.querySelector(".pts")?.textContent]);
    expect(rows).toEqual([
      ["Required skills", "17.5 of 35"],
      ["Freshness", "5.0 of 5"],
    ]);
    expect(el.querySelector(".breakdown td.bar")?.getAttribute("aria-hidden")).toBe("true");
  });
});

describe("CV drafts (M4-05)", () => {
  // AC6: draft sentences, evidence and findings render as text only.
  it("renders an HTML payload in a draft bullet as text", async () => {
    const { DraftView } = await import("./Drafts");
    const payload = `Led <img src=x onerror="window.pwned=4"> the move`;
    const el = document.createElement("div");
    document.body.append(el);
    const draft = {
      id: 1,
      status: "blocked" as const,
      variant: "BE",
      content: {
        person: { name: "Sam", headline: "", email: "", phone: "", location: "", links: [] },
        summary: [],
        roles: [{ id: "r", title: "Engineer", employer: "Acme", start: "2023-05", end: "present", bullets: [{ text: payload, evidence_ids: ["l1"] }] }],
        projects: [],
        skills: [],
        education: [],
        languages: [],
      },
      letter: { greeting: "Dear Hiring Team,", opening: "I am writing.", paragraphs: [[{ text: payload, evidence_ids: ["l1"] }]], closing: "Best regards,", name: "Sam", words: 9 },
      coverage: null,
      findings: { violations: [{ rule: "inflated_number", where: "roles[r].bullets[0]", sentence: payload, closest: "l1", detail: "5 isn't in the cited evidence" }], warnings: [] },
      evidence_version: null,
      prompt_version: 1,
      model: "m",
      error: null,
      requested_at: "2026-11-08T00:00:00Z",
      made_at: null,
      decided_at: null,
      documents: [],
    };
    act(() => createRoot(el).render(<DraftView draft={draft} evidence={{ l1: { text: "<b>evidence</b>", retired: false } }} waiting={false} />));
    expect(el.querySelector("img, b.x")).toBeNull();
    expect(el.textContent).toContain(payload);
    expect(el.textContent).toContain("Blocks the draft");
    expect(el.querySelectorAll("section[aria-label='Cover letter'] li").length).toBeGreaterThan(0);
    expect(el.textContent).toContain("“<b>evidence</b>”");
    expect((window as { pwned?: number }).pwned).toBeUndefined();
    expect(el.textContent).not.toContain("French");
  });

  // M7-02: the draft says its language, and why a French posting got an English one.
  it("shows the draft's language and the reason it isn't French", async () => {
    const { DraftView } = await import("./Drafts");
    const content = {
      person: { name: "Sam", headline: "", email: "", phone: "", location: "", links: [] },
      summary: [],
      roles: [],
      projects: [],
      skills: [],
      education: [],
      languages: [],
    };
    const base = { id: 2, status: "ready_for_review" as const, variant: "BE", letter: null, coverage: null, findings: null, evidence_version: null, prompt_version: 1, model: "m", error: null, requested_at: "2026-11-08T00:00:00Z", made_at: null, decided_at: null, documents: [] };
    const el = document.createElement("div");
    document.body.append(el);
    const root = createRoot(el);
    act(() => root.render(<DraftView draft={{ ...base, content: { ...content, language: "fr", language_note: "" } }} evidence={{}} waiting={false} />));
    expect(el.querySelector(".draft-status")?.textContent).toContain("French");
    const note = "English: no French text for example-deploys.";
    act(() => root.render(<DraftView draft={{ ...base, content: { ...content, language: "en", language_note: note } }} evidence={{}} waiting={false} />));
    expect(el.textContent).toContain(note);
  });
});

describe("board postings (M7-03)", () => {
  // AC2: a board's posting names the board, as its terms ask; the title links to it.
  it("says which board a lead came from", async () => {
    const { place } = await import("./format");
    expect(place(lead({ origin: "board", source: "remotive", locations: ["Worldwide"] }))).toBe("Worldwide · Remote · via Remotive");
    expect(place(lead({}))).not.toContain("via");
    expect(place(lead({ origin: "board", source: "remoteok", locations: [] }))).toBe("No location · Remote · via Remote OK");
    const el = render([lead({ origin: "board", source: "remotive", url: "https://remotive.com/remote-jobs/software-dev/x-1" })]);
    expect(el.textContent).toContain("via Remotive");
    expect(el.querySelector("a[href='https://remotive.com/remote-jobs/software-dev/x-1']")).not.toBeNull();
  });
});

describe("stretch (M9-03)", () => {
  // A flag beside the fit, not a fit level: four levels remain.
  it("tags a lead that asks for more than the profile", async () => {
    const { FIT_LEVELS } = await import("./api");
    expect(FIT_LEVELS).toEqual(["very_strong", "strong", "good", "no"]);
    const el = render([lead({ score: 61, fit_level: "strong", stretch: true, score_status: "scored" })]);
    const tag = [...el.querySelectorAll(".tag")].find((t) => t.textContent === "Stretch");
    expect(tag?.getAttribute("title")).toContain("higher level or more years");
    expect(render([lead({ score: 61, fit_level: "strong", score_status: "scored" })]).textContent).not.toContain("Stretch");
  });
});
