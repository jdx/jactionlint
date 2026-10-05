<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue";
import { useData, useRouter, withBase } from "vitepress";
import { decodeSource, encodeSource } from "./codec";
import { defaultSource } from "./sample";
import { getRemoteSource, linkifyMessage } from "./remote";
import { hooks, loadLinter, runLint, type LintError } from "./wasm";
import type { Editor } from "./editor";
import { version } from "../../version";

const { isDark } = useData();

const editorEl = ref<HTMLElement | null>(null);
const loading = ref(true);
const errorMessage = ref("");
const success = ref(false);
const errors = shallowRef<LintError[]>([]);
const urlInput = ref("");
const invalidInput = ref("");
const permalinkLabel = ref("Permalink");

let editor: Editor | null = null;
let wasmReady = false;
let contentChanged = false;
let debounceId: number | undefined;
// The last source seen in the URL hash, to tell our own permalink updates from navigation
let hashSource: string | null = null;

async function getInitialSource(): Promise<string> {
  const params = new URLSearchParams(window.location.search);

  const s = params.get("s");
  if (s !== null) return s;

  const u = params.get("u");
  if (u !== null) return getRemoteSource(u);

  if (window.location.hash.length > 1) {
    return decodeSource(window.location.hash.slice(1)); // Omit first '#'
  }
  return defaultSource;
}

function showResult(errs: LintError[]) {
  errors.value = errs;
  success.value = errs.length === 0;
  if (errs.length === 0) editor?.clearErrors();
  else editor?.setErrors(errs);
}

function startLint() {
  debounceId = undefined;
  if (!editor) return;
  errorMessage.value = "";
  success.value = false;
  invalidInput.value = "";
  try {
    runLint(editor.getValue());
  } catch (e) {
    errorMessage.value = e instanceof Error ? e.message : String(e);
  }
}

function onEditorChange(paste: boolean) {
  contentChanged = true;
  if (!wasmReady) {
    errorMessage.value = "Preparing Wasm file is not completed yet. Please wait for a while and try again.";
    return;
  }
  window.clearTimeout(debounceId);
  if (paste) {
    startLint(); // When pasting some code, apply jactionlint instantly
    return;
  }
  // Mobile keyboards are slow, give them more time
  const delay = window.matchMedia("(pointer: coarse)").matches ? 1000 : 300;
  debounceId = window.setTimeout(startLint, delay);
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error(`Failed to load ${src}`));
    document.head.appendChild(s);
  });
}

async function checkUrl() {
  const input = urlInput.value;
  let src: string;
  try {
    src = await getRemoteSource(input);
  } catch (e) {
    invalidInput.value = `Incorrect input "${input}": ${e instanceof Error ? e.message : String(e)}`;
    return;
  }
  invalidInput.value = "";
  editor?.setValue(src);
}

async function permalink() {
  if (!editor) return;
  const hash = encodeSource(editor.getValue());
  hashSource = hash;
  window.location.hash = hash;
  contentChanged = false; // The content is saved in the URL now
  try {
    await navigator.clipboard.writeText(window.location.href);
    permalinkLabel.value = "Copied!";
  } catch {
    permalinkLabel.value = "Permalink";
  }
  window.setTimeout(() => (permalinkLabel.value = "Permalink"), 1500);
}

function onHashChange() {
  const hash = window.location.hash.slice(1);
  if (!editor || hash === "" || hash === hashSource) return;
  hashSource = hash;
  try {
    editor.setValue(decodeSource(hash));
    contentChanged = false;
  } catch (e) {
    errorMessage.value = `Invalid permalink: ${e instanceof Error ? e.message : String(e)}`;
  }
}

function onBeforeUnload(e: BeforeUnloadEvent) {
  if (contentChanged) e.preventDefault();
}

// beforeunload does not run when VitePress navigates on the client (nav links, sidebar, search), so guard
// route changes too. A hash-only change on this page is a permalink being loaded and is not a navigation.
const router = useRouter();
let previousBeforeRouteChange: typeof router.onBeforeRouteChange;
function onBeforeRouteChange(to: string) {
  const target = new URL(to, window.location.href);
  const samePage = target.pathname === window.location.pathname;
  if (!samePage && contentChanged && !window.confirm("Changes you made may not be saved. Leave this page?")) {
    return false;
  }
  return previousBeforeRouteChange?.(to);
}

