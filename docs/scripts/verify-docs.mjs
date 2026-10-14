// Verifies the anchors and the links of the built site (run it after `vitepress build`).
//
// VitePress fails the build on a link to a page that does not exist, but not on a link to a #fragment that does
// not exist. Released binaries and other sites link to sections of the documentation, so this checks:
//
//   1. every "page#id" of tests/anchors-baseline.json (the ids of the site before the pages were split) still
//      resolves: the page has the id, or .vitepress/redirects.ts maps it to a place that has it;
//   2. every target of the redirect map exists;
//   3. every link between pages of the site points at an id that exists (without the help of the redirect map);
//   4. every https://jactionlint.jdx.dev/page#id and docs/page.md#id written in a file of the repository resolves.
//      Go sources may use a redirected location, because binaries that were released print it; every other file
//      must name the final location.
//
// Usage: node scripts/verify-docs.mjs [--dist .vitepress/dist] [--baseline tests/anchors-baseline.json]
//        node scripts/verify-docs.mjs --write-baseline    record the ids of the current build as the baseline
import { execSync } from "node:child_process";
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const docsDir = resolve(here, "..");
const args = process.argv.slice(2);
const opt = (name, fallback) => (args.includes(name) ? args[args.indexOf(name) + 1] : fallback);
const dist = resolve(docsDir, opt("--dist", ".vitepress/dist"));
const baselineFile = resolve(docsDir, opt("--baseline", "tests/anchors-baseline.json"));
const { redirects } = await import(join(docsDir, ".vitepress/redirects.ts"));

// ids and fragments of the theme (navigation, search, skip link), not content
const THEME_ID = /^(?:app|VP|__|skip|check-dark-mode|check-mac-os|local-search|.*-aria-label)/;
const dec = (s) => {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
};
// A playground permalink is "#e" and the base64 of a zlib stream (see .vitepress/config.mts): state, not an anchor.
const isPermalink = (f) => /^e[A-Za-z0-9+/%=]{15,}$/.test(f);
const pagePath = (p) =>
  ("/" + p.replace(/\\/g, "/").replace(/index\.html$/, "").replace(/\.html$/, "").replace(/\.md$/, "").replace(/^\/+/, ""))
    .replace(/(.)\/$/, "$1");

function htmlFiles(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) {
      if (name !== "assets") htmlFiles(p, out);
    } else if (name.endsWith(".html")) out.push(p);
  }
  return out;
}

