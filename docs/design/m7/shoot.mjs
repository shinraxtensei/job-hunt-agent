// Committed screenshots of the redesign (M7-00) with fixture data only: the API's golden
// files (Acme, Brick, Sam Example), served through Playwright mocks to the built page.
// Run: (cd web && npx vite preview --port 4173 &) ; node docs/design/m7/shoot.mjs
import { createRequire } from "node:module";
import { mkdirSync, readFileSync } from "node:fs";

const { chromium } = createRequire(new URL("../../../web/package.json", import.meta.url))("@playwright/test");
const golden = (n) => readFileSync(new URL(`../../../internal/httpapi/testdata/golden/${n}.json`, import.meta.url), "utf8");
const out = new URL("../screens/m7-00/", import.meta.url);
mkdirSync(out, { recursive: true });
const routes = {
  "/api/leads": golden("leads_default"),
  "/api/leads/hidden": golden("hidden_default"),
  "/api/facets": golden("facets_default"),
  "/api/presets": golden("presets"),
  "/api/leads/9001": golden("lead"),
  "/api/leads/9001/drafts": golden("drafts"),
  "/api/leads/9001/members": golden("members"),
  "/api/drafts": golden("recent_drafts"),
  "/api/board": golden("board_empty"),
  "/api/followups": JSON.stringify({
    followups: [{ application_id: 7, lead_id: 9001, company: "Acme", title: "Senior Backend Engineer", applied_at: "2026-10-25T09:00:00Z", days: 8, contact: "Ann Smith", message: "Hello Ann Smith, …" }],
  }),
};
const browser = await chromium.launch();
for (const [name, url, widths] of [
  ["today", "/?page=today", [1280, 375]],
  ["leads", "/?page=leads&lead=9001", [1280, 375]],
  ["sheet", "/?page=leads&lead=9001", [375]],
  ["cv", "/?page=leads&lead=9001&tab=cv", [1280]],
]) {
  for (const scheme of ["light", "dark"]) {
    for (const width of widths) {
      const page = await browser.newPage({ viewport: { width, height: width === 375 ? 812 : 860 }, colorScheme: scheme });
      await page.route("**/api/**", (r) => {
        const path = new URL(r.request().url()).pathname;
        const body = routes[path] ?? (path.endsWith("/application") ? '{"application":null}' : null);
        return body ? r.fulfill({ contentType: "application/json", body }) : r.fulfill({ status: 404, body: "{}" });
      });
      await page.goto(`http://localhost:4173${name === "leads" && width === 375 ? "/?page=leads" : url}`);
      await page.waitForTimeout(800);
      await page.screenshot({ path: new URL(`${name}-${scheme}-${width}.png`, out).pathname });
      await page.close();
    }
  }
}
await browser.close();
console.log("done");
