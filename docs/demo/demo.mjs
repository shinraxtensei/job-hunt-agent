// Demo data for the README screenshots (M8-03): the example profile "Sam Example"
// (data/profile/*.example.yaml) and invented leads at invented companies. Nothing here is
// real: no owner data, no real posting. Served through Playwright mocks by shoot.mjs.
import { readFileSync } from "node:fs";

const golden = (n) => JSON.parse(readFileSync(new URL(`../../internal/httpapi/testdata/golden/${n}.json`, import.meta.url), "utf8"));
const template = golden("lead");
const NOW = Date.parse("2026-11-09T09:00:00Z");
const hoursAgo = (h) => new Date(NOW - h * 3600e3).toISOString();

const FACTORS = [
  ["required", 35],
  ["title", 20],
  ["seniority", 15],
  ["location", 15],
  ["optional", 10],
  ["freshness", 5],
];

// [company, title, track, fit, score, role, seniority, workplace, location, hours ago, status, reason, gaps, source]
const ROWS = [
  ["Acme Payments", "Senior Backend Engineer", "remote", "very_strong", 81.5, "backend", "senior", "remote", "Remote – EMEA", 3, "new", "Your Node.js and PostgreSQL work on billing matches the core of this role.", ["Kafka"], "lever"],
  ["Northwind Data", "Backend Engineer, Platform", "remote", "strong", 68.2, "backend", "mid", "remote", "Remote – Morocco", 5, "new", "Backend services on PostgreSQL fit; the platform focus is a small stretch.", ["Terraform"], "greenhouse"],
  ["Kestrel Labs", "Full-Stack Developer", "remote", "strong", 64.0, "fullstack", "unknown", "remote", "Worldwide", 7, "new", "Your full-stack work with React and Node.js covers most of the stack.", [], "remotive"],
  ["Brick & Mortar", "Software Engineer, Billing", "relocation", "strong", 61.7, "backend", "mid", "hybrid", "Amsterdam", 20, "applied", "Billing experience is a direct match.", ["Go"], "ashby"],
  ["Juniper Cloud", "Platform Engineer", "remote", "good", 52.4, "platform", "mid", "remote", "Remote – Europe", 26, "new", "Docker and CI experience fit; Kubernetes is not in your evidence.", ["Kubernetes", "Helm"], "lever"],
  ["Otter Systems", "Backend Developer (Node.js)", "morocco", "good", 49.9, "backend", "unknown", "onsite", "Casablanca", 30, "drafted", "Node.js backend work matches; the posting asks for more years than stated.", ["NestJS"], "personio"],
  ["Larkspur Health", "Senior Software Engineer", "remote", "good", 45.1, "backend", "senior", "remote", "Remote – US or EMEA", 44, "new", "Strong backend overlap; healthcare domain is new to you.", ["HL7"], "remoteok"],
  ["Globex Consulting", "Software Engineer, Data", "consulting", "stretch", 34.8, "data", "mid", "hybrid", "Rabat", 50, "new", "Data pipelines are adjacent to your backend work.", ["Spark", "Airflow"], "greenhouse"],
  ["Pinecrest Mobile", "Mobile Engineer (Flutter)", "relocation", "stretch", 31.0, "mobile", "unknown", "onsite", "Lisbon", 70, "skipped", "Flutter is listed as learning in your skill map.", ["Flutter", "Dart"], "lever"],
  ["Quartz AI", "ML Engineer", "remote", "no", 18.3, "ai_ml", "senior", "remote", "Remote – US", 90, "new", "Machine learning roles are outside your target titles.", ["PyTorch", "MLOps"], "ashby"],
];

