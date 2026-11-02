// Screenshots the built tracker page served by a running API (default http://127.0.0.1:8089)
// in light and dark at 1280 and 375 px. Uses web/'s Playwright:
//   node docs/design/shoot-app.mjs [out-dir]
import { createRequire } from "node:module";

const { chromium } = createRequire(new URL("../../web/package.json", import.meta.url))("@playwright/test");
const base = process.env.APP_URL ?? "http://127.0.0.1:8089";
const out = process.argv[2] ?? new URL("screens/", import.meta.url).pathname;
const browser = await chromium.launch();
for (const theme of ["light", "dark"]) {
  for (const width of [1280, 375]) {
    const page = await browser.newPage({ viewport: { width, height: width === 375 ? 812 : 800 }, colorScheme: theme });
    const errors = [];
    page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
    await page.goto(base + "/");
    await page.getByRole("status").filter({ hasText: "shown" }).waitFor();
    await page.keyboard.press("j"); // show the focused-row state
    const summary = await page.getByRole("status").filter({ hasText: "shown" }).textContent();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    await page.screenshot({ path: `${out}/${theme}-${width}.png` });
    console.log(theme, width, JSON.stringify(summary), "overflow", overflow, "console errors", errors.length);
    await page.close();
  }
}
await browser.close();
