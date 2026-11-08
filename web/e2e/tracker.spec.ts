import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";

// The API's own golden responses (internal/httpapi/testdata/golden), so a contract change
// there shows up here too.
const golden = (name: string) =>
  readFileSync(new URL(`../../internal/httpapi/testdata/golden/${name}.json`, import.meta.url), "utf8");

// M6-01: an applied lead, its history and the CV sent; the snapshot title carries markup.
const APPLICATION = {
  application: {
    id: 1,
    status: "applied",
    status_at: "2026-11-03T09:00:00Z",
    applied_at: "2026-11-03T09:00:00Z",
    snapshot: { title: "Senior Backend Engineer <img src=x onerror=alert(1)>", url: "https://jobs.lever.co/acme/1", content_hash: "ab" },
    history: [
      { status: "applied", at: "2026-11-03T09:00:00Z", auto: false },
      { status: "drafted", at: "2026-11-02T10:00:00Z", auto: true },
      { status: "shortlisted", at: "2026-11-02T09:00:00Z", auto: false },
    ],
    documents: [{ id: 9201, kind: "cv", sent_at: "2026-11-03T08:55:00Z" }],
    notes: [{ id: 1, body: "Call went well <b>really</b>\nNext: tech round", created_at: "2026-11-04T10:00:00Z", edited_at: null }],
    contacts: [{ id: 1, name: "Ann Smith", role: "Recruiter", email: "ann@example.com", linkedin: "https://www.linkedin.com/in/ann", phone: "" }],
  },
};

// M6-02: three applications; the rejected one sits in Done; one follow-up is due.
const BOARD = {
  cards: [
    { lead_id: 9001, company: "Acme", title: "Senior Backend Engineer", fit_level: "strong", status: "applied", status_at: "2026-10-25T09:00:00Z", followup_due: true },
    { lead_id: 9006, company: "Brick", title: "Mobile <b>Engineer</b>", fit_level: null, status: "interviewing", status_at: "2026-11-01T09:00:00Z", followup_due: false },
    { lead_id: 9003, company: "Gone", title: "Data Engineer", fit_level: "good", status: "rejected", status_at: "2026-10-20T09:00:00Z", followup_due: false },
  ],
};

async function mockAPI(page: Page, leads = golden("leads_default")) {
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() === "POST" && /^\/api\/leads\/\d+\/reason$/.test(path)) {
      return route.fulfill({ status: 202, contentType: "application/json", body: '{"requested":true}' });
    }
    if (["POST", "DELETE"].includes(route.request().method()) && /^\/api\/(leads\/\d+\/(draft|status|skip|notes|contacts)|notes\/\d+|contacts\/\d+|drafts\/\d+\/(approve|discard)|documents\/\d+\/sent|applications\/\d+\/followup)$/.test(path)) {
      return route.fulfill({ status: 200, contentType: "application/json", body: '{"status":"ok"}' });
    }
    const body =
      path === "/api/leads" ? leads
      : path === "/api/leads/9001" ? golden("lead")
      : path === "/api/leads/9006" ? golden("lead_stale_reason")
      : path === "/api/leads/9001/drafts" ? golden("drafts")
      : path === "/api/leads/9006/drafts" ? golden("drafts_blocked")
      : path === "/api/leads/9001/application" ? JSON.stringify(APPLICATION)
      : path.endsWith("/application") ? '{"application":null}'
      : path === "/api/leads/hidden" ? golden("hidden_default")
      : path === "/api/facets" ? golden("facets_default")
      : path === "/api/presets" ? golden("presets")
      : path === "/api/followups" ? '{"followups":[]}'
      : path === "/api/board" ? JSON.stringify(BOARD)
      : path.endsWith("/members") ? golden("members")
      : null;
    return body
      ? route.fulfill({ contentType: "application/json", body })
      : route.fulfill({ status: 404, contentType: "application/json", body: '{"error":{"message":"no such endpoint"}}' });
  });
}

async function noHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow, "page scrolls horizontally").toBeLessThanOrEqual(0);
}