const leads = ROWS.map(([company, title, track, fit, score, role, seniority, workplace, location, hours, status, reason, gaps, source], i) => {
  const value = score / 100;
  return {
    ...template,
    id: 100 + i,
    company: { name: company, track },
    title,
    role_family: role,
    seniority,
    workplaces: [workplace],
    locations: [location],
    matched_location: location,
    experience_min_years: [3, null, 4, null, 5][i % 5],
    url: `https://example.com/jobs/${100 + i}`,
    published_at: hoursAgo(hours + 24),
    first_seen_at: hoursAgo(hours),
    origin: source === "remotive" || source === "remoteok" ? "board" : "collected",
    source,
    members: 1,
    members_url: `/api/leads/${100 + i}/members`,
    score,
    fit_level: fit,
    breakdown: FACTORS.map(([factor, weight]) => ({ factor, weight, value, points: Math.round(weight * value * 10) / 10, detail: "" })),
    reason,
    gaps,
    cv_variant: role === "fullstack" ? "FS" : "BE",
    reason_current: true,
    score_status: "scored",
    status,
    status_at: status === "new" ? null : hoursAgo(hours - 2),
  };
});

const count = (f) => leads.reduce((m, l) => ((m[f(l)] = (m[f(l)] ?? 0) + 1), m), {});
const facets = {
  total: leads.length,
  facets: {
    closed: { false: leads.length },
    company: count((l) => l.company.name),
    country: {},
    fit: count((l) => l.fit_level),
    remote_scope: {},
    role_family: count((l) => l.role_family),
    seniority: count((l) => l.seniority),
    track: count((l) => l.track ?? l.company.track),
    workplace: count((l) => l.workplaces[0]),
  },
};

// The CV & letter tab: the committed draft fixture, retitled for the demo lead.
const drafts = golden("drafts");
for (const d of drafts.drafts) {
  d.status = "ready_for_review";
  d.findings = { violations: [], warnings: [] };
  for (const r of d.content?.roles ?? []) for (const b of r.bullets) b.text = b.text.replace(/<[^>]*>/g, "").replace(/\s+/g, " ");
}

const applied = leads.filter((l) => l.status !== "new");
export const routes = {
  "/api/leads": { total: leads.length, leads, next_cursor: null },
  "/api/leads/hidden": { total: 0, leads: [], next_cursor: null },
  "/api/facets": facets,
  "/api/presets": golden("presets"),
  "/api/today": { since: hoursAgo(28), new_total: 7, new: leads.slice(0, 7), closed: [] },
  "/api/board": { cards: applied.map((l) => ({ lead_id: l.id, company: l.company.name, title: l.title, fit_level: l.fit_level, status: l.status, status_at: l.status_at, followup_due: l.status === "applied" })) },
  "/api/followups": {
    followups: [{ application_id: 1, lead_id: 103, company: "Brick & Mortar", title: "Software Engineer, Billing", applied_at: hoursAgo(24 * 8), days: 8, contact: "Jo Recruiter", message: "Hello Jo Recruiter,\n\nI applied for the Software Engineer, Billing role at Brick & Mortar on 1 November and wanted to follow up. I'm still very interested and happy to share anything else that would help.\n\nBest regards,\nSam Example" }],
  },
  "/api/drafts": { drafts: [{ id: 1, lead_id: 105, company: "Otter Systems", title: "Backend Developer (Node.js)", status: "ready_for_review", requested_at: hoursAgo(6), made_at: hoursAgo(6) }] },
  "/api/visits": {},
  ...Object.fromEntries(leads.map((l) => [`/api/leads/${l.id}`, l])),
  ...Object.fromEntries(leads.map((l) => [`/api/leads/${l.id}/drafts`, drafts])),
  ...Object.fromEntries(leads.map((l) => [`/api/leads/${l.id}/members`, { lead_id: l.id, members: [{ id: l.id, title: l.title, team: "Engineering", department: "", locations: l.locations, workplace: l.workplaces[0], url: l.url, first_seen_at: l.first_seen_at, closed: false }] }])),
  ...Object.fromEntries(
    leads.map((l) => [
      `/api/leads/${l.id}/application`,
      {
        application:
          l.status === "new"
            ? null
            : { id: l.id, status: l.status, status_at: l.status_at, applied_at: l.status === "applied" ? l.status_at : null, snapshot: null, history: [{ status: l.status, at: l.status_at, auto: false }], documents: [], notes: [], contacts: [] },
      },
    ]),
  ),
};
export const now = NOW;
