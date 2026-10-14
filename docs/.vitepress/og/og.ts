// Generates og.png, the social card of the site (og:image and twitter:image in config.mts). Called from the VitePress
// `buildEnd` hook, so the card follows each release without anyone redrawing it: the version comes from version.ts
// (release-please bumps it) and the bars count the rules of docs/rules.md by group at this release and at the two
// minor releases before it, read from their git tags.
//
// The card is an SVG rendered with resvg and the fonts next to this file, so the build needs no browser. Without git
// or tags (a shallow clone, a tarball) the card shows only the current rules.
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { Resvg, type ResvgRenderOptions } from "@resvg/resvg-js";

import { version } from "../version.ts";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "../../..");
const fontsDir = join(here, "fonts");
const logoFile = resolve(here, "../../public/logo.svg");

export const width = 1200;
export const height = 630;

export interface GroupCounts {
  correctness: number;
  security: number;
  /** policy and style together */
  policy: number;
}

export interface Release {
  /** major.minor, as the card shows it */
  label: string;
  counts: GroupCounts;
}

export const total = (c: GroupCounts) => c.correctness + c.security + c.policy;

/** Counts the rules of the table of docs/rules.md by its Group column, wherever that column is. */
export function countGroups(rulesMd: string): GroupCounts | undefined {
  // A summary may contain an escaped \| , which does not end a cell.
  const cells = (row: string) => row.split(/(?<!\\)\|/).map((c) => c.trim());
  const lines = rulesMd.split("\n");
  const header = lines.find((l) => /^\|\s*ID\s*\|/.test(l));
  if (!header) return undefined;
  const col = cells(header).indexOf("Group");
  if (col < 0) return undefined;
  const counts: GroupCounts = { correctness: 0, security: 0, policy: 0 };
  for (const line of lines) {
    if (!line.startsWith("| [")) continue;
    const group = cells(line)[col];
    if (group === "correctness" || group === "security") counts[group]++;
    else if (group === "policy" || group === "style") counts.policy++;
  }
  return total(counts) > 0 ? counts : undefined;
}

const parse = (v: string) => {
  const m = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(v);
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : undefined;
};
export const label = (v: string) => parse(v)?.slice(0, 2).join(".") ?? v;

/**
 * The tags of the last patch release of each of the `n` minor releases before `current`, oldest first. Pre-releases
 * and tags that do not look like vX.Y.Z are ignored.
 */
export function previousMinors(tags: string[], current: string, n = 2): string[] {
  const cur = parse(current);
  if (!cur) return [];
  const latest = new Map<string, { tag: string; patch: number }>();
  for (const tag of tags) {
    const v = parse(tag);
    if (!v || v[0] > cur[0] || (v[0] === cur[0] && v[1] >= cur[1])) continue;
    const key = `${v[0]}.${v[1]}`;
    if ((latest.get(key)?.patch ?? -1) < v[2]) latest.set(key, { tag, patch: v[2] });
  }
  return [...latest.entries()]
    .sort(([a], [b]) => {
      const [am, an] = a.split(".").map(Number);
      const [bm, bn] = b.split(".").map(Number);
      return am - bm || an - bn;
    })
    .slice(-n)
    .map(([, { tag }]) => tag);
}

function git(...args: string[]): string | undefined {
  try {
    return execFileSync("git", ["-C", repoRoot, ...args], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
      maxBuffer: 64 * 1024 * 1024,
    });
  } catch {
    return undefined;
  }
}

/**
 * The releases the card compares, oldest first, ending with the current version. The current version is read at its
 * tag when it exists, so rules merged after the release do not show up; before the tag exists (the release pull
 * request, the release commit) the working tree is the release.
 */
export function collectReleases(): Release[] {
  const tags = git("tag", "--list", "v*")?.split("\n").filter(Boolean) ?? [];
  const releases: Release[] = [];
  for (const tag of previousMinors(tags, version)) {
    const counts = countGroups(git("show", `${tag}:docs/rules.md`) ?? "");
    if (counts) releases.push({ label: label(tag), counts });
  }
  const currentTag = `v${version}`;
  const current =
    (tags.includes(currentTag) && countGroups(git("show", `${currentTag}:docs/rules.md`) ?? "")) ||
    countGroups(readFileSync(join(repoRoot, "docs/rules.md"), "utf8"));
  if (!current) throw new Error("og: no rule table in docs/rules.md");
  releases.push({ label: label(version), counts: current });
  return releases;
}

const fontOptions: ResvgRenderOptions["font"] = {
  fontFiles: ["SpaceGrotesk-Medium.ttf", "SpaceGrotesk-Bold.ttf", "JetBrainsMono-Medium.ttf"].map((f) =>
    join(fontsDir, f),
  ),
  loadSystemFonts: false,
  defaultFontFamily: "Space Grotesk",
};

const SG = `font-family="Space Grotesk"`;
const JB = `font-family="JetBrains Mono"`;

