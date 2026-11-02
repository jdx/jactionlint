import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { hooks, loadLinter, runLint, type LintError } from "../.vitepress/theme/playground/wasm";

const docsDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
export const wasmDir = resolve(docsDir, "public/playground");
export const checksMd = resolve(docsDir, "checks.md");

/** Load the real main.wasm (built by `mise run docs:wasm`) the way the browser page does. */
// exampleDay is the day scripts/check-checks lints the examples of docs/checks.md on (exampleDay in its main.go), as
// YYYY-MM-DD. A permalink is linted on the same day, so that a date-dependent check gives the documented output.
export function exampleDay(): string {
  const src = readFileSync(resolve(docsDir, "../scripts/check-checks/main.go"), "utf8");
  const m = src.match(/var exampleDay = time\.Date\((\d{4}), time\.(\w+), (\d{1,2}),/);
  if (!m) throw new Error("exampleDay is not found in scripts/check-checks/main.go");
  const months = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
  const month = months.indexOf(m[2]) + 1;
  if (month === 0) throw new Error(`unknown month ${m[2]} in exampleDay`);
  return `${m[1]}-${String(month).padStart(2, "0")}-${m[3].padStart(2, "0")}`;
}

export async function loadRealLinter(): Promise<(source: string, day?: string) => LintError[]> {
  const wasm = resolve(wasmDir, "main.wasm");
  const wasmExec = resolve(wasmDir, "wasm_exec.js");
  if (!existsSync(wasm) || !existsSync(wasmExec)) {
    throw new Error(`${wasm} and wasm_exec.js are missing. Build them first: mise run docs:wasm`);
  }

  // main.go talks to `window`; in node there is none, so provide one
  const win: Record<string, unknown> = {};
  (globalThis as Record<string, unknown>).window = win;
  createRequire(import.meta.url)(wasmExec); // defines globalThis.Go
  const Go = (globalThis as unknown as { Go: new () => never }).Go;

  let current: LintError[] | null = null;
  hooks.onCheckCompleted = (errs) => (current = errs);
  hooks.showError = (msg) => {
    throw new Error(`wasm reported: ${msg}`);
  };
  // main() lints the source of getSource() once on startup
  hooks.getSource = () => "on: push\njobs: {}\n";
  await loadLinter(Go, readFileSync(wasm), win);
  // go.run() is not awaited; wait for the startup lint
  for (let i = 0; i < 500 && current === null; i++) await new Promise((r) => setTimeout(r, 10));
  if (current === null) throw new Error("wasm did not finish starting");

  return (source, day) => {
    current = null;
    runLint(source, win, day);
    if (current === null) throw new Error("runActionlint did not report a result");
    return current;
  };
}
