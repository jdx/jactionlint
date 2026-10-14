// What the home page (theme/HomePage.vue) draws from the other pages at build time, so that it cannot go stale:
//
//   - the findings of the demo, parsed from the <!-- demo:output --> block of index.md. TestDocsDemoIsCurrent
//     (docs_demo_test.go) checks that block against what jactionlint prints, and the page marks each finding in
//     the workflow of the <!-- demo:input --> block;
//   - the number of rules, read from the table of the generated rules.md.
import { readFileSync } from "node:fs";
import { basename } from "node:path";

export type RuleGroup = "correctness" | "security" | "policy" | "style";

export interface DemoFinding {
  line: number;
  column: number;
  /** The number of characters that the ^~~~ marker under the line covers. */
  length: number;
  rule: string;
  group: RuleGroup;
  message: string;
}

export interface HomeData {
  findings: DemoFinding[];
  rules: { total: number; security: number; fixable: number };
}

declare const data: HomeData;
export { data };

interface RuleRow {
  group: RuleGroup;
  fixable: boolean;
}

/** The rows of the table at the top of rules.md: | [id](#id) | Summary | Group | Default level | Profile | Fixable | */
function parseRules(md: string): Map<string, RuleRow> {
  const rules = new Map<string, RuleRow>();
  const row = /^\| \[([a-z0-9-]+)\]\(#[a-z0-9-]+\) \| .* \| (correctness|security|policy|style) \| (?:error|warn|info) \| .* \| (yes|no) \|$/;
  for (const line of md.split("\n")) {
    const m = row.exec(line);
    if (m) rules.set(m[1], { group: m[2] as RuleGroup, fixable: m[3] === "yes" });
  }
  if (rules.size === 0) throw new Error("home.data.ts: found no rule in the table of rules.md");
  return rules;
}

/** The content of the code fence on the line after "<!-- marker -->", as demoBlock in docs_demo_test.go reads it. */
function demoBlock(md: string, marker: string): string {
  const lines = md.split("\n");
  const at = lines.findIndex((l) => l.trim() === `<!-- ${marker} -->`);
  if (at < 0 || !lines[at + 1]?.startsWith("```")) throw new Error(`home.data.ts: index.md has no <!-- ${marker} --> block`);
  const end = lines.findIndex((l, i) => i > at + 1 && l.startsWith("```"));
  if (end < 0) throw new Error(`home.data.ts: the <!-- ${marker} --> block of index.md is not closed`);
  return lines.slice(at + 2, end).join("\n");
}

/**
 * The findings of the default text format:
 *
 *   test.yaml:9:24: "github.event.head_commit.message" is potentially untrusted. ... [template-injection]
 *     |
 *   9 |       - run: echo "${{ github.event.head_commit.message }}"
 *     |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 */
function parseFindings(output: string, rules: Map<string, RuleRow>): DemoFinding[] {
  const findings: DemoFinding[] = [];
  const header = /^[^:\s][^:]*:(\d+):(\d+): (.*) \[([a-z0-9-]+)\]$/;
  const marker = /^\s*\|\s*(\^~*)\s*$/;
  for (const line of output.split("\n")) {
    const h = header.exec(line);
    if (h) {
      const rule = rules.get(h[4]);
      if (!rule) throw new Error(`home.data.ts: the demo output names the rule ${h[4]}, which rules.md does not list`);
      findings.push({ line: Number(h[1]), column: Number(h[2]), length: 1, rule: h[4], group: rule.group, message: h[3] });
      continue;
    }
    const m = marker.exec(line);
    if (m && findings.length > 0) findings[findings.length - 1].length = m[1].length;
  }
  if (findings.length === 0) throw new Error("home.data.ts: found no finding in the <!-- demo:output --> block of index.md");
  return findings;
}

export default {
  watch: ["../index.md", "../rules.md"],
  load(files: string[]): HomeData {
    const read = (name: string) => {
      const file = files.find((f) => basename(f) === name);
      if (!file) throw new Error(`home.data.ts: ${name} is not among the watched files`);
      return readFileSync(file, "utf8").replace(/\r\n/g, "\n"); // a Windows checkout can have CRLF
    };
    const rules = parseRules(read("rules.md"));
    const all = [...rules.values()];
    return {
      findings: parseFindings(demoBlock(read("index.md"), "demo:output"), rules),
      rules: {
        total: rules.size,
        security: all.filter((r) => r.group === "security").length,
        fixable: all.filter((r) => r.fixable).length,
      },
    };
  },
};
