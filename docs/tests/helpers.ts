import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { hooks, loadLinter, runLint, type LintError } from "../.vitepress/theme/playground/wasm";

const docsDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
export const wasmDir = resolve(docsDir, "public/playground");
export const checksMd = resolve(docsDir, "checks.md");

/** Load the real main.wasm (built by `mise run docs:wasm`) the way the browser page does. */
export async function loadRealLinter(): Promise<(source: string) => LintError[]> {
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

  return (source) => {
    current = null;
    runLint(source, win);
    if (current === null) throw new Error("runActionlint did not report a result");
    return current;
  };
}