// What the rendered content of each page defines (ids) and links to (links).
function model(html, page) {
  const start = html.indexOf('class="vp-doc');
  const body = start >= 0 ? html.slice(start) : html;
  const ids = new Set();
  for (const m of body.matchAll(/<[a-zA-Z][a-zA-Z0-9]*\b[^>]*?\sid="([^"]+)"/g)) ids.add(m[1]);
  const links = new Set();
  for (const m of body.matchAll(/<a\b[^>]*?\shref="([^"]+)"/g)) {
    let href = m[1].replace(/&amp;/g, "&");
    const site = /^https:\/\/jactionlint\.jdx\.dev(\/[^#?]*)?(#.*)?$/.exec(href);
    if (site) href = (site[1] || "/") + (site[2] || "");
    else if (/^[a-z][a-z0-9+.-]*:/i.test(href)) continue;
    if (href.startsWith("#")) href = page + href;
    else if (!href.startsWith("/")) continue;
    const [path, frag = ""] = href.replace(/\?[^#]*/, "").split("#");
    if (frag && (isPermalink(frag) || THEME_ID.test(frag))) continue; // state and theme UI, not content
    if (/\.(svg|png|ico|txt|json|wasm|js|webmanifest|css|xml)$/.test(path)) continue;
    links.add(pagePath(path) + (frag ? "#" + dec(frag) : ""));
  }
  return { ids: [...ids].filter((i) => !THEME_ID.test(i)).sort(), links: [...links] };
}

const pages = {};
for (const file of htmlFiles(dist)) {
  const page = pagePath(relative(dist, file));
  if (page !== "/404") pages[page] = model(readFileSync(file, "utf8"), page);
}

if (args.includes("--write-baseline")) {
  const ids = Object.fromEntries(Object.entries(pages).map(([p, m]) => [p, m.ids]));
  writeFileSync(baselineFile, JSON.stringify(ids, null, 1) + "\n");
  console.log(`wrote ${Object.values(ids).reduce((n, i) => n + i.length, 0)} ids of ${Object.keys(ids).length} pages to ${baselineFile}`);
  process.exit(0);
}

const problems = [];
const has = (page, frag) => Boolean(pages[page]) && (!frag || pages[page].ids.includes(frag));
const resolves = (loc) => {
  const [page, frag = ""] = loc.split("#");
  return has(page, frag);
};

// 1 and 2: the old ids and the redirect map
const baseline = JSON.parse(readFileSync(baselineFile, "utf8"));
for (const [page, ids] of Object.entries(baseline)) {
  for (const id of ids) {
    if (has(page, id)) continue;
    const to = redirects[`${page}#${id}`];
    if (!to) problems.push(`LOST ANCHOR   ${page}#${id}  (no such id, and no entry in .vitepress/redirects.ts)`);
  }
}
for (const [from, to] of Object.entries(redirects)) {
  if (!resolves(to)) problems.push(`BAD REDIRECT  ${from} -> ${to}  (the target does not exist)`);
  const [page, frag] = from.split("#");
  if (!(baseline[page] ?? []).includes(frag)) problems.push(`STALE REDIRECT ${from}  (not an id of the baseline: remove it or fix the key)`);
}

// 3: links between pages
for (const [page, m] of Object.entries(pages)) {
  for (const link of m.links) {
    const [target, frag = ""] = link.split("#");
    if (!pages[target]) problems.push(`DEAD LINK     ${page} -> ${link}  (no such page)`);
    else if (frag && !pages[target].ids.includes(frag)) problems.push(`DEAD FRAGMENT ${page} -> ${link}`);
  }
}

// 4: references from the files of the repository
const root = resolve(docsDir, "..");
const tracked = execSync("git ls-files -z", { cwd: root, encoding: "utf8", maxBuffer: 1 << 28 })
  .split("\0")
  .filter((f) => f && !/^(testdata\/|CHANGELOG\.md$|docs\/package-lock\.json$|docs\/tests\/)|\.(png|ico|svg|wasm|gif|zip)$/.test(f));
let checked = 0;
const seen = new Set();
for (const file of tracked) {
  let text;
  try {
    text = readFileSync(join(root, file), "utf8");
  } catch {
    continue;
  }
  const lenient = file.endsWith(".go");
  const refs = [];
  for (const m of text.matchAll(/jactionlint\.jdx\.dev(\/[A-Za-z0-9_./-]*)?#([A-Za-z0-9_%.-]+)/g)) refs.push([pagePath(m[1] || "/"), dec(m[2]), m[0]]);
  if (!file.startsWith("docs/")) for (const m of text.matchAll(/docs\/([a-z0-9-]+)\.md#([A-Za-z0-9_.-]+)/g)) refs.push(["/" + m[1], m[2], m[0]]);
  for (const [page, frag, raw] of refs) {
    if (page === "/" || isPermalink(frag)) continue;
    const key = `${file} ${page}#${frag}`;
    if (seen.has(key)) continue;
    seen.add(key);
    checked++;
    if (has(page, frag)) continue;
    if (lenient && redirects[`${page}#${frag}`]) continue;
    problems.push(`DEAD REFERENCE ${file}: ${raw}  (${lenient ? "no such id and no redirect" : "no such id; name the final location"})`);
  }
}

for (const p of problems) console.log(p);
console.log(
  problems.length
    ? `\nFAIL: ${problems.length} problem(s) in ${Object.keys(pages).length} pages (${checked} references from repository files checked)`
    : `OK: ${Object.keys(pages).length} pages, ${Object.values(baseline).flat().length} baseline ids, ${Object.keys(redirects).length} redirects, ${checked} references from repository files`,
);
process.exit(problems.length ? 1 : 0);
