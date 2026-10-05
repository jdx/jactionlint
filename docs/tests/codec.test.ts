import { describe, expect, it } from "vitest";
import { decodeSource, encodeSource } from "../.vitepress/theme/playground/codec";
import { defaultSource } from "../.vitepress/theme/playground/sample";
import { getUrlToFetch, linkifyMessage } from "../.vitepress/theme/playground/remote";

describe("permalink codec", () => {
  it("decodes a link produced by the original playground", () => {
    // A real link from docs/checks.md, produced by the original playground
    expect(decodeSource("eNrKz7NSKCgtzuDKyk8qtgIMACULBOo=")).toBe("on: push\njobs:");
  });

  it("round-trips sources", () => {
    for (const src of ["", "on: push", defaultSource, "名前: 日本語 🎉\n", "x: ${{ a }}\n".repeat(20000)]) {
      expect(decodeSource(encodeSource(src))).toBe(src);
    }
  });

  it("encodes the same bytes as pako.deflate + btoa (the original format)", async () => {
    const { deflate } = await import("pako");
    const src = defaultSource;
    const expected = btoa(String.fromCharCode(...deflate(new TextEncoder().encode(src))));
    expect(encodeSource(src)).toBe(expected);
  });

  it("accepts percent-encoded hashes", () => {
    const hash = encodeSource("on: push\n");
    expect(decodeSource(encodeURIComponent(hash))).toBe("on: push\n");
  });
});

describe("remote sources", () => {
  it("converts GitHub blob URLs", () => {
    expect(getUrlToFetch("https://github.com/o/r/blob/main/.github/workflows/ci.yaml")).toBe(
      "https://raw.githubusercontent.com/o/r/main/.github/workflows/ci.yaml",
    );
  });

  it("converts Gist URLs", () => {
    expect(getUrlToFetch("https://gist.github.com/u/0123abcd")).toBe(
      "https://gist.githubusercontent.com/u/0123abcd/raw",
    );
  });

  it("leaves other URLs alone", () => {
    expect(getUrlToFetch("https://example.com/a.yaml")).toBe("https://example.com/a.yaml");
  });

  it("linkifies URLs in messages", () => {
    expect(linkifyMessage("see https://x.dev/a for more")).toEqual([
      { text: "see " },
      { text: "https://x.dev/a", url: "https://x.dev/a" },
      { text: " for more" },
    ]);
  });
});