function goTo(e: LintError) {
  editor?.goTo(e.line, e.column);
}

onMounted(async () => {
  hashSource = window.location.hash.slice(1) || null;

  let initial = defaultSource;
  let initialError = "";
  try {
    initial = await getInitialSource();
  } catch (e) {
    initialError = e instanceof Error ? e.message : String(e);
  }

  const { createEditor } = await import("./editor");
  if (!editorEl.value) return; // unmounted while loading
  editor = createEditor({
    parent: editorEl.value,
    doc: initial,
    dark: isDark.value,
    onChange: onEditorChange,
  });

  hooks.getSource = () => editor?.getValue() ?? "";
  hooks.onCheckCompleted = showResult;
  hooks.showError = (msg) => (errorMessage.value = msg);
  hooks.dismissLoading = () => (loading.value = false);

  window.addEventListener("beforeunload", onBeforeUnload);
  window.addEventListener("hashchange", onHashChange);
  previousBeforeRouteChange = router.onBeforeRouteChange;
  router.onBeforeRouteChange = onBeforeRouteChange;

  try {
    if (typeof (window as any).Go === "undefined") {
      await loadScript(withBase("/playground/wasm_exec.js"));
    }
    await loadLinter((window as any).Go, fetch(withBase("/playground/main.wasm")));
    wasmReady = true;
    loading.value = false;
    startLint();
    if (initialError) errorMessage.value = initialError;
  } catch (e) {
    loading.value = false;
    errorMessage.value = `Failed to load jactionlint WebAssembly: ${e instanceof Error ? e.message : String(e)}`;
  }
});

watch(isDark, (dark) => editor?.setDark(dark));

onBeforeUnmount(() => {
  window.clearTimeout(debounceId);
  window.removeEventListener("beforeunload", onBeforeUnload);
  window.removeEventListener("hashchange", onHashChange);
  router.onBeforeRouteChange = previousBeforeRouteChange;
  hooks.getSource = () => "";
  hooks.onCheckCompleted = () => {};
  hooks.showError = () => {};
  hooks.dismissLoading = () => {};
  editor?.destroy();
  editor = null;
});
</script>

<template>
  <div class="jal-playground">
    <div class="jal-bar">
      <div class="jal-title">
        <h1>Playground</h1>
        <a
          class="jal-version"
          rel="noopener"
          :href="`https://github.com/jdx/jactionlint/releases/tag/v${version}`"
          >v{{ version }}</a
        >
        <p>Run jactionlint in your browser. Nothing you type leaves the page.</p>
      </div>
      <div class="jal-controls">
        <button type="button" class="jal-button" @click="permalink">{{ permalinkLabel }}</button>
        <form class="jal-url" @submit.prevent="checkUrl">
          <input
            v-model="urlInput"
            type="text"
            placeholder="GitHub or Gist URL"
            aria-label="GitHub or Gist URL of a workflow"
            :class="{ 'is-invalid': invalidInput !== '' }"
            @input="urlInput === '' && (invalidInput = '')"
          />
          <button type="submit" class="jal-button">Check</button>
        </form>
      </div>
    </div>
    <p v-if="invalidInput" class="jal-invalid" role="alert">{{ invalidInput }}</p>

    <div class="jal-linter">
      <div ref="editorEl" class="jal-editor"></div>
      <div class="jal-results" aria-live="polite">
        <div v-if="loading" class="jal-note">Loading WebAssembly binary...</div>
        <div v-if="errorMessage" class="jal-note jal-note-error" role="alert">{{ errorMessage }}</div>
        <div v-if="success" class="jal-note jal-note-ok">Yay! No error was detected.</div>
        <ul v-if="errors.length > 0" class="jal-errors">
          <li v-for="(e, i) in errors" :key="i">
            <button type="button" @click="goTo(e)">
              <span class="jal-pos">line:{{ e.line }}, col:{{ e.column }}</span>
              <span class="jal-msg"
                ><template v-for="(p, j) in linkifyMessage(e.message)" :key="j"
                  ><a v-if="p.url" :href="p.url" rel="noopener" @click.stop>{{ p.text }}</a
                  ><span v-else>{{ p.text }}</span></template
                ></span
              >
              <span class="jal-kind">{{ e.kind }}</span>
            </button>
          </li>
        </ul>
      </div>
    </div>

    <p class="jal-links">
      See <a href="/checks">all checks</a>, the GitHub docs on
      <a rel="noopener" href="https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions"
        >workflow syntax</a
      >,
      <a rel="noopener" href="https://docs.github.com/en/actions/learn-github-actions/contexts">contexts</a> and
      <a rel="noopener" href="https://docs.github.com/en/actions/reference/security/secure-use">security hardening</a>.
    </p>
  </div>
