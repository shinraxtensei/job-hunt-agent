import { describe, expect, it } from "vitest";
import type { Lead } from "./api";
import { label, reason } from "./labels";
import { place } from "./format";

describe("facet labels (M2-08 AC3)", () => {
  it.each([
    ["role_family", "ai_ml", "AI/ML"],
    ["role_family", "fullstack", "Full-stack"],
    ["role_family", "backend", "Backend"],
    ["role_family", "eng_management", "Eng management"],
    ["role_family", "qa", "QA"],
    ["seniority", "staff_plus", "Staff+"],
    ["seniority", "lead_manager", "Lead/manager"],
    ["workplace", "onsite", "On-site"],
    ["workplace", "unknown", "Unknown"],
    ["remote_scope", "emea", "EMEA"],
    ["remote_scope", "us_only", "US only"],
    ["remote_scope", "country:MA", "Morocco"],
    ["country", "MA", "Morocco"],
    ["country", "GB", "United Kingdom"],
    ["track", "relocation", "Relocation"],
    ["company", "Stripe", "Stripe"],
    ["role_family", "something_new", "something new"], // a value the map doesn't know yet
  ])("%s %s → %s", (facet, value, want) => {
    expect(label(facet, value)).toBe(want);
  });

  it.each([
    ["role_family=sales", "Role: Sales"],
    ["seniority=staff_plus", "Seniority: Staff+"],
    ["experience_min_years=7", "Needs 7+ years"],
    ["location", "Location"],
    ["closed", "Closed"],
  ])("hidden reason %s → %s", (raw, want) => {
    expect(reason(raw)).toBe(want);
  });
});

const lead = (over: Partial<Lead>): Lead =>
  ({ locations: [], workplaces: [], matched_location: "", ...over }) as Lead;

describe("row location (M2-08 AC1, AC2)", () => {
  it.each([
    [{ locations: ["Hybrid", "Paris"], workplaces: ["hybrid"] }, "Hybrid +1"],
    [{ locations: ["remote"], workplaces: ["remote"] }, "remote"],
    [{ locations: ["On-site"], workplaces: ["onsite"] }, "On-site"],
    [{ locations: ["Remote - EMEA"], workplaces: ["remote"] }, "Remote - EMEA"],
    [{ locations: ["Paris, France"], workplaces: ["hybrid"] }, "Paris, France · Hybrid"],
    [{ locations: ["Berlin"], workplaces: ["onsite", "remote"] }, "Berlin · On-site, Remote"],
    [{ locations: ["Berlin"], workplaces: ["unknown"] }, "Berlin"],
    [{ locations: [], workplaces: ["remote"] }, "No location · Remote"],
    [{ locations: ["Romania", "Brazil", "Morocco"], workplaces: ["remote"], matched_location: "Morocco" }, "Morocco +2 · Remote"],
    [{ locations: ["Romania", "Brazil"], workplaces: ["remote"], matched_location: "" }, "Romania +1 · Remote"],
  ] as [Partial<Lead>, string][])("%j → %s", (over, want) => {
    expect(place(lead(over))).toBe(want);
  });
});
