// @vitest-environment happy-dom
// Smoke test of the Vue component in a fake DOM, running the real wasm. CodeMirror needs layout APIs
// that happy-dom only approximates, so this only checks the wiring (editor, lint, results, permalink).
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve } from "node:path";
import { createApp, nextTick, ref } from "vue";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { encodeSource } from "../.vitepress/theme/playground/codec";
import { wasmDir } from "./helpers";

// A stand-in for the VitePress router: the component installs its route change guard on it
const router = vi.hoisted(() => ({ onBeforeRouteChange: undefined as ((to: string) => unknown) | undefined }));

vi.mock("vitepress", () => ({
  useData: () => ({ isDark: ref(false) }),
  useRouter: () => router,
  withBase: (s: string) => s,
}));

async function until(cond: () => boolean, what: string) {
  for (let i = 0; i < 300; i++) {
    if (cond()) return;
    await new Promise((r) => setTimeout(r, 20));
  }
  throw new Error(`timed out waiting for ${what}: ${document.body.querySelector(".jal-results")?.outerHTML}`);
}

describe("Playground component", () => {
  const root = document.createElement("div");
  document.body.appendChild(root);
  let app: ReturnType<typeof createApp>;
  const source = "on: foo\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n";

  beforeAll(async () => {
    createRequire(import.meta.url)(resolve(wasmDir, "wasm_exec.js"));
    const wasm = readFileSync(resolve(wasmDir, "main.wasm"));
    // happy-dom's Response is not accepted by node's instantiateStreaming; use the fallback path
    // (the one Safari used to need) in this test
    (WebAssembly as { instantiateStreaming?: unknown }).instantiateStreaming = undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(wasm, { headers: { "Content-Type": "application/wasm" } })),
    );
    // Old-style permalink in the hash, decoded on mount
    window.location.hash = encodeSource(source);
    const Playground = (await import("../.vitepress/theme/playground/Playground.vue")).default;
    app = createApp(Playground);
    app.mount(root);
  });

  afterAll(() => app?.unmount());

  it("installs a guard for client-side route changes that lets a pristine page navigate", async () => {
    await until(() => typeof router.onBeforeRouteChange === "function", "the route guard");
    // The permalink was just loaded, so nothing is unsaved and navigation is not blocked or confirmed
    const confirm = vi.fn(() => false);
    const original = window.confirm;
    window.confirm = confirm;
    expect(typeof router.onBeforeRouteChange).toBe("function");
    expect(router.onBeforeRouteChange?.("/install")).toBeUndefined();
    expect(confirm).not.toHaveBeenCalled();
    window.confirm = original;
  });

  it("loads the source from the permalink and lists the errors", async () => {
    await until(() => root.querySelector(".jal-errors") !== null, "lint results");
    expect(root.querySelector(".cm-content")?.textContent).toContain("on: foo");
    const rows = root.querySelectorAll(".jal-errors button");
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain('unknown Webhook event "foo"');
    expect(rows[0].textContent).toContain("line:1, col:5");
    expect(rows[0].textContent).toContain("unknown-event");
    expect(root.querySelector(".jal-note")).toBeNull(); // loading note is gone
    await nextTick();
  });
});
