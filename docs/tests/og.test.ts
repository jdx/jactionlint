import { describe, expect, it } from "vitest";
import {
  cardSvg,
  collectReleases,
  countGroups,
  height,
  label,
  previousMinors,
  renderPng,
  total,
  width,
  type Release,
} from "../.vitepress/og/og";
import { version } from "../.vitepress/version";

describe("countGroups", () => {
  it("finds the Group column wherever it is", () => {
    // The table of 2.1.0
    const v21 = [
      "| ID | Group | Default level | Profile |",
      "| --- | --- | --- | --- |",
      "| [a](#a) | correctness | error | correctness |",
      "| [b](#b) | security | warn | default |",
      "| [c](#c) | style | error | default |",
    ].join("\n");
    expect(countGroups(v21)).toEqual({ correctness: 1, security: 1, policy: 1 });

    // The table of 2.2.0, whose summaries may contain an escaped pipe
    const v22 = [
      "| ID | Summary | Group | Default level | Profile | Fixable |",
      "| --- | --- | --- | --- | --- | --- |",
      "| [a](#a) | Runs `a \\| b`. | security | error | default | no |",
      "| [b](#b) | B. | policy | warn | pedantic | no |",
      "| [c](#c) | C. | policy | warn | pedantic | no |",
    ].join("\n");
    expect(countGroups(v22)).toEqual({ correctness: 0, security: 1, policy: 2 });
  });

  it("returns undefined without a rule table", () => {
    expect(countGroups("# Rules\n\nNothing here.")).toBeUndefined();
    expect(countGroups("| ID | Level |\n| --- | --- |\n| [a](#a) | error |")).toBeUndefined();
  });
});

describe("previousMinors", () => {
  const tags = ["v1.8.2", "v2.0.0", "v2.0.2", "v2.1.0", "v2.1.1-rc.1", "v2.2.0", "v2.3.0", "nightly"];

  it("takes the last patch of the minor releases before the current one", () => {
    expect(previousMinors(tags, "2.2.0")).toEqual(["v2.0.2", "v2.1.0"]);
    expect(previousMinors(tags, "2.2.3")).toEqual(["v2.0.2", "v2.1.0"]);
    expect(previousMinors(tags, "2.0.0")).toEqual(["v1.8.2"]);
    expect(previousMinors(tags, "2.2.0", 3)).toEqual(["v1.8.2", "v2.0.2", "v2.1.0"]);
  });

  it("returns nothing for a version it cannot read", () => {
    expect(previousMinors(tags, "dev")).toEqual([]);
  });
});

describe("cardSvg", () => {
  const measure = (text: string) => text.length * 10;
  const releases: Release[] = [
    { label: "2.0", counts: { correctness: 66, security: 43, policy: 22 } },
    { label: "2.1", counts: { correctness: 67, security: 44, policy: 22 } },
    { label: "2.2", counts: { correctness: 81, security: 54, policy: 26 } },
  ];

  it("shows the version, the rule counts and what is new", () => {
    const svg = cardSvg(releases, measure);
    expect(svg).toContain(">2.2</tspan>");
    expect(svg).toContain("28 new rules since 2.1.");
    for (const n of ["131", "133", "161", "+28"]) expect(svg).toContain(`>${n}</text>`);
  });

  it("falls back to the total without an earlier release or new rules", () => {
    expect(cardSvg(releases.slice(2), measure)).toContain("161 rules for GitHub Actions workflows.");
    const same = [releases[1], { ...releases[1], label: "2.2" }];
    const svg = cardSvg(same, measure);
    expect(svg).toContain("133 rules for GitHub Actions workflows.");
    expect(svg).not.toContain(">+");
  });

  it("keeps the bars inside the card when the rules grow", () => {
    const big: Release[] = [{ label: "9.0", counts: { correctness: 400, security: 300, policy: 100 } }];
    const ends = [...cardSvg(big, measure).matchAll(/<rect x="(\d+)" y="\d+" width="(\d+)" height="26"/g)].map(
      (m) => Number(m[1]) + Number(m[2]),
    );
    expect(Math.max(...ends)).toBeLessThan(width - 150);
  });
});

describe("og.png", () => {
  it("renders the card of this checkout", () => {
    const releases = collectReleases();
    expect(releases[releases.length - 1].label).toBe(label(version));
    for (const r of releases) expect(total(r.counts)).toBeGreaterThan(0);

    const png = renderPng(cardSvg(releases));
    expect(png.subarray(1, 4).toString()).toBe("PNG");
    expect(png.readUInt32BE(16)).toBe(width);
    expect(png.readUInt32BE(20)).toBe(height);
  });
});
