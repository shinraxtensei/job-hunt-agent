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
    if (route.request().method() === "POST" && /^\/api\/leads\/\d+\/reason$/.test(path)) {
      return route.fulfill({ status: 202, contentType: "application/json", body: '{"requested":true}' });
    }
    const body =
      path === "/api/leads" ? leads
      : path === "/api/leads/9001" ? golden("lead")
      : path === "/api/leads/9006" ? golden("lead_stale_reason")
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
