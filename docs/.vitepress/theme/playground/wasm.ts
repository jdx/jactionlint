// Glue between the page and the jactionlint wasm module built from ./playground/main.go.
//
// main.go talks to its host only through functions on `window`:
//   window.getYamlSource() / onCheckCompleted(errors) / showError(msg) / dismissLoading()  (host -> called by wasm)
//   window.runActionlint(source)                                                          (wasm -> called by host)
// This module installs the host functions and forwards them to a swappable `hooks` object, so
// the wasm module is instantiated once even if the component is remounted. It is shared with the
// node tests, which load the same wasm.

export interface LintError {
  kind: string;
  message: string;
  line: number;
  column: number;
  /** "error", "warn" or "info" */
  severity?: string;
}

export interface Hooks {
  getSource(): string;
  onCheckCompleted(errors: LintError[]): void;
  showError(message: string): void;
  dismissLoading(): void;
}

export const hooks: Hooks = {
  getSource: () => "",
  onCheckCompleted: () => {},
  showError: () => {},
  dismissLoading: () => {},
};

interface GoLike {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<unknown>;
}

type Win = Record<string, any>;

let loading: Promise<void> | null = null;

/**
 * Instantiate the wasm module (once) and start it. `Go` must come from Go's wasm_exec.js.
 * `module` is the wasm bytes or a promise of a fetch Response.
 */
export function loadLinter(
  GoClass: new () => GoLike,
  module: BufferSource | Promise<Response>,
  win: Win = globalThis as Win,
): Promise<void> {
  if (loading) return loading;
  loading = (async () => {
    win.getYamlSource = () => hooks.getSource();
    win.onCheckCompleted = (errs: LintError[]) => hooks.onCheckCompleted(errs);
    win.showError = (msg: string) => hooks.showError(msg);
    win.dismissLoading = () => hooks.dismissLoading();

    const go = new GoClass();
    let result: WebAssembly.WebAssemblyInstantiatedSource;
    if (module instanceof Promise) {
      // WebAssembly.instantiateStreaming is not implemented in some older browsers
      if (typeof WebAssembly.instantiateStreaming === "function") {
        result = await WebAssembly.instantiateStreaming(module, go.importObject);
      } else {
        const res = await module;
        result = await WebAssembly.instantiate(await res.arrayBuffer(), go.importObject);
      }
    } else {
      result = await WebAssembly.instantiate(module, go.importObject);
    }
    // go.run() never settles because main() blocks forever; do not await it.
    go.run(result.instance).catch((e: unknown) => hooks.showError(String(e)));
  })().catch((e) => {
    loading = null;
    throw e;
  });
  return loading;
}

export function runLint(source: string, win: Win = globalThis as Win): void {
  if (typeof win.runActionlint !== "function") {
    throw new Error("jactionlint wasm is not loaded yet");
  }
  win.runActionlint(source);
}