</template>

<style>
.jal-playground {
  --jal-comment: #6a737d;
  --jal-string: #0a7a4a;
  --jal-number: #b45309;
  --jal-key: #0f5fa8;
  --jal-meta: #8250df;
  --jal-keyword: #cf222e;
  --jal-punct: #57606a;
  --jal-active-line: rgba(128, 128, 128, 0.1);
  --jal-selection: rgba(15, 118, 110, 0.25);
  --jal-error: #d1242f;

  max-width: 1440px;
  margin: 0 auto;
  padding: 24px 16px 48px;
}

.dark .jal-playground {
  --jal-comment: #8b949e;
  --jal-string: #7ee0b0;
  --jal-number: #f0b36a;
  --jal-key: #79c0ff;
  --jal-meta: #d2a8ff;
  --jal-keyword: #ff7b72;
  --jal-punct: #9da7b3;
  --jal-selection: rgba(52, 211, 153, 0.3);
  --jal-error: #ff6b6b;
}

@media (min-width: 640px) {
  .jal-playground {
    padding: 32px 32px 64px;
  }
}

.jal-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.jal-title h1 {
  display: inline;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.jal-title p {
  margin: 4px 0 0;
  color: var(--vp-c-text-2);
  font-size: 14px;
}

.jal-version {
  margin-left: 8px;
  padding: 2px 8px;
  border-radius: 10px;
  background: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font: 12px var(--vp-font-family-mono);
  vertical-align: middle;
  text-decoration: none;
}

.jal-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.jal-url {
  display: flex;
  gap: 8px;
}

.jal-url input {
  width: 240px;
  max-width: 100%;
  padding: 0 12px;
  height: 36px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
  font-size: 14px;
}

.jal-url input:focus {
  outline: 2px solid var(--vp-c-brand-1);
  outline-offset: -1px;
}

.jal-url input.is-invalid {
  border-color: var(--jal-error);
}

.jal-button {
  height: 36px;
  padding: 0 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: border-color 0.2s, color 0.2s;
}

.jal-button:hover {
  border-color: var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
}

.jal-invalid {
  margin: 0 0 12px;
  color: var(--jal-error);
  font-size: 13px;
  word-break: break-all;
}

.jal-linter {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

@media (min-width: 1100px) {
  .jal-linter {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  .jal-results {
    position: sticky;
    top: calc(var(--vp-nav-height) + 16px);
  }
}

.jal-editor {
  min-width: 0;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  overflow: hidden;
}

.jal-results {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.jal-note {
  padding: 12px 16px;
  border-radius: 8px;
  background: var(--vp-c-bg-soft);
  font-size: 14px;
  text-align: center;
}

.jal-note-ok {
  background: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
}

.jal-note-error {
  background: rgba(209, 36, 47, 0.1);
  color: var(--jal-error);
  text-align: left;
  white-space: pre-wrap;
}

.jal-errors {
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  overflow: hidden;
}

.jal-errors li {
  margin: 0;
}

.jal-errors li + li {
  border-top: 1px solid var(--vp-c-divider);
}

.jal-errors button {
  display: block;
  width: 100%;
  padding: 10px 14px;
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
  font-size: 14px;
  line-height: 1.5;
  text-align: left;
  cursor: pointer;
}

.jal-errors button:hover,
.jal-errors button:focus-visible {
  background: var(--vp-c-bg-elv);
}

.jal-pos,
.jal-kind {
  display: inline-block;
  padding: 0 8px;
  border-radius: 4px;
  background: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
  font: 12px/20px var(--vp-font-family-mono);
}

.jal-pos {
  margin-right: 8px;
}

.jal-kind {
  margin-left: 6px;
}

.jal-msg {
  overflow-wrap: anywhere;
}

.jal-msg a {
  color: var(--vp-c-brand-1);
  text-decoration: underline;
}

.jal-links {
  margin-top: 24px;
  color: var(--vp-c-text-2);
  font-size: 14px;
}

.jal-links a {
  color: var(--vp-c-brand-1);
}
</style>