/** The width in pixels of a line of text, measured with the fonts of the card. */
export function textWidth(text: string, font: string, size: number, weight = 500): number {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="2000" height="200"><text x="0" y="150" ${font} font-size="${size}" font-weight="${weight}">${text}</text></svg>`;
  return new Resvg(svg, { font: fontOptions }).getBBox()?.width ?? 0;
}

const colors = { correctness: "#60a5fa", security: "#34d399", policy: "#c084fc" } as const;

/** The SVG of the card. `measure` is textWidth, replaceable in tests. */
export function cardSvg(releases: Release[], measure = textWidth): string {
  const current = releases[releases.length - 1];
  const previous = releases[releases.length - 2];
  const added = previous ? total(current.counts) - total(previous.counts) : 0;
  const tagline =
    added > 0
      ? `${added} new rule${added === 1 ? "" : "s"} since ${previous.label}.`
      : `${total(current.counts)} rules for GitHub Actions workflows.`;

  // Layout of the 2.0 card: the logo on the left, a text column on the right, both centered above the footer.
  // The text block runs from the cap height of the title (70px above its baseline) to the last bar, and its middle
  // is level with the middle of the logo (y 290).
  const left = 376;
  const baseline = 222 + (3 - releases.length) * 20;
  const rowsTop = baseline + 100;
  const barX = left + 76;
  const maxTotal = Math.max(...releases.map((r) => total(r.counts)));
  const scale = Math.min(2.8, 470 / maxTotal);
  const out: string[] = [];

  out.push(
    `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">`,
    `<defs><linearGradient id="bg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#0b1512"/><stop offset="1" stop-color="#0e211c"/></linearGradient></defs>`,
    `<rect width="${width}" height="${height}" fill="url(#bg)"/>`,
  );

  const logo = readFileSync(logoFile, "utf8").replace(/^<svg /, `<svg x="90" y="175" width="230" height="230" `);
  out.push(logo);

  out.push(
    `<text x="${left - 4}" y="${baseline}" ${SG} font-size="96" font-weight="700" letter-spacing="-3" fill="#ecfdf5">jactionlint <tspan fill="#34d399">${current.label}</tspan></text>`,
    `<text x="${left}" y="${baseline + 62}" ${SG} font-size="40" font-weight="500" fill="#6ee7b7">${tagline}</text>`,
  );

  releases.forEach((r, i) => {
    const isCurrent = i === releases.length - 1;
    const y = rowsTop + i * 40;
    out.push(
      `<text x="${left}" y="${y + 20}" ${JB} font-size="21" font-weight="500" fill="${isCurrent ? "#6ee7b7" : "#7c8f8a"}">${r.label}</text>`,
    );
    const segments = (["correctness", "security", "policy"] as const).filter((g) => r.counts[g] > 0);
    const widths = segments.map((g) => Math.round(r.counts[g] * scale));
    const barWidth = widths.reduce((a, b) => a + b, 0) + 2 * (segments.length - 1);
    const clip = `bar${i}`;
    out.push(
      `<clipPath id="${clip}"><rect x="${barX}" y="${y}" width="${barWidth}" height="26" rx="7"/></clipPath>`,
      `<g clip-path="url(#${clip})"${isCurrent ? "" : ` opacity=".55"`}>`,
    );
    let x = barX;
    segments.forEach((g, j) => {
      out.push(`<rect x="${x}" y="${y}" width="${widths[j]}" height="26" fill="${colors[g]}"/>`);
      x += widths[j] + 2;
    });
    out.push(`</g>`);
    const n = String(total(r.counts));
    const size = isCurrent ? 36 : 30;
    const numX = barX + barWidth + 16;
    out.push(
      `<text x="${numX}" y="${y + 13 + size * 0.36}" ${SG} font-size="${size}" font-weight="700" fill="${isCurrent ? "#34d399" : "#9fb3ad"}">${n}</text>`,
    );
    if (isCurrent && added > 0) {
      const badge = `+${added}`;
      const bx = numX + measure(n, SG, size, 700) + 18;
      const bw = measure(badge, SG, 22, 700) + 20;
      out.push(
        `<rect x="${bx}" y="${y - 3}" width="${bw}" height="32" rx="8" fill="#34d399"/>`,
        `<text x="${bx + 10}" y="${y + 21}" ${SG} font-size="22" font-weight="700" fill="#0b1512">${badge}</text>`,
      );
    }
  });

  out.push(
    `<text x="90" y="580" ${JB} font-size="26" font-weight="500" fill="#7c8f8a">jactionlint.jdx.dev</text>`,
  );

  // The legend, right-aligned: a swatch, 8px, the name, 22px to the next one.
  const legend = [
    ["correctness", colors.correctness],
    ["security", colors.security],
    ["policy &amp; style", colors.policy],
  ] as const;
  const itemWidths = legend.map(([name]) => 22 + measure(name, SG, 20));
  let lx = width - 90 - itemWidths.reduce((a, b) => a + b, 0) - 22 * (legend.length - 1);
  legend.forEach(([name, color], i) => {
    out.push(
      `<rect x="${lx}" y="562" width="14" height="14" rx="4" fill="${color}"/>`,
      `<text x="${lx + 22}" y="576" ${SG} font-size="20" font-weight="500" fill="#9fb3ad">${name}</text>`,
    );
    lx += itemWidths[i] + 22;
  });

  out.push(`</svg>`);
  return out.join("\n");
}

export function renderPng(svg: string): Buffer {
  return new Resvg(svg, { font: fontOptions, fitTo: { mode: "original" } }).render().asPng();
}

/** Writes og.png into the build output. */
export function generateOgImage(outDir: string): void {
  writeFileSync(join(outDir, "og.png"), renderPng(cardSvg(collectReleases())));
}

// `node .vitepress/og/og.ts [file]` writes the card without building the site, to look at it.
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const file = resolve(process.argv[2] ?? "og.png");
  mkdirSync(dirname(file), { recursive: true });
  writeFileSync(file, renderPng(cardSvg(collectReleases())));
  console.log(`wrote ${file}`);
}
