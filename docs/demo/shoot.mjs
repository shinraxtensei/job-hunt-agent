// README screenshots from demo data only (M8-03 AC2). The built tracker page is served by
// `vite preview`; every /api request is answered from demo.mjs; any other request that
// isn't the page itself is aborted, so nothing real can appear.
// Run: make web-build; (cd web && npx vite preview --port 4173 &); node docs/demo/shoot.mjs
import { createRequire } from "node:module";
import { mkdirSync } from "node:fs";
import { now, routes } from "./demo.mjs";

const { chromium } = createRequire(new URL("../../web/package.json", import.meta.url))("@playwright/test");
const BASE = "http://localhost:4173";
const out = new URL("../screens/", import.meta.url);
mkdirSync(out, { recursive: true });

const shots = [
  ["today", "/?page=today", 1280, "dark"],
  ["leads", "/?page=leads&lead=100", 1280, "dark"],
  ["leads-light", "/?page=leads&lead=100", 1280, "light"],
  ["cv", "/?page=leads&lead=105&tab=cv", 1280, "dark"],
  ["board", "/?page=board", 1280, "light"],
  ["followups", "/?page=followups", 1280, "dark"],
  ["phone", "/?page=leads&lead=102", 375, "dark"],
];

const browser = await chromium.launch();
for (const [name, path, width, scheme] of shots) {
  const ctx = await browser.newContext({ viewport: { width, height: width === 375 ? 812 : 820 }, colorScheme: scheme });
  await ctx.addInitScript(`{ const N = ${now}; const D = Date; globalThis.Date = class extends D { constructor(...a) { super(...(a.length ? a : [N])); } static now() { return N; } }; }`);
  const page = await ctx.newPage();
  await page.route("**/*", (r) => {
    const url = new URL(r.request().url());
    if (url.origin !== BASE) return r.abort();
    if (!url.pathname.startsWith("/api/")) return r.continue();
    const body = routes[url.pathname];
    return body ? r.fulfill({ contentType: "application/json", body: JSON.stringify(body) }) : r.fulfill({ status: 404, contentType: "application/json", body: "{}" });
  });
  await page.goto(BASE + path);
  await page.waitForTimeout(900);
  await page.screenshot({ path: new URL(`${name}.png`, out).pathname });
  await ctx.close();
}
await browser.close();
console.log("done");
