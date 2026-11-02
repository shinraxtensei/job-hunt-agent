import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";

// The API's own golden responses (internal/httpapi/testdata/golden), so a contract change
// there shows up here too.
const golden = (name: string) =>
  readFileSync(new URL(`../../internal/httpapi/testdata/golden/${name}.json`, import.meta.url), "utf8");

async function mockAPI(page: Page, leads = golden("leads_default")) {
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    const body =
      path === "/api/leads" ? leads
      : path === "/api/leads/hidden" ? golden("hidden_default")
      : path === "/api/facets" ? golden("facets_default")
      : path === "/api/presets" ? golden("presets")
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
  const { violations } = await new AxeBuilder({ page }).analyze();
  expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.length})`)).toEqual([]);
}

for (const width of [375, 1280]) {
  test.describe(`at ${width}px`, () => {
    test.use({ viewport: { width, height: 900 } });

    test("lists the default leads with filters, reset and the hidden line", async ({ page }) => {
      await mockAPI(page);
      await page.goto("/");
      await expect(page.getByText("2 leads. These filters hide 3.")).toBeVisible();
      await expect(page.getByRole("link", { name: "Senior Backend Engineer" })).toHaveAttribute("rel", "noopener noreferrer");
      await expect(page.getByRole("checkbox", { name: /backend \(1\)/ })).toBeChecked(); // from the default preset
      await expect(page.getByRole("button", { name: "Reset to my defaults" })).toBeVisible();
      await noHorizontalScroll(page);
      await noAxeViolations(page);
    });

    test("filters live in the URL", async ({ page }) => {
      await mockAPI(page);
      await page.goto("/");
      await page.getByRole("checkbox", { name: /New/ }).check();
      await expect(page).toHaveURL(/new=true/);
      await page.getByRole("button", { name: "Reset to my defaults" }).click();
      await expect(page).toHaveURL(/\/$/);
    });

    test("the hidden view says why each lead is hidden", async ({ page }) => {
      await mockAPI(page);
      await page.goto("/");
      await page.getByRole("link", { name: "See what is hidden, and why" }).click();
      await expect(page).toHaveURL(/view=hidden/);
      await expect(page.getByText("role_family=sales, location")).toBeVisible();
      await noHorizontalScroll(page);
      await noAxeViolations(page);
    });

    test("a lead with several postings expands to its members", async ({ page }) => {
      await mockAPI(page);
      await page.goto("/");
      await page.getByRole("button", { name: "2 postings" }).click();
      await expect(page.getByRole("button", { name: "2 postings" })).toHaveAttribute("aria-expanded", "true");
      await expect(page.getByRole("listitem").filter({ hasText: "Remote - EMEA" })).toBeVisible(); // a member, not the row
      await noHorizontalScroll(page);
      await noAxeViolations(page);
    });
  });
}

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
  expect(dialogs).toBe(0);
});
