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
    if (["POST", "DELETE"].includes(route.request().method()) && /^\/api\/(leads\/\d+\/(draft|status|skip|reapply|notes|contacts)|notes\/\d+|contacts\/\d+|drafts\/\d+\/(approve|discard)|documents\/\d+\/sent|applications\/\d+\/followup|visits(\/seen)?)$/.test(path)) {
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
      : path === "/api/followups" ? golden("followups_none")
      : path === "/api/drafts" ? golden("recent_drafts")
      : path === "/api/today" ? golden("today_first_visit")
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


const row = (page: Page, text: string) => page.locator(".lead-row", { hasText: text });
// select a lead the way a person does: click the row (not its title link)
async function pick(page: Page, text: string) {
  await row(page, text).locator(".company").click();
  await expect(page.locator(".detail h2")).toContainText(text);
}

for (const width of [375, 1280]) {
  for (const scheme of ["light", "dark"] as const) {
    test.describe(`${width}px ${scheme}`, () => {
      test.use({ viewport: { width, height: 900 }, colorScheme: scheme });

      // M7-00: Today is the start page: stat tiles, then new leads, follow-ups, closed.
      test("Today shows tiles and sections, and opens a lead", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/");
        await expect(page.getByRole("link", { name: "Today" })).toHaveAttribute("aria-current", "page");
        const tiles = page.getByRole("list", { name: "At a glance" });
        await expect(tiles.getByRole("link")).toHaveCount(4);
        await expect(tiles.getByRole("link", { name: /Drafts ready 1/ })).toBeVisible(); // 9102 is ready
        // M7-01: opening Today records the visit; "new" is since the last one (the golden: first visit)
        await expect(tiles.getByRole("link", { name: /New since your last visit 1/ })).toBeVisible();
        await expect(page.getByRole("heading", { name: /New leads/ })).toBeVisible();
        await expect(page.locator(".lead-card")).toContainText("Mobile Engineer (Flutter)");
        const seen = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/visits/seen"));
        await page.getByRole("button", { name: "Mark all seen" }).click();
        expect((await seen).headers()["content-type"]).toBe("application/json");
        await expect(page.getByRole("heading", { name: /Follow-ups due/ })).toBeVisible();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
        await tiles.getByRole("link", { name: /Drafts ready/ }).click();
        await expect(page).toHaveURL(/page=drafts/);
        await expect(page.getByRole("heading", { name: "Drafts", level: 1 })).toBeVisible();
        await page.getByRole("link", { name: /Acme · Senior Backend Engineer/ }).click();
        await expect(page).toHaveURL(/page=leads&lead=9001&tab=cv/);
        await expect(page.getByRole("tab", { name: "CV & letter" })).toHaveAttribute("aria-selected", "true");
      });

      test("lists the default leads with the hidden count and no axe violations", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?page=leads");
        await expect(summary(page)).toHaveText("2 shown · 3 hidden by these filters");
        await expect(page.getByRole("link", { name: "Senior Backend Engineer" }).first()).toHaveAttribute("rel", "noopener noreferrer");
        await expect(page.getByRole("button", { name: /Filters \(5\)/ })).toBeVisible();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      test("the filter sheet lists what is applied and resets", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?page=leads&new=true");
        await page.getByRole("button", { name: /Filters/ }).click();
        const sheet = page.getByRole("dialog", { name: "Filters" });
        await expect(sheet.getByText("New: last 24 h")).toBeVisible();
        await expect(sheet.getByRole("checkbox", { name: /^Backend/ })).toBeChecked(); // from the preset, labelled
        await noAxeViolations(page);
        await sheet.getByRole("button", { name: "Reset to my defaults" }).click();
        await expect(page).toHaveURL(/\?page=leads(&lead=\d+)?$/);
        await page.keyboard.press("Escape");
        await expect(sheet).toBeHidden();
        await noHorizontalScroll(page);
      });

      test("the hidden view says why each lead is hidden", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?page=leads");
        await page.getByRole("link", { name: "3 hidden by these filters" }).click();
        await expect(page).toHaveURL(/view=hidden/);
        await expect(page.getByText("hidden by Role: Sales, Location").first()).toBeVisible();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      // M7-00: the row selects the lead; the detail shows beside the list on wide screens and
      // as a full-screen sheet with Back on the phone; the title stays a link.
      test("selecting a lead shows its detail, with Back on the phone", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?page=leads");
        await pick(page, "Mobile Engineer");
        await expect(row(page, "Mobile Engineer")).toHaveAttribute("aria-current", "true");
        await expect(page.locator(".detail h2").getByRole("link")).toHaveAttribute("target", "_blank");
        const back = page.getByRole("button", { name: "Back to leads" });
        if (width === 375) {
          await expect(back).toBeVisible();
          await expect(page.locator(".list-pane")).toBeHidden();
          await noHorizontalScroll(page);
          await noAxeViolations(page);
          await back.click();
          await expect(page.locator(".detail")).toBeHidden();
          await expect(row(page, "Mobile Engineer")).toBeFocused(); // back where you were
          await pick(page, "Mobile Engineer");
          await page.keyboard.press("Escape");
          await expect(page.locator(".detail")).toBeHidden();
        } else {
          await expect(back).toBeHidden();
          await expect(page.locator(".list-pane")).toBeVisible();
          await noHorizontalScroll(page);
          await noAxeViolations(page);
        }
      });

      test("a lead with several postings lists its members", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?page=leads");
        await pick(page, "Senior Backend Engineer");
        await expect(page.getByRole("list", { name: "2 postings of this role" }).getByText(/Remote - EMEA/)).toBeVisible();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      // M3-08 AC4 in the new design: best fit first; the row's fit in words and numbers; the
      // Overview shows the fit, the reason, the gaps and the breakdown with numbers.
      test("a scored lead shows its fit, reason, gaps and breakdown", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?page=leads");
        const rows = page.locator(".lead-row");
        await expect(rows.first()).toContainText("Senior Backend Engineer"); // 60.0 before 20.0
        await expect(rows.first().locator(".tag .visually-hidden")).toHaveText("Strong fit, score ");
        await expect(rows.first().locator(".tag")).toContainText("60.0");
        await pick(page, "Senior Backend Engineer");
        const fit = page.getByRole("region", { name: "Fit", exact: true });
        await expect(fit).toContainText("60.0 / 100");
        await expect(fit).toContainText("Strong fit · CV: Backend");
        const why = page.getByRole("region", { name: "Why this fit" });
        await expect(why.getByText("Your backend stack fits. Go is missing.")).toBeVisible();
        await expect(why.getByRole("list", { name: "Gaps" }).getByRole("listitem")).toHaveText(["Go"]);
        await expect(why.getByRole("table", { name: /Score breakdown/ }).getByRole("row")).toHaveCount(6);
        await expect(why.getByRole("row", { name: /Required skills/ })).toContainText("22.1 of 35");
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      // M4-05, M5-01, M5-04 in the "CV & letter" tab: sentences with their evidence, findings
      // in place, the letter, the PDFs; approve is a same-origin JSON POST; nothing is sent.
      test("a CV draft is reviewed next to its evidence", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?page=leads&lead=9001&tab=cv");
        const panel = page.getByRole("region", { name: "CV and letter" });
        await expect(panel.getByRole("status").filter({ hasText: "Ready for review" })).toBeVisible();
        await expect(panel.getByText("Cut the invoice job from 40 to 6 minutes.")).toBeVisible();
        await expect(panel.getByText("Led <img src=x onerror=alert(1)> the move to Docker images.")).toBeVisible();
        await expect(panel.locator("img")).toHaveCount(0);
        await panel.locator("li", { hasText: "Cut the invoice job" }).getByRole("button", { name: "Evidence (1)" }).click();
        await expect(panel.getByText(/cutting its run time from 40 to 6 minutes/)).toBeVisible();
        await expect(panel.getByText(/A stronger verb than the evidence uses/)).toBeVisible();
        await expect(panel.getByText(/possible injection, ignored/)).toBeVisible();
        const letter = panel.getByRole("region", { name: "Cover letter" });
        await expect(letter.getByText("Dear Hiring Team,")).toBeVisible();
        await expect(letter.getByText("I am writing to apply for the Backend Engineer role at Example Corp.")).toBeVisible();
        await expect(letter.getByText("I am available to start <b>within</b> one month.")).toBeVisible();
        await letter.locator("li", { hasText: "available to start" }).getByRole("button", { name: "Evidence (1)" }).click();
        await expect(letter.getByText("Available to start within one month.")).toBeVisible();
        const docs = panel.getByRole("region", { name: "Documents" });
        await expect(docs.locator("li", { hasText: "CV PDF" })).toContainText("Sent");
        await expect(docs.locator("li", { hasText: "CV PDF" }).getByRole("link", { name: "Download" })).toHaveAttribute("href", "/api/documents/9201/pdf");
        const sent = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/documents/9202/sent"));
        await docs.locator("li", { hasText: "Cover letter PDF" }).getByRole("button", { name: "Mark as sent" }).click();
        expect((await sent).headers()["content-type"]).toBe("application/json");
        await expect(panel.getByLabel("Version")).toBeVisible(); // two versions
        await noHorizontalScroll(page);
        await noAxeViolations(page);
        const approve = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/drafts/9102/approve"));
        await panel.getByRole("button", { name: "Approve" }).click();
        expect((await approve).headers()["content-type"]).toBe("application/json");
      });

      // M6-01, M6-03 in the tabs: status (a same-origin JSON write), history with automatic
      // changes marked, the posting as applied to, the CV sent; notes and contacts as text.
      test("a lead's status, history, notes and contacts", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?page=leads&lead=9001");
        const app = page.getByRole("region", { name: "Application" });
        await expect(app.getByLabel("Status")).toHaveValue("applied");
        await page.getByRole("tab", { name: "History" }).click();
        const history = page.getByRole("tabpanel");
        await expect(history.getByText("Senior Backend Engineer <img src=x onerror=alert(1)>")).toBeVisible();
        await expect(history.locator("img")).toHaveCount(0);
        await expect(history.getByRole("link", { name: "CV" })).toHaveAttribute("href", "/api/documents/9201/pdf");
        await expect(history.getByRole("list", { name: "History (3)" }).getByRole("listitem").nth(1)).toContainText("automatic");
        await noHorizontalScroll(page);
        await noAxeViolations(page);
        await page.getByRole("tab", { name: "Notes" }).click();
        const notes = page.getByRole("region", { name: "Notes" });
        await expect(notes.getByText("Call went well <b>really</b>", { exact: false })).toBeVisible();
        const note = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9001/notes"));
        await notes.getByLabel("New note").fill("Sent the take-home");
        await notes.getByRole("button", { name: "Add note" }).click();
        expect((await note).postDataJSON()).toEqual({ body: "Sent the take-home" });
        await page.getByRole("tab", { name: "Contacts" }).click();
        const contacts = page.getByRole("region", { name: "Contacts" });
        await expect(contacts.getByText("Ann Smith")).toBeVisible();
        await expect(contacts.getByRole("link")).toHaveCount(0);
        page.once("dialog", (d) => void d.accept());
        const del = page.waitForRequest((r) => r.method() === "DELETE" && r.url().endsWith("/api/contacts/1"));
        await contacts.getByRole("button", { name: "Delete" }).click();
        await del;
        await noAxeViolations(page);
        await page.getByRole("tab", { name: "Overview" }).click();
        const sent = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9001/status"));
        await page.getByRole("region", { name: "Application" }).getByLabel("Status").selectOption("interviewing");
        const req = await sent;
        expect(req.headers()["content-type"]).toBe("application/json");
        expect(req.postDataJSON()).toEqual({ status: "interviewing" });
      });

      // M6-05: one click skips; the answer's previous status powers Undo; a reason can follow.
      test("a lead is skipped with a reason, and undone", async ({ page }) => {
        // the application turns skipped only once the skip arrives (no race with the first load)
        const skipped = JSON.stringify({
          application: { id: 2, status: "skipped", status_at: "2026-11-08T10:00:00Z", applied_at: null, snapshot: null, history: [{ status: "skipped", at: "2026-11-08T10:00:00Z", auto: false }], documents: [], notes: [], contacts: [] },
        });
        let app = '{"application":null}';
        await mockAPI(page);
        await page.route("**/api/leads/9006/application", (route) => route.fulfill({ contentType: "application/json", body: app }));
        await page.route("**/api/leads/9006/skip", (route) => {
          app = skipped;
          return route.fulfill({ contentType: "application/json", body: '{"status":"skipped","previous":"new"}' });
        });
        await page.goto("/?page=leads&lead=9006");
        const panel = page.getByRole("region", { name: "Application" });
        const skip = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9006/skip"));
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

      // M6-04: the Follow-ups page has the message to copy; Done and Snooze are same-origin
      // JSON writes; nothing is sent. The title's markup stays text.
      test("a follow-up is due, copied and snoozed", async ({ page, context }) => {
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
        await page.goto("/?page=followups");
        const due = page.locator(".card", { hasText: "Acme" });
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
        await page.goto("/?page=board");
        const applied = page.getByRole("region", { name: /^Applied/ });
        await expect(applied.locator(".card-title")).toHaveText("Acme · Senior Backend Engineer");
        await expect(applied.getByText("Follow-up due")).toBeVisible();
        await expect(page.getByRole("region", { name: /^Interviewing/ }).locator(".card-title")).toHaveText("Brick · Mobile <b>Engineer</b>");
        await expect(page.getByRole("region", { name: /^Done/ }).locator(".card-tags")).toContainText("Rejected");
        await noHorizontalScroll(page);
        await noAxeViolations(page);
        const menu = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9001/status"));
        await page.getByLabel("Move Acme Senior Backend Engineer to").selectOption("replied");
        expect((await menu).postDataJSON()).toEqual({ status: "replied" });
        const dragged = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9006/status"));
        await page.locator(".card", { hasText: "Brick" }).dragTo(page.getByRole("region", { name: /^Offer/ }));
        expect((await dragged).postDataJSON()).toEqual({ status: "offer" });
      });

      // M9-04: Done holds three statuses, so dropping on it asks which; Cancel moves nothing,
      // Skipped goes through the skip endpoint (it records the score, M6-05).
      test("dropping on Done asks rejected, skipped or closed", async ({ page }) => {
        // Drag and drop is a pointer interaction: on a phone a card moves with its menu.
        test.skip((page.viewportSize()?.width ?? 0) < 768, "no drag and drop on a phone");
        await mockAPI(page);
        await page.goto("/?page=board");
        const done = page.getByRole("region", { name: /^Done/ });
        const acme = page.locator(".card", { hasText: "Acme" });
        const posts: string[] = [];
        page.on("request", (r) => {
          if (r.method() === "POST") posts.push(r.url());
        });
        await acme.dragTo(done);
        const ask = page.getByRole("dialog", { name: "Why is it done?" });
        await expect(ask).toBeVisible();
        await noAxeViolations(page);
        await ask.getByRole("button", { name: "Cancel" }).click();
        await expect(ask).toBeHidden();
        expect(posts).toEqual([]);
        await acme.dragTo(done);
        const rejected = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9001/status"));
        await ask.getByRole("button", { name: "Rejected" }).click();
        expect((await rejected).postDataJSON()).toEqual({ status: "rejected" });
        await page.locator(".card", { hasText: "Brick" }).dragTo(done);
        const skipped = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9006/skip"));
        await ask.getByRole("button", { name: "Skipped" }).click();
        await skipped;
      });

      // M9-05: a rejected or closed application can be set aside to apply again; earlier
      // ones are listed in History.
      test("apply again after a rejection", async ({ page }) => {
        await mockAPI(page);
        await page.route("**/api/leads/9001/application", (route) =>
          route.fulfill({
            contentType: "application/json",
            body: JSON.stringify({
              application: { ...APPLICATION.application, status: "rejected", status_at: "2026-11-05T09:00:00Z" },
              previous: [{ id: 3, status: "closed", status_at: "2026-06-01T09:00:00Z", applied_at: "2026-05-20T09:00:00Z", archived_at: "2026-06-02T09:00:00Z" }],
            }),
          }),
        );
        await page.goto("/?page=leads&lead=9001");
        const again = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9001/reapply"));
        await page.getByRole("button", { name: "Apply again" }).click();
        await again;
        await page.getByRole("tab", { name: "History" }).click();
        await expect(page.getByText(/Earlier application: Closed/)).toBeVisible();
        await noAxeViolations(page);
      });

      test("a blocked draft can't be approved", async ({ page }) => {
        await mockAPI(page);
        await page.goto("/?page=leads&lead=9006&tab=cv");
        const panel = page.getByRole("region", { name: "CV and letter" });
        await expect(panel.getByText("Blocked: a sentence breaks the evidence rules")).toBeVisible();
        await expect(panel.getByText(/Blocks the draft:.*A number that isn’t in the cited evidence/)).toBeVisible();
        await expect(panel.getByRole("button", { name: "Approve" })).toBeDisabled();
        await noHorizontalScroll(page);
        await noAxeViolations(page);
      });

      // M3-08 AC4-AC6: not enough info and an older reason are said plainly; opening the lead
      // asks for a fresh reason; an HTML payload in the reason stays text.
      test("an older reason is marked and a fresh one requested", async ({ page }) => {
        await mockAPI(page);
        const posted = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/leads/9006/reason"));
        await page.goto("/?page=leads&lead=9006");
        const request = await posted;
        expect(request.headers()["content-type"]).toBe("application/json");
        const why = page.getByRole("region", { name: "Why this fit" });
        await expect(why.getByText(/^Not enough info/)).toBeVisible();
        await expect(why.getByText("This reason is from older inputs. Updating…")).toBeVisible();
        await expect(why.locator(".reason")).toHaveText('<script>alert(1)</script> <a href="javascript:alert(1)">x</a>.');
        await expect(why.locator("script, a")).toHaveCount(0);
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
  await page.goto("/?page=leads");
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
  await page.goto("/?page=leads");
  await expect(row(page, "Senior Backend Engineer").locator(".place")).toHaveText("Remote – Morocco +1");
  await expect(row(page, "Mobile Engineer").locator(".place")).toHaveText("Amsterdam · On-site");
});

test.describe("desktop behaviour", () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test("pill navigation reaches every page, in the URL", async ({ page }) => {
    await mockAPI(page);
    await page.goto("/");
    for (const [name, id] of [
      ["Leads", "leads"],
      ["Board", "board"],
      ["Follow-ups", "followups"],
      ["Drafts", "drafts"],
      ["Today", "today"],
    ]) {
      await page.getByRole("navigation", { name: "Pages" }).getByRole("link", { name }).click();
      await expect(page).toHaveURL(new RegExp(`page=${id}`));
      await expect(page.getByRole("link", { name, exact: true }).first()).toHaveAttribute("aria-current", "page");
    }
    await page.goBack();
    await expect(page).toHaveURL(/page=drafts/);
  });

  test("chips show applied filters; removing one goes back to the preset", async ({ page }) => {
    await mockAPI(page);
    await page.goto("/?page=leads&role_family=sales");
    const chips = page.getByLabel("Applied filters");
    await expect(chips.getByText("Role Sales")).toBeVisible();
    await expect(chips.getByText("Seniority Junior, Mid, Senior, Unknown")).toBeVisible(); // from the preset, labelled
    await chips.getByRole("button", { name: "Remove Role filter" }).click();
    await expect(page).toHaveURL(/\?page=leads(&lead=\d+)?$/);
  });

  test("keyboard: j/k move and the detail follows, Enter opens, / searches, Esc clears, ? helps", async ({ page, context }) => {
    await mockAPI(page);
    await page.goto("/?page=leads");
    await expect(summary(page)).toBeVisible();
    const rows = page.locator(".lead-row");
    await expect(rows.nth(0)).toHaveAttribute("aria-current", "true"); // the first lead shows beside the list
    await page.keyboard.press("j");
    await expect(rows.nth(1)).toBeFocused();
    await expect(rows.nth(1)).toHaveAttribute("aria-current", "true");
    await expect(page.locator(".detail h2")).toContainText("Mobile Engineer");
    await expect(page).toHaveURL(/lead=9006/);
    await page.keyboard.press("k");
    await expect(rows.nth(0)).toBeFocused();
    await expect(page.locator(".detail h2")).toContainText("Senior Backend Engineer");

    const opened = context.waitForEvent("page");
    await page.keyboard.press("Enter");
    expect((await opened).url()).toMatch(/^https:\/\/jobs\.lever\.co\/x\//);

    // the detail's tabs follow the ARIA pattern
    await page.getByRole("tab", { name: "Overview" }).focus();
    await page.keyboard.press("ArrowRight");
    await expect(page.getByRole("tab", { name: "CV & letter" })).toBeFocused();
    await expect(page.getByRole("tab", { name: "CV & letter" })).toHaveAttribute("aria-selected", "true");

    await rows.nth(0).focus();
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

  test("the title link opens the posting without selecting the row", async ({ page, context }) => {
    await mockAPI(page);
    await page.goto("/?page=leads");
    await expect(row(page, "Senior Backend Engineer")).toHaveAttribute("aria-current", "true");
    const opened = context.waitForEvent("page");
    await row(page, "Mobile Engineer").getByRole("link").click();
    expect((await opened).url()).toMatch(/^https:\/\//);
    await expect(row(page, "Senior Backend Engineer")).toHaveAttribute("aria-current", "true");
  });

  test("works when localStorage throws", async ({ page }) => {
    await page.addInitScript(() => {
      Object.defineProperty(window, "localStorage", { get: () => { throw new Error("blocked"); } });
    });
    await mockAPI(page);
    await page.goto("/?page=leads");
    await expect(summary(page)).toBeVisible();
    await page.getByRole("button", { name: /^Theme/ }).click();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  });
});

test.describe("states", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test("loading, then an error with a retry", async ({ page }) => {
    let calls = 0;
    await page.route("**/api/**", (route) => {
      calls++;
      return route.fulfill({ status: 500, contentType: "application/json", body: '{"error":{"message":"internal error"}}' });
    });
    await page.goto("/?page=leads");
    await expect(page.getByRole("alert")).toContainText("Couldn’t load leads.");
    const before = calls;
    await page.getByRole("button", { name: "Try again" }).click();
    await expect.poll(() => calls).toBeGreaterThan(before);
    await noAxeViolations(page);
  });

  test("everything hidden", async ({ page }) => {
    await mockAPI(page, JSON.stringify({ total: 0, leads: [] }));
    await page.goto("/?page=leads");
    await expect(page.getByText("Your filters hide all 3 leads.")).toBeVisible();
    await noAxeViolations(page);
  });

  test("links from before M7-00 still land on the right page", async ({ page }) => {
    await mockAPI(page);
    await page.goto("/?view=board");
    await expect(page.getByRole("heading", { name: "Board", level: 1 })).toBeVisible();
    await page.goto("/?role_family=sales");
    await expect(page.getByLabel("Applied filters").getByText("Role Sales")).toBeVisible();
  });
});

// WCAG AA for the token pairs used for text, in both themes (axe also checks the rendered
// pages above): text on surfaces, near-black text on every pastel, the accent at 3:1.
test("token colours meet WCAG AA in light and dark", async ({ page }) => {
  await mockAPI(page);
  await page.goto("/");
  for (const theme of ["light", "dark"]) {
    const ratios = await page.evaluate((t) => {
      document.documentElement.setAttribute("data-theme", t);
      const css = getComputedStyle(document.documentElement);
      const rgb = (name: string) => {
        let hex = css.getPropertyValue(name).trim().slice(1);
        if (hex.length === 3) hex = [...hex].map((c) => c + c).join(""); // the minifier shortens #ffffff
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
        for (const bg of ["--bg", "--surface", "--surface-2", "--hover"]) out[`${fg} on ${bg}`] = ratio(fg, bg);
      for (const bg of ["--mint", "--sand", "--salmon", "--blue", "--lilac"]) out[`--on-pastel on ${bg}`] = ratio("--on-pastel", bg);
      for (const bg of ["--bg", "--surface", "--surface-2"]) out[`--accent on ${bg} (non-text, 3:1)`] = ratio("--accent", bg);
      return out;
    }, theme);
    for (const [pair, r] of Object.entries(ratios)) {
      expect(r, `${theme}: ${pair} = ${r.toFixed(2)}`).toBeGreaterThanOrEqual(pair.includes("3:1") ? 3 : 4.5);
    }
  }
});

// AC4 (M2-06): hostile posting text and URLs stay inert in the real browser.
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
  await page.goto("/?page=leads");
  await expect(page.getByText(evil.leads[0].title).first()).toBeVisible();
  await expect(page.locator(".lead-list img, .lead-list script, .detail img, .detail script")).toHaveCount(0);
  await expect(page.locator('a[href^="javascript:"]')).toHaveCount(0);
  await page.getByText(evil.leads[0].title).first().click();
  await page.keyboard.press("Enter"); // o/Enter must not open a javascript: URL either
  expect(dialogs).toBe(0);
});
