// Generates llms.txt (an index of the pages) and llms-full.txt (all pages concatenated) so that
// LLM tools can read the docs. Called from the VitePress `buildEnd` hook; the files land in the
// build output and are served at https://jactionlint.jdx.dev/llms.txt and /llms-full.txt.
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

import { sidebar, type SidebarItem } from "./sidebar.ts";

const siteUrl = "https://jactionlint.jdx.dev";
const title = "jactionlint";
const summary =
  "A static checker for GitHub Actions workflow files: syntax, type-checked expressions, action inputs, reusable workflows, shellcheck and security checks.";
const maxDescription = 200;

interface Entry {
  text: string;
  link: string;
}

function flatten(items: SidebarItem[], entries: Entry[] = []): Entry[] {
  for (const item of items) {
    if (item.link?.startsWith("/")) entries.push({ text: item.text ?? item.link, link: item.link });
    if (item.items) flatten(item.items, entries);
  }
  return entries;
}

function plain(markdown: string): string {
  return markdown
    .replace(/!\[[^\]]*\]\([^)]*\)/g, "")
    .replace(/\[((?:[^[\]]|\[[^[\]]*\])*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]+)\]\[[^\]]*\]/g, "$1")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/[*_]{1,3}([^*_]+)[*_]{1,3}/g, "$1")
    .replace(/<\/?[a-z][^>]*>|<!--[\s\S]*?-->/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

/** Remove front matter, returning it and the body. */
function splitFrontMatter(md: string): { front: string; body: string } {
  const m = /^---\n([\s\S]*?)\n---\n?/.exec(md);
  return m ? { front: m[1], body: md.slice(m[0].length) } : { front: "", body: md };
}

/** First prose paragraph after the page heading (ATX `# x` or setext `x\n===`). */
function description(body: string): string | undefined {
  const lines = body.split("\n");
  let i = 0;
  for (; i < lines.length; i++) {
    if (lines[i].startsWith("# ") || /^=+\s*$/.test(lines[i + 1] ?? "x")) break;
  }
  i += lines[i]?.startsWith("# ") ? 1 : 2;

  const paragraph: string[] = [];
  for (; i < lines.length; i++) {
    const line = lines[i].trim();
    if (paragraph.length === 0) {
      if (line === "" || /^(#|:::|```|<|\||- |\* |\d+\. |\[!\[|!\[)/.test(line)) continue;
    } else if (line === "" || /^(#|:::|```)/.test(line)) {
      break;
    }
    paragraph.push(line);
  }

  const text = plain(paragraph.join(" ")).replace(/:$/, ".");
  if (!text) return undefined;
  if (text.length <= maxDescription) return text;
  const sentence = text.slice(0, maxDescription).lastIndexOf(". ");
  if (sentence > maxDescription / 2) return text.slice(0, sentence + 1);
  const word = text.lastIndexOf(" ", maxDescription);
  return `${text.slice(0, word > 0 ? word : maxDescription)}…`;
}

export function generateLlms(srcDir: string, outDir: string): void {
  const index: string[] = [];
  const full: string[] = [];

  for (const group of sidebar) {
    const entries = flatten(group.items ?? []);
    if (entries.length === 0) continue;
    const lines: string[] = [];
    for (const { text, link } of entries) {
      let md: string;
      try {
        md = readFileSync(resolve(srcDir, `${link.replace(/^\//, "")}.md`), "utf8");
      } catch {
        continue;
      }
      const { front, body } = splitFrontMatter(md);
      const fm = /^description:\s*(.+)$/m.exec(front)?.[1]?.replace(/^['"]|['"]$/g, "");
      const desc = fm ? plain(fm) : description(body);
      lines.push(`- [${text}](${siteUrl}${link}): ${desc ?? text}`);
      full.push(`# ${text}\n\nSource: ${siteUrl}${link}\n\n${body.trim()}\n`);
    }
    if (lines.length > 0) index.push(`## ${group.text}\n\n${lines.join("\n")}`);
  }

  mkdirSync(outDir, { recursive: true });
  writeFileSync(join(outDir, "llms.txt"), `# ${title}\n\n> ${summary}\n\n${index.join("\n\n")}\n`);
  writeFileSync(join(outDir, "llms-full.txt"), `# ${title}\n\n> ${summary}\n\n${full.join("\n---\n\n")}`);
}