async function noAxeViolations(page: Page) {
  // Let open/reveal animations finish: mid-fade text measures as low contrast.
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running"));
  const { violations } = await new AxeBuilder({ page }).analyze();
  expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target).join(" ")})`)).toEqual([]);
}

const summary = (page: Page) => page.getByRole("status").filter({ hasText: "shown" });

for (const width of [375, 1280]) {
  for (const scheme of ["light", "dark"] as const) {
    test.describe(`${width}px ${scheme}`, () => {
      test.use({ viewport: { width, height: 900 }, colorScheme: scheme });

      test("lists the default leads with the hidden count and no axe violations", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/");
        await expect(summary(page)).toHaveText("2 shown · 3 hidden by these filters");
        await expect(page.getByRole("link", { name: "Senior Backend Engineer" })).toHaveAttribute("rel", "noopener noreferrer");
        await expect(page.getByRole("button", { name: /Filters \(5\)/ })).toBeVisible();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      test("the filter sheet lists what is applied and resets", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?new=true");
        await page.getByRole("button", { name: /Filters/ }).click();
        const sheet = page.getByRole("dialog", { name: "Filters" });
        await expect(sheet.getByText("New: last 24 h")).toBeVisible();
        await expect(sheet.getByRole("checkbox", { name: /^Backend/ })).toBeChecked(); // from the preset, labelled
        await noAxeViolations(page);
        await sheet.getByRole("button", { name: "Reset to my defaults" }).click();
        await expect(page).toHaveURL(/\/$/);
        await page.keyboard.press("Escape");
        await expect(sheet).toBeHidden();
        await noHorizontalScroll(page);
      });

      test("the hidden view says why each lead is hidden", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/");
        await page.getByRole("link", { name: "3 hidden by these filters" }).click();
        await expect(page).toHaveURL(/view=hidden/);
        await expect(page.getByText("hidden by Role: Sales, Location")).toBeVisible();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      test("a lead with several postings expands to its members", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/");
        const button = page.getByRole("button", { name: "Show details and 2 postings" });
        await button.click();
        await expect(button).toHaveAttribute("aria-expanded", "true");
        await expect(page.getByRole("list", { name: "2 postings of this role" }).getByText(/Remote - EMEA/)).toBeVisible();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      // M3-08 AC4: best fit first; the row's fit in words and numbers; the expanded row
      // shows the reason, the gaps and the breakdown with numbers.
      test("a scored lead shows its fit, reason, gaps and breakdown", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/");
        const rows = page.locator("tr.row");
        await expect(rows.first()).toContainText("Senior Backend Engineer"); // 60.0 before 20.0
        await expect(rows.first().locator("td.fit-cell")).toHaveText("Strong fit, score 60.0");
        await rows.first().getByRole("button", { name: /Show details/ }).click();
        const fit = page.getByRole("region", { name: "Fit" });
        await expect(fit.getByText("Strong fit · 60.0 of 100 · CV: Backend")).toBeVisible();
        await expect(fit.getByText("Your backend stack fits. Go is missing.")).toBeVisible();
        await expect(fit.getByRole("list", { name: "Gaps" }).getByRole("listitem")).toHaveText(["Go"]);
        await expect(fit.getByRole("table", { name: /Score breakdown/ }).getByRole("row")).toHaveCount(6);
        await expect(fit.getByRole("row", { name: /Required skills/ })).toContainText("22.1 of 35");
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      // M4-05: a lead's CV draft opens in a dialog: status, sentences with their evidence,
      // findings in place, coverage with the possible injection flagged; approve records
      // the decision (a same-origin JSON POST); nothing is sent.
      test("a CV draft is reviewed next to its evidence", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/");
        await page.locator("tr.row", { hasText: "Senior Backend Engineer" }).getByRole("button", { name: /Show details/ }).click();
        await page.getByRole("button", { name: "CV draft" }).click();
        const dialog = page.getByRole("dialog", { name: /CV draft/ });
        await expect(dialog.getByRole("status").filter({ hasText: "Ready for review" })).toBeVisible();
        await expect(dialog.getByText("Cut the invoice job from 40 to 6 minutes.")).toBeVisible();
        await expect(dialog.getByText("Led <img src=x onerror=alert(1)> the move to Docker images.")).toBeVisible();
        await expect(dialog.locator("img")).toHaveCount(0);
        await dialog.locator("li", { hasText: "Cut the invoice job" }).getByRole("button", { name: "Evidence (1)" }).click();
        await expect(dialog.getByText(/cutting its run time from 40 to 6 minutes/)).toBeVisible();
        await expect(dialog.getByText(/A stronger verb than the evidence uses/)).toBeVisible();
        await expect(dialog.getByText(/possible injection, ignored/)).toBeVisible();
        // M5-01: the letter under the CV, its sentences with their evidence, markup as text
        const letter = dialog.getByRole("region", { name: "Cover letter" });
        await expect(letter.getByText("Dear Hiring Team,")).toBeVisible();
        await expect(letter.getByText("I am writing to apply for the Backend Engineer role at Example Corp.")).toBeVisible();
        await expect(letter.getByText("I am available to start <b>within</b> one month.")).toBeVisible();
        await letter.locator("li", { hasText: "available to start" }).getByRole("button", { name: "Evidence (1)" }).click();
        await expect(letter.getByText("Available to start within one month.")).toBeVisible();
        // M5-04: the exported PDFs; the sent CV is marked, the letter can be marked sent
        const docs = dialog.getByRole("region", { name: "Documents" });
        await expect(docs.locator("li", { hasText: "CV PDF" })).toContainText("Sent");
        await expect(docs.locator("li", { hasText: "CV PDF" }).getByRole("link", { name: "Download" })).toHaveAttribute("href", "/api/documents/9201/pdf");
        const sent = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/documents/9202/sent"));
        await docs.locator("li", { hasText: "Cover letter PDF" }).getByRole("button", { name: "Mark as sent" }).click();
        expect((await sent).headers()["content-type"]).toBe("application/json");
        await expect(dialog.getByLabel("Version")).toBeVisible(); // two versions
        await noHorizontalScroll(page);
        await noAxeViolations(page);
        const approve = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/drafts/9102/approve"));
        await dialog.getByRole("button", { name: "Approve" }).click();
        expect((await approve).headers()["content-type"]).toBe("application/json");
      });

      // M6-01: the expanded lead shows its status (a same-origin JSON write to change it),
      // the history with automatic changes marked, the posting as applied to, the CV sent.
      test("a lead's status and history", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/");
        await page.locator("tr.row", { hasText: "Senior Backend Engineer" }).getByRole("button", { name: /Show details/ }).click();
        const app = page.getByRole("region", { name: "Application" });
        await expect(app.getByLabel("Status")).toHaveValue("applied");
        await expect(app.getByText("Senior Backend Engineer <img src=x onerror=alert(1)>")).toBeVisible();
        await expect(app.locator("img")).toHaveCount(0);
        await expect(app.getByRole("link", { name: "CV" })).toHaveAttribute("href", "/api/documents/9201/pdf");
        await app.getByText("History (3)").click();
        await expect(app.getByText(/Drafted ·.*· automatic/)).toBeVisible();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
        // M6-03: notes and contacts as text (markup inert, addresses not links); adding sends JSON
        await expect(app.getByText("Call went well <b>really</b>", { exact: false })).toBeVisible();
        await expect(app.getByRole("region", { name: "Contacts" }).getByRole("link")).toHaveCount(0);
        const note = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9001/notes"));
        await app.getByLabel("New note").fill("Sent the take-home");
        await app.getByRole("button", { name: "Add note" }).click();
        expect((await note).postDataJSON()).toEqual({ body: "Sent the take-home" });
        page.once("dialog", (d) => void d.accept());
        const del = page.waitForRequest((r) => r.method() === "DELETE" && r.url().endsWith("/api/contacts/1"));
        await app.getByRole("region", { name: "Contacts" }).getByRole("button", { name: "Delete" }).click();
        await del;
        const sent = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9001/status"));
        await app.getByLabel("Status").selectOption("interviewing");
        const req = await sent;
        expect(req.headers()["content-type"]).toBe("application/json");
        expect(req.postDataJSON()).toEqual({ status: "interviewing" });
      });

      // M6-05: one click skips; the answer's previous status powers Undo; a reason can follow.
      test("a lead is skipped with a reason, and undone", async ({ page }) => {
        let app = '{"application":null}';
        await mockAPI(page);
        await page.route("**/api/leads/9006/application", (route) => route.fulfill({ contentType: "application/json", body: app }));
        await page.route("**/api/leads/9006/skip", (route) =>
          route.fulfill({ contentType: "application/json", body: '{"status":"skipped","previous":"new"}' }),
        );
        await page.goto("/");
        await page.locator("tr.row", { hasText: "Mobile Engineer" }).getByRole("button", { name: /Show details/ }).click();
        const panel = page.getByRole("region", { name: "Application" });
        const skip = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9006/skip"));
        app = JSON.stringify({
          application: { id: 2, status: "skipped", status_at: "2026-11-08T10:00:00Z", applied_at: null, snapshot: null, history: [{ status: "skipped", at: "2026-11-08T10:00:00Z", auto: false }], documents: [], notes: [], contacts: [] },
        });
        await panel.getByRole("button", { name: "Skip" }).click();
        expect((await skip).postDataJSON()).toEqual({});
        const why = panel.getByRole("group", { name: "Why skip it?" });
        await expect(why).toBeVisible();
        const reason = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9006/skip"));
        await why.getByRole("button", { name: "Visa / relocation" }).click();
        expect((await reason).postDataJSON()).toEqual({ reason: "visa_relocation", note: "" });
        await expect(why.getByRole("button", { name: "Visa / relocation" })).toHaveAttribute("aria-pressed", "true");
        await noHorizontalScroll(page);
        await noAxeViolations(page);
        const undo = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9006/status"));
        await why.getByRole("button", { name: "Undo" }).click();
        expect((await undo).postDataJSON()).toEqual({ status: "new" });
      });

      // M6-04: due follow-ups above the leads, with the message to copy; Done and Snooze are
      // same-origin JSON writes; nothing is sent. The title's markup stays text.
      test("a follow-up is due, copied and marked done", async ({ page, context }) => {
        await context.grantPermissions(["clipboard-read", "clipboard-write"]);
        await mockAPI(page);
        const message = "Hello Ann Smith,\n\nI applied for the Backend Engineer role at Acme on 25 October 2026 and wanted to follow up.";
        await page.route("**/api/followups", (route) =>
          route.fulfill({
            contentType: "application/json",
            body: JSON.stringify({
              followups: [{ application_id: 7, lead_id: 9001, company: "Acme", title: "Backend <i>Engineer</i>", applied_at: "2026-10-25T09:00:00Z", days: 8, contact: "Ann Smith", message }],
            }),
          }),
        );
        await page.goto("/");
        const due = page.getByRole("region", { name: /Follow-ups due \(1\)/ });
        await expect(due.getByText("Backend <i>Engineer</i>", { exact: false })).toBeVisible();
        await expect(due.locator("i")).toHaveCount(0);
        await due.getByRole("button", { name: "Copy message" }).click();
        await expect(due.getByRole("button", { name: "Copied" })).toBeVisible();
        expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(message);
        await noHorizontalScroll(page);
        await noAxeViolations(page);
        const snooze = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/applications/7/followup"));
        await due.getByRole("button", { name: "Snooze 3 days" }).click();
        expect((await snooze).postDataJSON()).toEqual({ action: "snooze", days: 3 });
      });

      // M6-02: applications by status; a card moves with its menu (keyboard) or by drag and
      // drop, both through the status endpoint; at 375 px the columns stack, no side scroll.
      test("the board moves an application", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?view=board");
        const applied = page.getByRole("region", { name: /^Applied/ });
        await expect(applied.locator(".card-title")).toHaveText("Acme · Senior Backend Engineer");
        await expect(applied.getByText("Follow-up due", { exact: false })).toBeVisible();
        await expect(page.getByRole("region", { name: /^Interviewing/ }).locator(".card-title")).toHaveText("Brick · Mobile <b>Engineer</b>");
        await expect(page.getByRole("region", { name: /^Done/ }).locator(".card .cv-dates")).toContainText("Rejected");
        await noHorizontalScroll(page);
        await noAxeViolations(page);
        const menu = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9001/status"));
        await page.getByLabel("Move Acme Senior Backend Engineer to").selectOption("replied");
        expect((await menu).postDataJSON()).toEqual({ status: "replied" });
        const dragged = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9006/status"));
        await page.locator(".card", { hasText: "Brick" }).dragTo(page.getByRole("region", { name: /^Offer/ }));
        expect((await dragged).postDataJSON()).toEqual({ status: "offer" });
      });

      test("a blocked draft can't be approved", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/");
        await page.locator("tr.row", { hasText: "Mobile Engineer" }).getByRole("button", { name: /Show details/ }).click();
        await page.getByRole("button", { name: "CV draft" }).click();
        const dialog = page.getByRole("dialog", { name: /CV draft/ });
        await expect(dialog.getByText("Blocked: a sentence breaks the evidence rules")).toBeVisible();
        await expect(dialog.getByText(/Blocks the draft:.*A number that isn’t in the cited evidence/)).toBeVisible();
        await expect(dialog.getByRole("button", { name: "Approve" })).toBeDisabled();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      // M3-08 AC4, AC5, AC6: not enough info and an older reason are said plainly; opening
      // the lead asks for a fresh reason; an HTML payload in the reason stays text.
      test("an older reason is marked and a fresh one requested", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/");
        const posted = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9006/reason"));
        await page.locator("tr.row", { hasText: "Mobile Engineer" }).getByRole("button", { name: /Show details/ }).click();
        const request = await posted;
        expect(request.headers()["content-type"]).toBe("application/json");
        const fit = page.getByRole("region", { name: "Fit" });
        await expect(fit.getByText(/^Not enough info/)).toBeVisible();
        await expect(fit.getByText("This reason is from older inputs. Updating…")).toBeVisible();
        await expect(fit.locator(".reason")).toHaveText('<script>alert(1)</script> <a href="javascript:alert(1)">x</a>.');
        await expect(fit.locator("script, a")).toHaveCount(0);
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });
    });
  }
}

// M2-08 AC4: at 375 px the search placeholder fits its field.
test("the search placeholder fits at 375px", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await mockAPI(page);
  await page.goto("/");
  const search = page.getByRole("searchbox");
  await expect(search).toHaveAttribute("placeholder", "Search");
  const fits = await search.evaluate((el: HTMLInputElement) => {
    const ctx = document.createElement("canvas").getContext("2d")!;
    const cs = getComputedStyle(el);
    ctx.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`;
    return ctx.measureText(el.placeholder).width <= el.clientWidth;
  });
  expect(fits, "placeholder wider than the field").toBe(true);
  await noAxeViolations(page);
});

