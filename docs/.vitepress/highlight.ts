// Syntax highlighting for the code blocks of the home page (theme/HomePage.vue).
import type { MarkdownOptions } from "vitepress";

type Language = NonNullable<MarkdownOptions["languages"]>[number];
type Transformer = NonNullable<MarkdownOptions["codeTransformers"]>[number];

/**
 * The default text format of jactionlint, colored the way a terminal shows it: the position and the rule ID of a
 * finding dimmed, the message bold and the ^~~~ marker green. Use it as ```jactionlint-output.
 *
 *   test.yaml:3:5: unexpected key "branch" for "push" section. ... [workflow-syntax]
 *     |
 *   3 |     branch: main
 *     |     ^~~~~~~
 */
export const jactionlintOutput: Language = {
  name: "jactionlint-output",
  scopeName: "text.jactionlint-output",
  patterns: [
    {
      match: "^(.+?)(:)(\\d+)(:)(\\d+)(:) (.*) (\\[[a-z0-9-]+\\])$",
      captures: {
        1: { name: "variable.path.jactionlint-output" },
        2: { name: "comment.separator.jactionlint-output" },
        3: { name: "constant.numeric.line.jactionlint-output" },
        4: { name: "comment.separator.jactionlint-output" },
        5: { name: "constant.numeric.column.jactionlint-output" },
        6: { name: "comment.separator.jactionlint-output" },
        7: { name: "markup.bold.message.jactionlint-output" },
        8: { name: "comment.rule.jactionlint-output" },
      },
    },
    {
      match: "^(\\s*\\|)(\\s*)(\\^~*)$",
      captures: {
        1: { name: "comment.gutter.jactionlint-output" },
        3: { name: "entity.name.tag.marker.jactionlint-output" },
      },
    },
    {
      match: "^(\\s*\\d+ \\|)",
      captures: { 1: { name: "comment.gutter.jactionlint-output" } },
    },
    { match: "^\\s*\\|$", name: "comment.gutter.jactionlint-output" },
  ],
};

/**
 * Marks the lines of a ```diff block as added, removed or a hunk header, so that CSS can give a whole line a
 * background: Shiki colors only the text of the line.
 */
export const diffLines: Transformer = {
  name: "jactionlint:diff-lines",
  line(node, line) {
    if (this.options.lang !== "diff") return;
    const text = this.source.split("\n")[line - 1] ?? "";
    if (text.startsWith("+++") || text.startsWith("---")) this.addClassToHast(node, "diff-line-file");
    else if (text.startsWith("+")) this.addClassToHast(node, "diff-line-add");
    else if (text.startsWith("-")) this.addClassToHast(node, "diff-line-remove");
    else if (text.startsWith("@@")) this.addClassToHast(node, "diff-line-hunk");
  },
};
