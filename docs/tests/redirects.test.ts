import { describe, expect, it } from "vitest";
import { redirectFor, redirects } from "../.vitepress/redirects";

describe("redirects", () => {
  it("ignores .html and a trailing slash of the old location", () => {
    expect(redirectFor("/usage", "#online-checks")).toBe("/online#online-checks");
    expect(redirectFor("/usage.html", "#online-checks")).toBe("/online#online-checks");
    expect(redirectFor("/usage/", "#online-checks")).toBe("/online#online-checks");
  });

  it("leaves other locations alone", () => {
    expect(redirectFor("/usage", "")).toBeUndefined();
    expect(redirectFor("/usage", "#choose-a-profile")).toBeUndefined();
    expect(redirectFor("/checks", "#hk")).toBeUndefined();
  });

  // A target that is itself redirected would send the browser around in circles (or at least through two hops).
  it("never redirects to a location that is redirected again", () => {
    for (const [from, to] of Object.entries(redirects)) {
      const [path, hash = ""] = to.split("#");
      expect(redirectFor(path, hash ? `#${hash}` : ""), `${from} -> ${to}`).toBeUndefined();
    }
  });
});