// M2-08 AC1-AC3 in the real page: matched location first, no repeated workplace, labels.
test("rows show the matched location and labelled values", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await mockAPI(page);
  await page.goto("/");
  await expect(page.locator("tr.row", { hasText: "Senior Backend Engineer" }).locator("td.place")).toHaveText("Remote – Morocco +1");
  await expect(page.locator("tr.row", { hasText: "Mobile Engineer" }).locator("td.place")).toHaveText("Amsterdam · On-site");
});

test.describe("desktop behaviour", () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test("chips show applied filters; removing one goes back to the preset", async ({ page }) => {
    await mockAPI(page);
    await page.goto("/?role_family=sales");
    const chips = page.getByLabel("Applied filters");
    await expect(chips.getByText("Role Sales")).toBeVisible();
    await expect(chips.getByText("Seniority Junior, Mid, Senior, Unknown")).toBeVisible(); // from the preset, labelled
    await chips.getByRole("button", { name: "Remove Role filter" }).click();
    await expect(page).toHaveURL(/\/$/);
  });

  test("keyboard: j/k move, Enter opens, e expands, / searches, Esc clears, ? helps", async ({ page, context }) => {
    await mockAPI(page);
    await page.goto("/");
    await expect(summary(page)).toBeVisible();
    await page.keyboard.press("j");
    const rows = page.locator("tr.row");
    await expect(rows.nth(0)).toBeFocused();
    await page.keyboard.press("j");
    await expect(rows.nth(1)).toBeFocused();
    await expect(rows.nth(1)).toHaveClass(/focused/);
    await page.keyboard.press("k");
    await expect(rows.nth(0)).toBeFocused();

    const opened = context.waitForEvent("page");
    await page.keyboard.press("Enter");
    expect((await opened).url()).toMatch(/^https:\/\/jobs\.lever\.co\/x\//);

    await page.keyboard.press("e");
    await expect(page.locator("tr.detail")).toHaveCount(1);
    await page.keyboard.press("Escape");
    await expect(page.locator("tr.detail")).toHaveCount(0);

    await page.keyboard.press("/");
    await expect(page.getByRole("searchbox")).toBeFocused();
    await page.keyboard.type("backend");
    await expect(rows).toHaveCount(1);
    await expect(page).toHaveURL(/q=backend/); // the search is in the URL too
    await page.reload();
    await expect(page.getByRole("searchbox")).toHaveValue("backend");
    await expect(rows).toHaveCount(1);
    await page.getByRole("searchbox").focus();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("searchbox")).toHaveValue("");
    await expect(page).not.toHaveURL(/q=/);
    await expect(rows).toHaveCount(2);

    await page.getByRole("searchbox").blur();
    await page.keyboard.press("?");
    await expect(page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
    await noAxeViolations(page);
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeHidden();
  });

  test("the sidebar collapses to a rail with [ and remembers it", async ({ page }) => {
    await mockAPI(page);
    await page.goto("/");
    await expect(page.locator(".app")).not.toHaveClass(/rail/);
    await page.keyboard.press("[");
    await expect(page.locator(".app")).toHaveClass(/rail/);
    await page.reload();
    await expect(page.locator(".app")).toHaveClass(/rail/);
  });

  test("works when localStorage throws", async ({ page }) => {
    await page.addInitScript(() => {
      Object.defineProperty(window, "localStorage", { get: () => { throw new Error("blocked"); } });
    });
    await mockAPI(page);
    await page.goto("/");
    await expect(summary(page)).toBeVisible();
    await page.keyboard.press("[");
    await expect(page.locator(".app")).toHaveClass(/rail/);
  });
});

