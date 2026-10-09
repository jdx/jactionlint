import { readFileSync } from "node:fs";
import { beforeAll, describe, expect, it } from "vitest";
import { decodeSource } from "../.vitepress/theme/playground/codec";
import type { LintError } from "../.vitepress/theme/playground/wasm";
import { checksMd, loadRealLinter } from "./helpers";

let lint: (source: string) => LintError[];

beforeAll(async () => {
  lint = await loadRealLinter();
});

describe("main.wasm", () => {
  it("reports a missing runs-on", () => {
    const errs = lint("\non: push\n\njobs:\n  test:\n    steps:\n      - run: echo 'hi'");
    expect(errs).toEqual([
      { id: "workflow-syntax", message: '"runs-on" section is missing in job "test"', line: 5, column: 3, severity: "error" },
    ]);
  });

  it("reports an unknown event", () => {
    const errs = lint("\non: foo\n\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo 'hi'");
    expect(errs).toHaveLength(1);
    expect(errs[0].message).toContain('unknown Webhook event "foo"');
    expect([errs[0].line, errs[0].column, errs[0].id]).toEqual([2, 5, "unknown-event"]);
  });

  it("reports no error for a valid workflow", () => {
    expect(lint("\non: push\n\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo 'hi'")).toEqual([]);
  });
});

// Every permalink in docs/checks.md was produced by the original playground. Decode each of them,
// lint the source with the real wasm and compare the messages with the "Output:" block above it
// (positions differ: the permalink source has the `# ERROR:` comment lines of the example removed).
describe("permalinks in docs/checks.md", () => {
  const md = readFileSync(checksMd, "utf8");
  const re = /```\n((?:(?!```)[\s\S])*?)```\n\n\[Playground\]\(https:\/\/jactionlint\.jdx\.dev\/#([A-Za-z0-9+/=]+)\)/g;
  const links: { output: string; hash: string }[] = [];
  for (const m of md.matchAll(re)) links.push({ output: m[1], hash: m[2] });

  it("finds all of them", () => {
    // The count is derived from the document, so adding a section does not need an edit here. A link which
    // the pattern above does not match (a missing blank line, a changed URL) would be skipped silently
    // otherwise.
    const all = (md.match(/\[Playground\]\(https:\/\/jactionlint\.jdx\.dev\/#/g) ?? []).length;
    expect(links.length, `${all} playground links in docs/checks.md but only ${links.length} follow an Output block`).toBe(all);
    expect(links.length).toBeGreaterThanOrEqual(45);
  });

  it("first link is the unexpected keys example", () => {
    const src = decodeSource(links[0].hash);
    expect(src).toContain("Shell: bash");
    const errs = lint(src);
    // The permalink source has the `# ERROR:` comments of the documented input removed
    expect(errs.map((e) => [e.line, e.column])).toEqual([
      [5, 5],
      [10, 9],
    ]);
    expect(errs[0].message).toContain('unexpected key "default" for "job" section');
  });

  it("every link decodes and reproduces the documented errors", () => {
    expect(links.length).toBeGreaterThan(0);
    const failures: string[] = [];
    // Positions inside messages ("previously defined at line 7, column 9") shift as well
    const norm = (s: string) => s.replace(/line[: ]\s*\d+,\s*col(?:umn)?[: ]\s*\d+/g, "line:N,col:N");
    for (const { output, hash } of links) {
      const errs = lint(decodeSource(hash));
      // The CLI prints "warning: " or "info: " before the message of a finding which is not an error and the
      // rule ID in brackets. The playground runs the default profile without a configuration file, so a
      // section whose rule is not on by default has to skip its playground link (see CONTRIBUTING.md).
      const actual = errs.map((e) => norm(`${prefix(e.severity)}${e.message} [${e.id}]`));
      const documented = [...output.matchAll(/^test\.yaml:\d+:\d+: (.*)$/gm)].map((m) => norm(m[1]))
        // The CLI reads local files, which the wasm build cannot
        .filter((m) => !m.includes("no such file or directory"));
      if (JSON.stringify(actual) !== JSON.stringify(documented)) {
        failures.push(`${hash.slice(0, 24)}...\n  documented: ${JSON.stringify(documented)}\n  playground: ${JSON.stringify(actual)}`);
      }
    }
    expect(failures, `${failures.length} permalink(s) differ from their documented output`).toEqual([]);
  });
});

function prefix(severity: string | undefined): string {
  return severity === "warn" ? "warning: " : severity === "info" ? "info: " : "";
}
