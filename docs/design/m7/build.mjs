// M7-00 gate: builds two direction mockups (Leads split view + Today) from a real-data
// snapshot in docs/design/m7/private/ (git-ignored: job postings and the owner's fit
// reasons), then screenshots them at 1280 and 375 px, light and dark, side by side.
// Posting text is untrusted: every string goes through esc().
// Run: node docs/design/m7/build.mjs   (needs web/'s Playwright and the private snapshot)
import { createRequire } from "node:module";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";

const { chromium } = createRequire(new URL("../../../web/package.json", import.meta.url))("@playwright/test");
const priv = new URL("private/", import.meta.url);
const read = (f) => JSON.parse(readFileSync(new URL(f, priv), "utf8"));
const leads = read("leads.json").leads;
const total = read("leads.json").total;
const lead = read("lead55.json");
const recent = read("recent.json").leads;
const closed = read("closed.json").leads;
const NOW = new Date("2026-11-04T09:00:00Z"); // the morning after the last collect
const LAST_VISIT = new Date("2026-11-03T08:00:00Z");

const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
const href = (u) => (/^https?:\/\//i.test(u) ? esc(u) : "#");
const ago = (iso) => {
  const h = Math.round((NOW - new Date(iso)) / 3600e3);
  return h < 1 ? "just now" : h < 24 ? `${h} h ago` : `${Math.round(h / 24)} d ago`;
};
const place = (l) => {
  const [first, ...rest] = l.locations;
  return esc(first || "No location") + (rest.length ? ` +${rest.length}` : "");
};
const FIT = { very_strong: ["Very strong", "mint"], strong: ["Strong", "mint"], good: ["Good", "sand"], stretch: ["Stretch", "salmon"] };
const fit = (l) => FIT[l.fit_level] ?? ["Unscored", "lilac"];
const fresh = recent.filter((l) => new Date(l.first_seen_at) > LAST_VISIT && l.fit_level).sort((a, b) => (b.score ?? 0) - (a.score ?? 0));
const followups = [
  // illustrative: none are due in the snapshot (no lead is applied yet)
  { company: leads[2].company.name, title: leads[2].title, days: 8, contact: "Recruiter" },
  { company: leads[7].company.name, title: leads[7].title, days: 11, contact: "" },
];
const tiles = [
  ["New since your last visit", fresh.length, "since yesterday 08:00", "mint"],
  ["Follow-ups due", followups.length, "7+ days after applying", "sand"],
  ["Applied this week", 3, "2 more than last week", "blue"],
  ["Drafts ready", 1, "waiting for your review", "lilac"],
];
const fontFace = (name, file) =>
  `@font-face{font-family:"${name}";src:url("private/fonts/${file}") format("woff2");font-weight:100 900;font-display:swap}`;

// ------------------------------------------------------------------ shared pieces
const pills = (active) =>
  ["Today", "Leads", "Board", "Follow-ups", "Drafts"]
    .map((p) => `<a class="pill${p === active ? " on" : ""}" href="#"${p === active ? ' aria-current="page"' : ""}>${p}</a>`)
    .join("");
const breakdown = (lead.breakdown ?? [])
  .map((f) => {
    const pct = Math.round((f.points / f.weight) * 100);
    const name = { required: "Required skills", title: "Title", seniority: "Seniority", location: "Location", optional: "Optional skills", freshness: "Freshness" }[f.factor] ?? f.factor;
    return `<div class="bar"><span>${esc(name)}</span><i><b style="width:${pct}%"></b></i><em>${f.points.toFixed(1)} / ${f.weight}</em></div>`;
  })
  .join("");
const gaps = (lead.gaps ?? []).map((g) => `<span class="tag salmon">${esc(g)}</span>`).join("");
const tabs = ["Overview", "CV &amp; letter", "Notes", "Contacts", "History"].map((t, i) => `<button class="tab${i ? "" : " on"}">${t}</button>`).join("");
const detail = (cls) => `
<section class="detail ${cls}" aria-label="Lead details">
  <div class="dhead">
    <button class="back" aria-label="Back to leads">←</button>
    <div>
      <p class="dcompany">${esc(lead.company.name)}</p>
      <h2><a href="${href(lead.url)}" target="_blank" rel="noopener noreferrer">${esc(lead.title)}</a></h2>
      <p class="dmeta">${place(lead)} · ${esc(lead.workplaces.join(", "))} · first seen ${ago(lead.first_seen_at)}</p>
    </div>
  </div>
  <div class="tabs" role="tablist">${tabs}</div>
  <div class="dgrid">
    <div class="tile ${fit(lead)[1]} small"><p>Fit</p><strong>${lead.score.toFixed(1)}</strong><span>${fit(lead)[0]} · CV ${esc(lead.cv_variant ?? "")}</span></div>
    <div class="tile lilac small"><p>Status</p><strong class="word">New</strong><span>Set status ▾ · Skip</span></div>
  </div>
  <p class="reason">${esc(lead.reason ?? "")}</p>
  <div class="gaps"><span class="lbl">Gaps</span>${gaps}</div>
  <div class="bars">${breakdown}</div>
  <div class="actions"><button class="btn primary">CV &amp; letter draft</button><button class="btn">Add note</button></div>
</section>`;
const row = (l, i, cls) => {
  const [word, colour] = fit(l);
  return `<li class="${cls}${i === 0 ? " sel" : ""}" tabindex="0"${i === 0 ? ' aria-selected="true"' : ""}>
  <div class="who"><p class="co">${esc(l.company.name)}</p>
  <p class="ti"><a href="${href(l.url)}" target="_blank" rel="noopener noreferrer">${esc(l.title)}</a></p>
  <p class="meta">${place(l)}</p></div>
  <div class="side"><span class="tag ${colour}">${word}${l.score != null ? ` · ${Math.round(l.score)}` : ""}</span><p class="when">${ago(l.first_seen_at)}</p></div>
</li>`;
};
const todayBody = (cardCls) => `
<h1>Today</h1>
<p class="sub">Since your last visit, yesterday at 08:00 · <a href="#">Mark all seen</a></p>
<div class="tiles">${tiles.map(([t, n, s, c]) => `<div class="tile ${c}"><p>${t}</p><strong>${n}</strong><span>${s}</span></div>`).join("")}</div>
<div class="cols">
  <section><h2>New leads <span class="count">${fresh.length}</span></h2>
    <ul class="list">${fresh.slice(0, 6).map((l, i) => row(l, i + 1, cardCls)).join("")}</ul></section>
  <div>
    <section><h2>Follow-ups due <span class="count">${followups.length}</span></h2>
      <ul class="list">${followups
        .map(
          (f) => `<li class="${cardCls}"><div class="who"><p class="co">${esc(f.company)}</p><p class="ti">${esc(f.title)}</p>
          <p class="meta">${f.contact ? `${esc(f.contact)} · ` : ""}<a href="#">Copy message</a> · <a href="#">Snooze</a></p></div>
          <div class="side"><span class="tag sand">Due</span><p class="when">applied ${f.days} d ago</p></div></li>`,
        )
        .join("")}</ul></section>
    <section><h2>Closed since your last visit <span class="count">${closed.slice(0, 3).length}</span></h2>
      <ul class="list">${closed
        .slice(0, 3)
        .map(
          (l) => `<li class="${cardCls}"><div class="who"><p class="co">${esc(l.company.name)}</p><p class="ti">${esc(l.title)}</p></div>
          <div class="side"><span class="tag salmon">Closed</span><p class="when">${ago(l.first_seen_at)}</p></div></li>`,
        )
        .join("")}</ul></section>
  </div>
</div>`;
const leadsBody = (cardCls) => `
<div class="split">
  <section class="listpane" aria-label="Leads">
    <div class="lhead"><h1>Leads</h1><span class="count">${total} shown · best fit first</span></div>
    <div class="chips"><span class="chip">Backend +6</span><span class="chip">≤ 5 years</span><span class="chip">My locations</span><span class="chip ghost">Filters</span></div>
    <ul class="list">${leads.slice(0, 12).map((l, i) => row(l, i, cardCls)).join("")}</ul>
  </section>
  ${detail("")}
</div>`;
const page = (title, css, body, extra = "") => `<!doctype html><html lang="en" data-theme="dark"${extra}><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1"><title>${esc(title)}</title><style>${css}</style></head><body>${body}</body></html>`;

// ------------------------------------------------------------------ A: Soft cards
const cssA = `${fontFace("Geist", "geist-latin-wght-normal.woff2")}
:root{--bg:#f3f4f2;--surface:#ffffff;--surface2:#ecedea;--text:#121412;--muted:#5d625c;--line:#e2e4df;
--mint:#9fdcb4;--sand:#ead48f;--salmon:#f1a59c;--blue:#a8d3dd;--lilac:#cdb3f2;--on:#0f1411;--r:22px}
[data-theme=dark]{--bg:#0b0c0d;--surface:#17181a;--surface2:#202124;--text:#eef0ec;--muted:#9da39c;--line:#26282b}
*{box-sizing:border-box}html{color-scheme:light dark}
body{margin:0;background:var(--bg);color:var(--text);font:15px/1.45 Geist,system-ui,sans-serif;font-variant-numeric:tabular-nums}
a{color:inherit}
header{display:flex;align-items:center;gap:12px;padding:20px 32px;flex-wrap:wrap}
.pills{display:flex;gap:8px;background:var(--surface);padding:6px;border-radius:999px;overflow-x:auto;max-width:100%}
.pill{padding:10px 18px;border-radius:999px;text-decoration:none;font-weight:600;color:var(--muted);white-space:nowrap}
.pill.on{background:var(--text);color:var(--bg)}
.tools{margin-left:auto;display:flex;gap:8px}.tools span{background:var(--surface);border-radius:999px;padding:10px 16px;color:var(--muted)}
main{padding:8px 32px 40px}
h1{font-size:40px;letter-spacing:-.02em;margin:8px 0 4px;font-weight:700}
h2{font-size:20px;margin:28px 0 12px;font-weight:650}
.sub,.count{color:var(--muted)}.count{font-weight:500;font-size:15px;margin-left:6px}
.tiles{display:grid;grid-template-columns:repeat(4,1fr);gap:16px;margin-top:20px}
.tile{border-radius:var(--r);padding:20px 22px;color:var(--on)}
.tile p{margin:0;font-weight:650}.tile strong{display:block;font-size:56px;line-height:1.1;font-weight:750;letter-spacing:-.03em}
.tile span{font-size:13px;opacity:.8}.tile.small strong{font-size:40px}.tile .word{font-size:32px}
.mint{background:var(--mint)}.sand{background:var(--sand)}.salmon{background:var(--salmon)}.blue{background:var(--blue)}.lilac{background:var(--lilac)}
.cols{display:grid;grid-template-columns:1.3fr 1fr;gap:24px}
.list{list-style:none;margin:0;padding:0;display:grid;gap:10px}
.card{display:flex;gap:16px;justify-content:space-between;background:var(--surface);border-radius:var(--r);padding:16px 20px;cursor:pointer}
.card.sel{outline:2px solid var(--mint)}
.co{margin:0;font-weight:700;font-size:16px}.ti{margin:2px 0 0}.ti a{text-decoration:none}.ti a:hover{text-decoration:underline}
.meta,.when{margin:4px 0 0;color:var(--muted);font-size:13px}
.side{text-align:right;flex:none}
.tag{display:inline-block;border-radius:999px;padding:4px 12px;font-weight:650;font-size:13px;color:var(--on)}
.split{display:grid;grid-template-columns:minmax(360px,1fr) 1.15fr;gap:24px;align-items:start}
.lhead{display:flex;align-items:baseline;gap:8px}
.chips{display:flex;gap:8px;flex-wrap:wrap;margin:10px 0 16px}.chip{background:var(--surface);border-radius:999px;padding:6px 14px;font-size:13px;color:var(--muted)}
.detail{background:var(--surface);border-radius:28px;padding:28px;position:sticky;top:16px}
.dhead{display:flex;gap:12px}.back{display:none}
.dcompany{margin:0;color:var(--muted);font-weight:650}.detail h2{margin:4px 0;font-size:28px;letter-spacing:-.01em}.detail h2 a{text-decoration:none}
.dmeta{margin:0;color:var(--muted)}
.tabs{display:flex;gap:6px;margin:20px 0;background:var(--surface2);padding:5px;border-radius:999px;overflow-x:auto}
.tab{border:0;background:none;color:var(--muted);font:inherit;font-weight:600;padding:8px 14px;border-radius:999px;white-space:nowrap}.tab.on{background:var(--surface);color:var(--text)}
.dgrid{display:grid;grid-template-columns:1fr 1fr;gap:12px}
.reason{font-size:16px;margin:18px 0 12px}
.gaps{display:flex;gap:6px;flex-wrap:wrap;align-items:center}.lbl{color:var(--muted);margin-right:4px}
.bars{margin-top:16px;display:grid;gap:8px}.bar{display:grid;grid-template-columns:130px 1fr 72px;gap:10px;align-items:center;font-size:13px}
.bar i{height:8px;border-radius:99px;background:var(--surface2);display:block}.bar b{display:block;height:100%;border-radius:99px;background:var(--mint)}.bar em{font-style:normal;color:var(--muted);text-align:right}
.actions{display:flex;gap:10px;margin-top:20px}.btn{border:0;border-radius:999px;padding:12px 18px;font:inherit;font-weight:650;background:var(--surface2);color:var(--text)}.btn.primary{background:var(--mint);color:var(--on)}
@media (max-width:767px){header{padding:14px 16px}.tools{display:none}main{padding:4px 16px 32px}h1{font-size:32px}
.tiles{grid-template-columns:1fr 1fr;gap:10px}.tile{padding:14px 16px}.tile strong{font-size:40px}
.cols,.split{grid-template-columns:1fr}.detail{display:none}.sheet .detail{display:block;position:fixed;inset:0;border-radius:0;overflow:auto;z-index:2}
.sheet .back{display:block;border:0;background:var(--surface2);color:var(--text);border-radius:999px;width:40px;height:40px;font-size:18px;flex:none}
.card{padding:14px 16px}.bar{grid-template-columns:110px 1fr 64px}}`;

// ------------------------------------------------------------------ B: Quiet ledger
const cssB = `${fontFace("Jakarta", "plus-jakarta-sans-latin-wght-normal.woff2")}
:root{--bg:#f6f6f4;--surface:#ffffff;--surface2:#efefec;--text:#16171a;--muted:#62656c;--line:#e6e6e2;
--mint:#2f8f5b;--sand:#9a7414;--salmon:#c0473c;--blue:#2c7a8c;--lilac:#7550c4;
--mint-bg:#dff3e6;--sand-bg:#f6ecc9;--salmon-bg:#fbe1dd;--blue-bg:#dcedf1;--lilac-bg:#ebe2fb;--r:20px}
[data-theme=dark]{--bg:#0f1012;--surface:#18191c;--surface2:#212226;--text:#ecedee;--muted:#a0a3aa;--line:#26272b;
--mint:#9fdcb4;--sand:#ead48f;--salmon:#f1a59c;--blue:#a8d3dd;--lilac:#cdb3f2;
--mint-bg:#1d2e24;--sand-bg:#2f2a18;--salmon-bg:#33201d;--blue-bg:#1b2a2e;--lilac-bg:#261f35}
*{box-sizing:border-box}html{color-scheme:light dark}
body{margin:0;background:var(--bg);color:var(--text);font:14px/1.5 Jakarta,system-ui,sans-serif;font-variant-numeric:tabular-nums}
a{color:inherit}
header{display:flex;align-items:center;gap:16px;padding:16px 28px;border-bottom:1px solid var(--line);flex-wrap:wrap}
.pills{display:flex;gap:4px;overflow-x:auto;max-width:100%}
.pill{padding:8px 16px;border-radius:999px;text-decoration:none;font-weight:600;color:var(--muted);white-space:nowrap;border:1px solid transparent}
.pill.on{border-color:var(--line);background:var(--surface);color:var(--text)}
.tools{margin-left:auto;display:flex;gap:8px}.tools span{border:1px solid var(--line);border-radius:999px;padding:7px 14px;color:var(--muted)}.tools span:first-child{min-width:220px}
main{padding:20px 28px 40px}
h1{font-size:30px;letter-spacing:-.02em;margin:4px 0;font-weight:750}
h2{font-size:15px;text-transform:uppercase;letter-spacing:.06em;color:var(--muted);margin:28px 0 10px;font-weight:700}
.sub,.count{color:var(--muted)}.count{font-weight:500;margin-left:6px;text-transform:none;letter-spacing:0}
.tiles{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-top:18px}
.tile{border-radius:var(--r);padding:18px 20px;background:var(--surface);border:1px solid var(--line)}
.tile p{margin:0;color:var(--muted);font-weight:600}.tile strong{display:block;font-size:44px;line-height:1.15;font-weight:750;letter-spacing:-.02em}
.tile span{font-size:12px;color:var(--muted)}.tile.small strong{font-size:32px}.tile .word{font-size:26px}
.tile.mint strong{color:var(--mint)}.tile.sand strong{color:var(--sand)}.tile.salmon strong{color:var(--salmon)}.tile.blue strong{color:var(--blue)}.tile.lilac strong{color:var(--lilac)}
.tile::before{content:"";display:block;width:28px;height:6px;border-radius:99px;margin-bottom:12px}
.tile.mint::before{background:var(--mint)}.tile.sand::before{background:var(--sand)}.tile.blue::before{background:var(--blue)}.tile.lilac::before{background:var(--lilac)}.tile.salmon::before{background:var(--salmon)}
.cols{display:grid;grid-template-columns:1.3fr 1fr;gap:24px}
.list{list-style:none;margin:0;padding:0;background:var(--surface);border:1px solid var(--line);border-radius:var(--r);overflow:hidden}
.row{display:flex;gap:14px;justify-content:space-between;padding:12px 18px;border-top:1px solid var(--line);cursor:pointer}
.row:first-child{border-top:0}.row.sel{background:var(--surface2);box-shadow:inset 3px 0 var(--mint)}
.co{margin:0;font-weight:700}.ti{margin:1px 0 0}.ti a{text-decoration:none}.ti a:hover{text-decoration:underline}
.meta,.when{margin:2px 0 0;color:var(--muted);font-size:12.5px}
.side{text-align:right;flex:none}
.tag{display:inline-block;border-radius:999px;padding:3px 10px;font-weight:650;font-size:12px}
.tag.mint{background:var(--mint-bg);color:var(--mint)}.tag.sand{background:var(--sand-bg);color:var(--sand)}.tag.salmon{background:var(--salmon-bg);color:var(--salmon)}.tag.blue{background:var(--blue-bg);color:var(--blue)}.tag.lilac{background:var(--lilac-bg);color:var(--lilac)}
.split{display:grid;grid-template-columns:minmax(360px,1fr) 1.2fr;gap:20px;align-items:start}
.lhead{display:flex;align-items:baseline;gap:8px}
.chips{display:flex;gap:6px;flex-wrap:wrap;margin:8px 0 14px}.chip{border:1px solid var(--line);border-radius:999px;padding:4px 12px;font-size:12.5px;color:var(--muted)}
.detail{background:var(--surface);border:1px solid var(--line);border-radius:24px;padding:24px;position:sticky;top:16px}
.dhead{display:flex;gap:12px}.back{display:none}
.dcompany{margin:0;color:var(--muted);font-weight:650}.detail h2{margin:2px 0;font-size:24px;text-transform:none;letter-spacing:-.01em;color:var(--text)}.detail h2 a{text-decoration:none}
.dmeta{margin:0;color:var(--muted)}
.tabs{display:flex;gap:2px;margin:18px 0;border-bottom:1px solid var(--line);overflow-x:auto}
.tab{border:0;background:none;color:var(--muted);font:inherit;font-weight:600;padding:8px 12px;border-bottom:2px solid transparent;white-space:nowrap}.tab.on{color:var(--text);border-bottom-color:var(--text)}
.dgrid{display:grid;grid-template-columns:1fr 1fr;gap:10px}
.reason{font-size:15px;margin:16px 0 10px}
.gaps{display:flex;gap:6px;flex-wrap:wrap;align-items:center}.lbl{color:var(--muted);margin-right:4px}
.gaps .tag{background:var(--salmon-bg);color:var(--salmon)}
.bars{margin-top:14px;display:grid;gap:7px}.bar{display:grid;grid-template-columns:130px 1fr 72px;gap:10px;align-items:center;font-size:12.5px}
.bar i{height:6px;border-radius:99px;background:var(--surface2);display:block}.bar b{display:block;height:100%;border-radius:99px;background:var(--mint)}.bar em{font-style:normal;color:var(--muted);text-align:right}
.actions{display:flex;gap:8px;margin-top:18px}.btn{border:1px solid var(--line);border-radius:999px;padding:9px 16px;font:inherit;font-weight:650;background:var(--surface);color:var(--text)}.btn.primary{background:var(--text);color:var(--bg);border-color:var(--text)}
@media (max-width:767px){header{padding:12px 16px}.tools{display:none}main{padding:12px 16px 32px}h1{font-size:26px}
.tiles{grid-template-columns:1fr 1fr;gap:8px}.tile{padding:12px 14px}.tile strong{font-size:34px}
.cols,.split{grid-template-columns:1fr}.detail{display:none}.sheet .detail{display:block;position:fixed;inset:0;border-radius:0;border:0;overflow:auto;z-index:2}
.sheet .back{display:block;border:1px solid var(--line);background:var(--surface);color:var(--text);border-radius:999px;width:38px;height:38px;flex:none}
.bar{grid-template-columns:110px 1fr 64px}}`;

const shell = (active, body) => `<header><nav class="pills" aria-label="Pages">${pills(active)}</nav><div class="tools"><span>Search  /</span><span>Filters</span><span>☾</span></div></header><main>${body}</main>`;
const out = new URL("private/out/", import.meta.url);
mkdirSync(out, { recursive: true });
const directions = [
  ["a", "A · Soft cards", cssA, "card"],
  ["b", "B · Quiet ledger", cssB, "row"],
];
for (const [id, , css, cardCls] of directions) {
  writeFileSync(new URL(`${id}-leads.html`, out), page("Leads", css.replaceAll("private/fonts/", "../fonts/"), shell("Leads", leadsBody(cardCls))));
  writeFileSync(new URL(`${id}-today.html`, out), page("Today", css.replaceAll("private/fonts/", "../fonts/"), shell("Today", todayBody(cardCls))));
}

// ------------------------------------------------------------------ screenshots
const browser = await chromium.launch();
const shots = {};
for (const [id] of directions) {
  for (const screen of ["leads", "today", "sheet"]) {
    for (const theme of ["light", "dark"]) {
      for (const width of screen === "sheet" ? [375] : [1280, 375]) {
        const p = await browser.newPage({ viewport: { width, height: width === 375 ? 812 : 860 } });
        await p.goto(new URL(`${id}-${screen === "sheet" ? "leads" : screen}.html`, out).href);
        await p.evaluate(([t, sheet]) => {
          document.documentElement.setAttribute("data-theme", t);
          if (sheet) document.body.classList.add("sheet");
        }, [theme, screen === "sheet"]);
        await p.evaluate(() => document.fonts.ready);
        const overflow = await p.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
        const file = new URL(`${id}-${screen}-${theme}-${width}.png`, out);
        await p.screenshot({ path: file.pathname });
        (shots[`${id}-${screen}`] ??= []).push({ file, theme, width, overflow });
        await p.close();
      }
    }
  }
}
const img = (s) => `<figure><img src="data:image/png;base64,${readFileSync(s.file).toString("base64")}"><figcaption>${s.theme} · ${s.width}px${s.overflow > 0 ? ` · OVERFLOW ${s.overflow}` : ""}</figcaption></figure>`;
for (const [id, label] of directions) {
  const sheet = await browser.newPage({ viewport: { width: 2700, height: 1000 } });
  const L = shots[`${id}-leads`], T = shots[`${id}-today`], S = shots[`${id}-sheet`];
  const by = (arr, theme, w) => arr.find((s) => s.theme === theme && s.width === w);
  await sheet.setContent(`<style>body{margin:0;padding:24px;background:#777;font:20px system-ui;color:#fff}h1{margin:0 0 12px;font-size:30px}
  h2{margin:18px 0 10px;font-size:22px}.r{display:flex;gap:20px;align-items:flex-start}figure{margin:0}img{display:block;border:1px solid #444}figcaption{margin-top:6px}</style>
  <h1>${label}</h1><h2>Leads (split view)</h2><div class="r">${img(by(L, "light", 1280))}${img(by(L, "dark", 1280))}</div>
  <h2>Today</h2><div class="r">${img(by(T, "light", 1280))}${img(by(T, "dark", 1280))}</div>
  <h2>Phone: Leads · detail sheet · Today</h2><div class="r">${img(by(L, "light", 375))}${img(by(L, "dark", 375))}${img(by(S, "light", 375))}${img(by(S, "dark", 375))}${img(by(T, "light", 375))}${img(by(T, "dark", 375))}</div>`);
  await sheet.screenshot({ path: new URL(`${id}-sheet.png`, out).pathname, fullPage: true });
  await sheet.close();
  console.log(label, Object.entries(shots).filter(([k]) => k.startsWith(id)).map(([k, v]) => `${k}: ${v.map((s) => s.overflow).join("/")}`).join(" · "));
}
await browser.close();