test("the sidebar starts as a rail below 1024px", async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 800 });
  await mockAPI(page);
  await page.goto("/");
  await expect(page.locator(".app")).toHaveClass(/rail/);
});

test.describe("states", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test("loading, then an error with a retry", async ({ page }) => {
    let calls = 0;
    await page.route("**/api/**", (route) => {
      calls++;
      return route.fulfill({ status: 500, contentType: "application/json", body: '{"error":{"message":"internal error"}}' });
    });
    await page.goto("/");
    await expect(page.getByRole("alert")).toContainText("Couldn’t load leads.");
    const before = calls;
    await page.getByRole("button", { name: "Try again" }).click();
    await expect.poll(() => calls).toBeGreaterThan(before);
    await noAxeViolations(page);
  });

  test("everything hidden", async ({ page }) => {
    await mockAPI(page, JSON.stringify({ total: 0, leads: [] }));
    await page.goto("/");
    await expect(page.getByText("Your filters hide all 3 leads.")).toBeVisible();
    await noAxeViolations(page);
  });
});

// WCAG AA for the token pairs used for text, in both themes (axe also checks the
// rendered page above).
test("token colours meet WCAG AA in light and dark", async ({ page }) => {
  await mockAPI(page);
  await page.goto("/");
  for (const theme of ["light", "dark"]) {
    const ratios = await page.evaluate((t) => {
      document.documentElement.setAttribute("data-theme", t);
      const css = getComputedStyle(document.documentElement);
      const rgb = (name: string) => {
        const hex = css.getPropertyValue(name).trim().slice(1);
        return [0, 2, 4].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
      };
      const lum = (c: number[]) => {
        const [r, g, b] = c.map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4)) as [number, number, number];
        return 0.2126 * r + 0.7152 * g + 0.0722 * b;
      };
      const ratio = (a: string, b: string) => {
        const [x, y] = [lum(rgb(a)), lum(rgb(b))].sort((p, q) => q - p) as [number, number];
        return (x + 0.05) / (y + 0.05);
      };
      const out: Record<string, number> = {};
      for (const fg of ["--text", "--muted", "--faint"])
        for (const bg of ["--bg", "--panel", "--hover", "--row-focus"]) out[`${fg} on ${bg}`] = ratio(fg, bg);
      out["--accent on --bg (non-text, 3:1)"] = ratio("--accent", "--bg");
      out["--accent on --row-focus (non-text, 3:1)"] = ratio("--accent", "--row-focus");
      return out;
    }, theme);
    for (const [pair, r] of Object.entries(ratios)) {
      expect(r, `${theme}: ${pair} = ${r.toFixed(2)}`).toBeGreaterThanOrEqual(pair.includes("3:1") ? 3 : 4.5);
    }
  }
});

// AC4: hostile posting text and URLs stay inert in the real browser.
test("an HTML title and a javascript: URL are inert", async ({ page }) => {
  const evil = JSON.parse(golden("leads_default"));
  evil.leads[0].title = `<img src=x onerror="alert(1)"><script>alert(2)</script>Hostile`;
  evil.leads[0].url = "javascript:alert(3)";
  await mockAPI(page, JSON.stringify(evil));
  let dialogs = 0;
  page.on("dialog", (d) => {
    dialogs++;
    void d.dismiss();
  });
  await page.goto("/");
  await expect(page.getByText(evil.leads[0].title)).toBeVisible();
  await expect(page.locator("table img, table script")).toHaveCount(0);
  await expect(page.locator('a[href^="javascript:"]')).toHaveCount(0);
  await page.getByText(evil.leads[0].title).click();
  await page.keyboard.press("Enter"); // o/Enter must not open a javascript: URL either
  expect(dialogs).toBe(0);
});
