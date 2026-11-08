// The one place raw facet values become words on the page (M2-08). Raw values stay in the
// URL and the API; anything this map doesn't know is shown with underscores as spaces.

const words: Record<string, Record<string, string>> = {
  skip_reason: {
    location: "Location",
    seniority: "Seniority",
    stack: "Stack",
    salary: "Salary",
    company: "Company",
    role_type: "Role type",
    visa_relocation: "Visa / relocation",
    language: "Language",
    not_interested: "Not interested",
    other: "Other",
  },
  status: {
    new: "New",
    shortlisted: "Shortlisted",
    drafted: "Drafted",
    applied: "Applied",
    replied: "Replied",
    interviewing: "Interviewing",
    offer: "Offer",
    rejected: "Rejected",
    closed: "Closed",
    skipped: "Skipped",
  },
  role_family: {
    backend: "Backend",
    frontend: "Frontend",
    fullstack: "Full-stack",
    mobile: "Mobile",
    platform: "Platform",
    data: "Data",
    ai_ml: "AI/ML",
    security: "Security",
    qa: "QA",
    eng_management: "Eng management",
    product: "Product",
    design: "Design",
    sales: "Sales",
    marketing: "Marketing",
    customer: "Customer",
    operations: "Operations",
    finance: "Finance",
    people: "People",
    legal: "Legal",
    other: "Other",
    unknown: "Unknown",
  },
  seniority: {
    intern: "Intern",
    junior: "Junior",
    mid: "Mid",
    senior: "Senior",
    staff_plus: "Staff+",
    lead_manager: "Lead/manager",
    unknown: "Unknown",
  },
  workplace: { remote: "Remote", hybrid: "Hybrid", onsite: "On-site", unknown: "Unknown" },
  remote_scope: {
    worldwide: "Worldwide",
    emea: "EMEA",
    europe: "Europe",
    africa: "Africa",
    americas: "Americas",
    apac: "APAC",
    us_only: "US only",
    unknown: "Unknown",
  },
  track: { remote: "Remote", relocation: "Relocation", morocco: "Morocco", consulting: "Consulting" },
  closed: { true: "Closed", false: "Open" },
  fit: { very_strong: "Very strong", strong: "Strong", good: "Good", stretch: "Stretch", no: "No fit", unscored: "Not scored" },
  factor: {
    required: "Required skills",
    title: "Title",
    seniority: "Seniority and years",
    location: "Location, remote, visa",
    optional: "Optional skills",
    freshness: "Freshness",
  },
  cv_variant: { BE: "Backend", PAY: "Payments", DIST: "Distributed systems", FS: "Full-stack", AI: "AI", CONS: "Consulting" },
};

const regions = new Intl.DisplayNames(["en"], { type: "region" });

function country(code: string): string {
  try {
    return regions.of(code) ?? code;
  } catch {
    return code; // not a region code
  }
}

// label is the display form of one facet value.
export function label(facet: string, value: string): string {
  if (facet === "country") return country(value);
  if (facet === "remote_scope" && value.startsWith("country:")) return country(value.slice("country:".length));
  if (facet === "company") return value;
  return words[facet]?.[value] ?? value.replaceAll("_", " ");
}

const reasonNames: Record<string, string> = { role_family: "Role", seniority: "Seniority" };

// reason is the display form of one of the API's hidden_by reasons ("role_family=sales").
export function reason(raw: string): string {
  const [facet, value] = raw.split("=", 2) as [string, string | undefined];
  if (facet === "experience_min_years" && value) return `Needs ${value}+ years`;
  if (value !== undefined) return `${reasonNames[facet] ?? facet}: ${label(facet, value)}`;
  return facet.charAt(0).toUpperCase() + facet.slice(1).replaceAll("_", " ");
}
