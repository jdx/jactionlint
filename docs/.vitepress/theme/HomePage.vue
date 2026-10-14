<script setup lang="ts">
// The landing page (docs/index.md). The code blocks that a test or the release workflow keeps up to date stay in
// index.md and fill the slots of this component: the demo workflow, its output and its fixes, and the CI workflow.
// home.data.ts reads the findings of the demo and the number of rules at build time.
import { computed, onUnmounted, ref } from "vue";
import { data as home, type RuleGroup } from "../home.data";

const installCommand = "mise use -g jactionlint";
const copied = ref<"" | "hero" | "cta">("");
const copyFailed = ref(false);
let copyTimer: ReturnType<typeof setTimeout> | undefined;

async function copyInstall(where: "hero" | "cta") {
  clearTimeout(copyTimer);
  try {
    await navigator.clipboard.writeText(installCommand);
    copyFailed.value = false;
  } catch {
    copyFailed.value = true;
  }
  copied.value = where;
  copyTimer = setTimeout(() => {
    copied.value = "";
  }, 3000);
}

onUnmounted(() => clearTimeout(copyTimer));

const copyLabel = (where: "hero" | "cta") => (copied.value === where && !copyFailed.value ? "Copied" : "Copy");
const copyNote = computed(() =>
  copyFailed.value ? "Select the command to copy it by hand." : "Copied to the clipboard.",
);

const tab = ref<"problems" | "terminal">("problems");

// The arrow keys move between the two tabs, as in the tabs pattern of WAI-ARIA.
function onTabKey(e: KeyboardEvent) {
  if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
  e.preventDefault();
  tab.value = tab.value === "problems" ? "terminal" : "problems";
  document.getElementById(`jh-tab-${tab.value}`)?.focus();
}

/** The finding under the pointer or the keyboard focus, which the workflow highlights. */
const active = ref<number | null>(null);

const groupNames: Record<RuleGroup, string> = {
  correctness: "Correctness",
  security: "Security",
  policy: "Policy",
  style: "Style",
};
// Findings at the same place share one mark, drawn in the color of the more urgent group.
const urgency: Record<RuleGroup, number> = { security: 0, correctness: 1, policy: 2, style: 3 };

interface Mark {
  line: number;
  column: number;
  length: number;
  group: RuleGroup;
  findings: number[];
}

const marks = computed(() => {
  const byPlace = new Map<string, Mark>();
  home.findings.forEach((f, i) => {
    const key = `${f.line}:${f.column}:${f.length}`;
    const mark = byPlace.get(key);
    if (!mark) {
      byPlace.set(key, { line: f.line, column: f.column, length: f.length, group: f.group, findings: [i] });
    } else {
      mark.findings.push(i);
      if (urgency[f.group] < urgency[mark.group]) mark.group = f.group;
    }
  });
  return [...byPlace.values()];
});

/** The lines that have a finding, with the most urgent group among their findings. */
const flaggedLines = computed(() => {
  const lines = new Map<number, RuleGroup>();
  for (const m of marks.value) {
    const g = lines.get(m.line);
    if (!g || urgency[m.group] < urgency[g]) lines.set(m.line, m.group);
  }
  return [...lines].map(([line, group]) => ({ line, group }));
});

const activeLine = computed(() => (active.value === null ? null : home.findings[active.value].line));
const groupsInDemo = computed(() =>
  (Object.keys(groupNames) as RuleGroup[]).filter((g) => home.findings.some((f) => f.group === g)),
);
const ruleWidth = Math.max(...home.findings.map((f) => f.rule.length));

// A few rules of each group, linked to their entries in the rules reference.
const exampleRules: Record<"correctness" | "security" | "policy", string[]> = {
  correctness: [
    "workflow-syntax",
    "expression-type",
    "undefined-property",
    "unknown-action-input",
    "undefined-job-needs",
    "shellcheck",
  ],
  security: [
    "template-injection",
    "excessive-permissions",
    "dangerous-triggers",
    "cache-poisoning",
    "artipacked",
    "agentic-actions",
  ],
  policy: ["unpinned-uses", "missing-permissions", "missing-timeout", "concurrency-limits"],
};
</script>

<template>
  <div class="jh">
    <section class="jh-hero" aria-labelledby="jh-title">
      <div class="jh-hero-copy">
        <p class="jh-eyebrow"><span class="jh-eyebrow-dot" aria-hidden="true"></span>Static checker for GitHub Actions</p>
        <h1 id="jh-title">Catch broken and insecure workflows <span class="jh-gradient">before they run</span></h1>
        <p class="jh-lede">
          jactionlint checks your workflows, composite actions and <code>dependabot.yml</code>. It type-checks
          expressions, validates the inputs of actions and runs shellcheck on your scripts. It finds script injection,
          risky triggers and tokens with more permissions than they need, and fixes what it can.
        </p>
        <div class="jh-actions">
          <a class="jh-button jh-button-brand" href="/install">Get started <span aria-hidden="true">→</span></a>
          <a class="jh-button" href="/playground">Try the playground</a>
        </div>
        <div id="install" class="jh-install">
          <span class="jh-prompt" aria-hidden="true">$</span>
          <code>{{ installCommand }}</code>
          <button type="button" aria-label="Copy the command that installs jactionlint with mise" @click="copyInstall('hero')">
            {{ copyLabel("hero") }}
          </button>
        </div>
        <p class="jh-install-note" role="status">
          <template v-if="copied === 'hero'">{{ copyNote }}</template>
          <template v-else>
            Install with mise, or with Homebrew, PyPI, Docker or Go.
            <a href="/install#choose-a-method">All methods →</a>
          </template>
        </p>
      </div>

      <div id="example-of-a-broken-workflow" class="jh-window jh-demo">
        <div class="jh-window-bar">
          <span class="jh-dots" aria-hidden="true"><i></i><i></i><i></i></span>
          <span class="jh-window-title">test.yaml</span>
          <div class="jh-tabs" role="tablist" aria-label="What jactionlint reports for test.yaml" @keydown="onTabKey">
            <button
              id="jh-tab-problems"
              type="button"
              role="tab"
              class="jh-tab"
              aria-controls="jh-panel-problems"
              :aria-selected="tab === 'problems'"
              :tabindex="tab === 'problems' ? 0 : -1"
              @click="tab = 'problems'"
            >
              Problems <span class="jh-count">{{ home.findings.length }}</span>
            </button>
            <button
              id="jh-tab-terminal"
              type="button"
              role="tab"
              class="jh-tab"
              aria-controls="jh-panel-terminal"
              :aria-selected="tab === 'terminal'"
              :tabindex="tab === 'terminal' ? 0 : -1"
              @click="tab = 'terminal'"
            >
              Terminal
            </button>
          </div>
        </div>
        <div class="jh-panels">
          <div
            id="jh-panel-problems"
            role="tabpanel"
            aria-labelledby="jh-tab-problems"
            class="jh-problems"
            :class="{ 'is-covered': tab !== 'problems' }"
          >
            <div class="jh-editor">
              <div class="jh-editor-inner">
                <slot name="workflow" />
                <div class="jh-marks" aria-hidden="true">
                  <span v-if="activeLine !== null" class="jh-band" :style="{ '--l': activeLine }"></span>
                  <span
                    v-for="f in flaggedLines"
                    :key="`dot-${f.line}`"
                    class="jh-gutter-dot"
                    :class="`is-${f.group}`"
                    :style="{ '--l': f.line }"
                  ></span>
                  <span
                    v-for="m in marks"
                    :key="`${m.line}:${m.column}`"
                    class="jh-mark"
                    :class="[`is-${m.group}`, { 'is-active': active !== null && m.findings.includes(active) }]"
                    :style="{ '--l': m.line, '--c': m.column, '--n': m.length }"
                    @mouseenter="active = m.findings[0]"
                    @mouseleave="active = null"
                  ></span>
                </div>
              </div>
            </div>
            <ol id="jactionlint-reports-7-errors" class="jh-findings" :style="{ '--jh-rule-width': ruleWidth }">
              <li
                v-for="(f, i) in home.findings"
                :key="i"
                class="jh-finding"
                :class="[`is-${f.group}`, { 'is-active': active === i }]"
                @mouseenter="active = i"
                @mouseleave="active = null"
              >
                <span class="jh-finding-dot" :title="groupNames[f.group]"></span>
                <span class="jh-finding-pos">{{ f.line }}:{{ f.column }}</span>
                <a class="jh-finding-rule" :href="`/rules#${f.rule}`" @focus="active = i" @blur="active = null">{{
                  f.rule
                }}</a>
                <span class="jh-finding-msg" :title="f.message">{{ f.message }}</span>
              </li>
            </ol>
          </div>
          <div
            id="jh-panel-terminal"
            role="tabpanel"
            aria-labelledby="jh-tab-terminal"
            class="jh-terminal"
            :hidden="tab !== 'terminal'"
          >
            <p class="jh-term-command"><span aria-hidden="true">$ </span>jactionlint test.yaml</p>
            <slot name="output" />
          </div>
        </div>
        <div class="jh-window-foot">
          <ul class="jh-legend" aria-label="Rule groups">
            <li v-for="g in groupsInDemo" :key="g" :class="`is-${g}`">{{ groupNames[g] }}</li>
          </ul>
          <a href="/playground">Try it in the playground →</a>
        </div>
      </div>
    </section>

    <section class="jh-stats" aria-label="jactionlint in numbers">
      <ul>
        <li>
          <a href="/rules">
            <span class="jh-stat-value">{{ home.rules.total }}</span>
            <span class="jh-stat-label">rules, each with a stable ID</span>
          </a>
        </li>
        <li>
          <a href="/zizmor-parity">
            <span class="jh-stat-value">{{ home.rules.security }}</span>
            <span class="jh-stat-label">security rules, covering the audits of zizmor</span>
          </a>
        </li>
        <li>
          <a href="/fix">
            <span class="jh-stat-value">{{ home.rules.fixable }}</span>
            <span class="jh-stat-label">rules that <code>--fix</code> can repair</span>
          </a>
        </li>
        <li>
          <a href="/online">
            <span class="jh-stat-value">0</span>
            <span class="jh-stat-label">network requests unless you pass <code>--online</code></span>
          </a>
        </li>
      </ul>
    </section>

    <section class="jh-section" aria-labelledby="jh-checks-title">
      <p class="jh-kicker">What it checks</p>
      <h2 id="jh-checks-title">Bugs, security holes and policy gaps in one pass</h2>
      <p class="jh-section-lede">
        jactionlint started as a fork of actionlint and covers its checks, and its security rules follow the audits of
        zizmor. Every finding names its rule, and <code>jactionlint --explain &lt;rule&gt;</code> tells you why it
        matters and how to fix it.
      </p>
      <div class="jh-cards jh-cards-3">
        <article class="jh-card is-correctness">
          <h3><span class="jh-card-dot" aria-hidden="true"></span>Correctness</h3>
          <p>
            Mistakes that make a run fail or misbehave: unknown keys and events, type errors in
            <code v-pre>${{ }}</code> expressions, wrong inputs of actions and reusable workflows, broken
            <code>needs:</code> and cron, and scripts that shellcheck or pyflakes reject.
          </p>
          <ul class="jh-chips" aria-label="Some correctness rules">
            <li v-for="r in exampleRules.correctness" :key="r">
              <a :href="`/rules#${r}`">{{ r }}</a>
            </li>
          </ul>
          <a class="jh-card-link" href="/checks">Browse the checks →</a>
        </article>
        <article class="jh-card is-security">
          <h3><span class="jh-card-dot" aria-hidden="true"></span>Security</h3>
          <p>
            Patterns that let an outsider run code or steal a token: script injection, excessive permissions, dangerous
            triggers, cache poisoning, credentials left in the checkout and AI agents that anyone can steer.
          </p>
          <ul class="jh-chips" aria-label="Some security rules">
            <li v-for="r in exampleRules.security" :key="r">
              <a :href="`/rules#${r}`">{{ r }}</a>
            </li>
          </ul>
          <a class="jh-card-link" href="/zizmor-parity">Compare with zizmor →</a>
        </article>
        <article class="jh-card is-policy">
          <h3><span class="jh-card-dot" aria-hidden="true"></span>Policy</h3>
          <p>
            Conventions worth enforcing: actions pinned to a commit, explicit permissions, a timeout on every job and a
            concurrency group that cancels superseded runs. Turn each rule up, down or off in the config file.
          </p>
          <ul class="jh-chips" aria-label="Some policy rules">
            <li v-for="r in exampleRules.policy" :key="r">
              <a :href="`/rules#${r}`">{{ r }}</a>
            </li>
          </ul>
          <a class="jh-card-link" href="/rules">See every rule →</a>
        </article>
      </div>
    </section>

    <section class="jh-section jh-split" aria-labelledby="jh-fix-title">
      <div class="jh-split-copy">
        <p class="jh-kicker">Automatic fixes</p>
        <h2 id="jh-fix-title">Fixes, not just findings</h2>
        <p class="jh-section-lede">
          <code>jactionlint --fix</code> repairs the findings that have a mechanical fix, and <code>--diff</code> shows
          the change without writing it. On <code>test.yaml</code> above, it passes the untrusted message through an environment
          variable, gives the token read-only permissions and stops <code>actions/checkout</code> from keeping the
          token.
        </p>
        <ul class="jh-points">
          <li>Safe fixes keep what the workflow does. <code>--fix=unsafe</code> also applies the ones that may change it.</li>
          <li>With the online checks, <code>--fix</code> pins each action to the commit of its tag.</li>
          <li>A finding that a baseline hides is fixed too, which pays the baseline down.</li>
        </ul>
        <a class="jh-more" href="/fix">How fixes work →</a>
      </div>
      <div class="jh-window jh-code-window">
        <div class="jh-window-bar">
          <span class="jh-dots" aria-hidden="true"><i></i><i></i><i></i></span>
          <span class="jh-window-title">Terminal</span>
        </div>
        <div class="jh-code jh-diff">
          <p class="jh-term-command"><span aria-hidden="true">$ </span>jactionlint --diff test.yaml</p>
          <slot name="diff" />
        </div>
      </div>
    </section>

    <section class="jh-section" aria-labelledby="jh-adopt-title">
      <p class="jh-kicker">Adoption</p>
      <h2 id="jh-adopt-title">Turn it on without a cleanup sprint</h2>
      <p class="jh-section-lede">
        The default profile can fail the first run on a repository that never followed it. Start with the bugs, record
        the rest and let the build fail only on what is new.
      </p>
      <ol class="jh-steps">
        <li>
          <code class="jh-step-command">jactionlint --profile correctness</code>
          <h3>Fix the bugs first</h3>
          <p>
            The <code>correctness</code> profile has the checks of actionlint and the bug detectors of jactionlint.
            Its findings are mistakes, not debt.
          </p>
        </li>
        <li>
          <code class="jh-step-command">jactionlint --baseline-write</code>
          <h3>Record the rest</h3>
          <p>The default profile adds the security and policy rules. Write the findings that are left to a baseline.</p>
        </li>
        <li>
          <code class="jh-step-command">jactionlint --baseline</code>
          <h3>Fail only on new findings</h3>
          <p>Pay the baseline down at your own pace. An ignore comment silences a finding that you accept for good.</p>
        </li>
      </ol>
      <p class="jh-links">
        <a href="/config#profiles">Profiles →</a>
        <a href="/baseline">Adopt with a baseline →</a>
        <a href="/ignore">Ignore findings →</a>
      </p>
    </section>

    <section class="jh-section" aria-labelledby="jh-ci-title">
      <p class="jh-kicker">Integrations</p>
      <h2 id="jh-ci-title">In CI, in Git hooks and in your browser</h2>
      <p class="jh-section-lede">
        The jactionlint action checks every push and pull request. Add this workflow to a repository, and pin the action
        to the commit of a release the way the default profile asks of every action.
      </p>
      <div class="jh-ci">
        <div class="jh-window jh-code-window">
          <div class="jh-window-bar">
            <span class="jh-dots" aria-hidden="true"><i></i><i></i><i></i></span>
            <span class="jh-window-title">.github/workflows/jactionlint.yaml</span>
          </div>
          <div class="jh-code">
            <slot name="ci" />
          </div>
          <p class="jh-window-note">This workflow passes the default profile itself.</p>
        </div>
        <ul class="jh-integrations">
          <li>
            <a href="/ci#the-action">
              <strong>The jactionlint action</strong>
              <span>Annotates pull requests with the findings, or uploads them to code scanning.</span>
            </a>
          </li>
          <li>
            <a href="/ci#mise">
              <strong>mise and Docker</strong>
              <span>Pin a verified version with mise, or use the image that ships shellcheck and pyflakes.</span>
            </a>
          </li>
          <li>
            <a href="/hooks">
              <strong>hk and pre-commit</strong>
              <span>Check the workflow files before every commit, and fix them with <code>hk fix</code>.</span>
            </a>
          </li>
          <li>
            <a href="/playground">
              <strong>Playground</strong>
              <span>The real jactionlint, compiled to WebAssembly, runs in your browser.</span>
            </a>
          </li>
        </ul>
      </div>
    </section>

    <section class="jh-section" aria-labelledby="jh-switch-title">
      <p class="jh-kicker">Switching</p>
      <h2 id="jh-switch-title">Coming from actionlint or zizmor?</h2>
      <div class="jh-cards jh-cards-2">
        <article class="jh-card">
          <h3>From actionlint</h3>
          <p>
            jactionlint is a fork of <a href="https://github.com/rhysd/actionlint">rhysd/actionlint</a> and reads its
            config file. <code>--profile correctness</code> keeps the checks that you have, and the default profile
            adds the security and policy rules when you are ready for them.
          </p>
          <a class="jh-card-link" href="/actionlint">Coming from actionlint →</a>
        </article>
        <article class="jh-card">
          <h3>From zizmor</h3>
          <p>
            Each audit of <a href="https://github.com/zizmorcore/zizmor">zizmor</a> 1.30.1 has a rule, under the same
            ID in every case but one. <code># zizmor: ignore[...]</code> comments keep working, and
            <code>--migrate-ignores</code> rewrites them.
          </p>
          <a class="jh-card-link" href="/zizmor-parity">jactionlint and zizmor →</a>
        </article>
      </div>
    </section>

    <section class="jh-cta" aria-labelledby="jh-cta-title">
      <h2 id="jh-cta-title">Run it on your repository</h2>
      <p>One command installs it. Run <code>jactionlint</code> at the root of a repository, with no configuration.</p>
      <div class="jh-install">
        <span class="jh-prompt" aria-hidden="true">$</span>
        <code>{{ installCommand }}</code>
        <button type="button" aria-label="Copy the command that installs jactionlint with mise" @click="copyInstall('cta')">
          {{ copyLabel("cta") }}
        </button>
      </div>
      <p class="jh-install-note" role="status">
        <template v-if="copied === 'cta'">{{ copyNote }}</template>
        <template v-else>
          Install with mise, or with Homebrew, PyPI, Docker or Go.
          <a href="/install#choose-a-method">All methods →</a>
        </template>
      </p>
      <div class="jh-actions">
        <a class="jh-button jh-button-brand" href="/install">Get started <span aria-hidden="true">→</span></a>
        <a class="jh-button" href="/usage">Read the guide</a>
        <a class="jh-button" href="https://github.com/jdx/jactionlint">GitHub</a>
      </div>
    </section>
  </div>
</template>

<style>
.jh {
  --jh-max: 1152px;
  --jh-gutter: 24px;
  --jh-border: var(--vp-c-divider);
  --jh-correctness: #d02f35;
  --jh-security: #b45309;
  --jh-policy: #1f6fd1;

  overflow-x: clip;
}

.dark .jh {
  --jh-correctness: #ff7b7b;
  --jh-security: #f5b544;
  --jh-policy: #6aa8ff;
}

.jh .is-correctness {
  --jh-group: var(--jh-correctness);
}

.jh .is-security {
  --jh-group: var(--jh-security);
}

.jh .is-policy {
  --jh-group: var(--jh-policy);
}

.jh .is-style {
  --jh-group: var(--vp-c-text-3);
}

.jh :not(pre) > code {
  padding: 0.1em 0.35em;
  border-radius: 4px;
  background: var(--vp-code-bg);
  color: var(--vp-code-color);
  font-family: var(--vp-font-family-mono);
  font-size: 0.86em;
  white-space: nowrap;
}

/* Hero */

.jh-hero {
  position: relative;
  isolation: isolate;
  display: grid;
  gap: 48px;
  max-width: var(--jh-max);
  margin: 0 auto;
  padding: 40px var(--jh-gutter) 56px;
}

.jh-hero::before {
  content: "";
  position: absolute;
  z-index: -1;
  inset: -96px -30% auto;
  height: 720px;
  background:
    radial-gradient(42% 46% at 72% 42%, rgba(52, 211, 153, 0.2), transparent 70%),
    radial-gradient(30% 34% at 12% 8%, rgba(15, 118, 110, 0.12), transparent 70%);
  pointer-events: none;
}

.dark .jh-hero::before {
  background:
    radial-gradient(42% 46% at 72% 42%, rgba(52, 211, 153, 0.13), transparent 70%),
    radial-gradient(30% 34% at 12% 8%, rgba(15, 118, 110, 0.2), transparent 70%);
}

@media (min-width: 960px) {
  .jh-hero {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.06fr);
    align-items: center;
    gap: 56px;
    padding: 64px var(--jh-gutter) 80px;
  }
}

.jh-eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  margin: 0 0 22px;
  padding: 5px 14px 5px 10px;
  border: 1px solid var(--jh-border);
  border-radius: 999px;
  background: var(--vp-c-bg);
  color: var(--vp-c-text-2);
  font-size: 13px;
  font-weight: 500;
  line-height: 20px;
}

.jh-eyebrow-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--vp-c-brand-1);
  box-shadow: 0 0 0 4px var(--vp-c-brand-soft);
}

.jh-hero h1 {
  margin: 0;
  color: var(--vp-c-text-1);
  font-size: clamp(38px, 4.9vw, 56px);
  font-weight: 750;
  line-height: 1.04;
  letter-spacing: -0.035em;
  text-wrap: balance;
}

.jh-gradient {
  background: linear-gradient(110deg, #0f766e 0%, #0d9488 45%, #10b981 100%);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}

.dark .jh-gradient {
  background-image: linear-gradient(110deg, #34d399 0%, #6ee7b7 100%);
}

.jh-lede {
  max-width: 34em;
  margin: 22px 0 0;
  color: var(--vp-c-text-2);
  font-size: 18px;
  line-height: 1.65;
}

.jh-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 30px;
}

.jh-button {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 44px;
  padding: 0 20px;
  border: 1px solid var(--jh-border);
  border-radius: 10px;
  background: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-size: 15px;
  font-weight: 600;
  white-space: nowrap;
  transition:
    border-color 0.2s,
    color 0.2s,
    background-color 0.2s;
}

.jh-button:hover {
  border-color: var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
}

.jh-button-brand,
.jh-button-brand:hover {
  border-color: var(--vp-button-brand-bg);
  background: var(--vp-button-brand-bg);
  color: var(--vp-button-brand-text);
}

.jh-button-brand:hover {
  border-color: var(--vp-button-brand-hover-bg);
  background: var(--vp-button-brand-hover-bg);
}

.jh-install {
  display: flex;
  align-items: center;
  gap: 12px;
  max-width: 400px;
  margin-top: 24px;
  padding: 6px 6px 6px 16px;
  border: 1px solid var(--jh-border);
  border-radius: 10px;
  background: var(--vp-c-bg-soft);
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
}

.jh .jh-install code {
  flex: 1;
  min-width: 0;
  padding: 0;
  overflow-x: auto;
  background: none;
  color: var(--vp-c-text-1);
  font-size: inherit;
  white-space: nowrap;
}

.jh-prompt {
  color: var(--vp-c-brand-1);
  user-select: none;
}

.jh-install button {
  flex: none;
  min-width: 64px;
  padding: 6px 12px;
  border: 1px solid var(--jh-border);
  border-radius: 7px;
  background: var(--vp-c-bg);
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-base);
  font-size: 13px;
  font-weight: 500;
  transition:
    border-color 0.2s,
    color 0.2s;
}

.jh-install button:hover {
  border-color: var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
}

.jh-install-note {
  min-height: 20px;
  margin: 10px 0 0;
  color: var(--vp-c-text-3);
  font-size: 13px;
  line-height: 20px;
}

.jh-install-note a {
  color: var(--vp-c-brand-1);
  font-weight: 500;
  white-space: nowrap;
}

/* Windows: an editor or a terminal, dark in both themes */

.jh-window {
  --jh-correctness: #ff7b7b;
  --jh-security: #f5b544;
  --jh-policy: #6aa8ff;
  --jh-win-bg: #0f1816;
  --jh-win-bar: #15211e;
  --jh-win-line: rgba(255, 255, 255, 0.08);
  --jh-win-text: #e3ebe8;
  --jh-win-muted: #8d9c97;

  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
  border: 1px solid #1d2a26;
  border-radius: 14px;
  background: var(--jh-win-bg);
  color: var(--jh-win-text);
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.06) inset,
    0 28px 56px -24px rgba(6, 41, 34, 0.5),
    0 12px 24px -16px rgba(6, 41, 34, 0.35);
}

.dark .jh-window {
  border-color: #23332e;
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.05) inset,
    0 28px 64px -24px rgba(0, 0, 0, 0.8);
}

.jh-window-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 44px;
  padding: 0 8px 0 14px;
  border-bottom: 1px solid var(--jh-win-line);
  background: var(--jh-win-bar);
}

.jh-dots {
  display: flex;
  flex: none;
  gap: 6px;
}

.jh-dots i {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.16);
}

.jh-window-title {
  min-width: 0;
  overflow: hidden;
  color: var(--jh-win-muted);
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.jh-tabs {
  display: flex;
  flex: none;
  gap: 2px;
  margin-left: auto;
  padding: 3px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.05);
}

.jh-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 6px;
  color: var(--jh-win-muted);
  font-size: 12.5px;
  font-weight: 500;
  line-height: 20px;
  transition:
    background-color 0.2s,
    color 0.2s;
}

.jh-tab:hover {
  color: var(--jh-win-text);
}

.jh-tab[aria-selected="true"] {
  background: rgba(255, 255, 255, 0.1);
  color: var(--jh-win-text);
}

.jh-count {
  padding: 0 6px;
  border-radius: 999px;
  background: rgba(255, 123, 123, 0.16);
  color: #ffb1b1;
  font-size: 11px;
  font-weight: 600;
  line-height: 16px;
}

/* Code in a window: the blocks of index.md, colored with the dark theme of Shiki in both themes of the site */

.jh .jh-window .vp-code span {
  color: var(--shiki-dark, inherit);
  font-weight: var(--shiki-dark-font-weight, inherit);
}

.jh-window div[class*="language-"] {
  position: relative;
  margin: 0;
  background: transparent;
}

.jh-window div[class*="language-"] > .lang {
  display: none;
}

.jh-window pre {
  margin: 0;
  padding: 0;
  background: transparent !important;
}

.jh-window code {
  display: block;
  width: max-content;
  min-width: 100%;
  font-family: var(--vp-font-family-mono);
}

.jh-window div[class*="language-"] > button.copy {
  position: absolute;
  top: -6px;
  right: 10px;
  z-index: 2;
  width: 34px;
  height: 34px;
  border: 1px solid var(--jh-win-line);
  border-radius: 6px;
  background: var(--jh-win-bar) var(--vp-icon-copy) 50% / 18px no-repeat;
  opacity: 0;
  cursor: pointer;
  transition: opacity 0.2s;
}

.jh-window div[class*="language-"]:hover > button.copy,
.jh-window div[class*="language-"] > button.copy:focus,
.jh-window div[class*="language-"] > button.copy.copied {
  opacity: 1;
}

.jh-window div[class*="language-"] > button.copy.copied {
  background-image: var(--vp-icon-copied);
}

.jh-term-command {
  margin: 0 0 6px;
  color: var(--jh-win-text);
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  line-height: 1.7;
}

.jh-term-command span {
  color: #34d399;
  user-select: none;
}

/* The demo: the workflow with its findings marked, and the list of findings */

.jh-panels {
  position: relative;
}

.jh-problems.is-covered {
  visibility: hidden;
}

.jh-editor {
  --jh-fs: 13px;
  --jh-lh: 1.75;
  --jh-pad-x: 24px;
  --jh-pad-y: 14px;
  --jh-num: 2ch;
  --jh-num-gap: 18px;

  overflow-x: auto;
  border-bottom: 1px solid var(--jh-win-line);
}

.jh-editor-inner {
  position: relative;
  width: max-content;
  min-width: 100%;
}

.jh-editor div[class*="language-"] > button.copy,
.jh-terminal div[class*="language-"] > button.copy {
  display: none;
}

.jh-editor pre {
  padding: var(--jh-pad-y) 0;
  overflow: visible;
}

.jh-editor code {
  padding: 0 var(--jh-pad-x);
  font-size: var(--jh-fs);
  line-height: var(--jh-lh);
  counter-reset: jh-line;
}

.jh-editor .line::before {
  content: counter(jh-line);
  counter-increment: jh-line;
  display: inline-block;
  width: var(--jh-num);
  margin-right: var(--jh-num-gap);
  color: var(--jh-win-muted);
  opacity: 0.55;
  text-align: right;
}

.jh-marks {
  position: absolute;
  inset: 0;
  font-family: var(--vp-font-family-mono);
  font-size: var(--jh-fs);
  line-height: var(--jh-lh);
  pointer-events: none;
}

.jh-band,
.jh-mark,
.jh-gutter-dot {
  position: absolute;
  top: calc(var(--jh-pad-y) + (var(--l) - 1) * var(--jh-lh) * 1em);
  height: calc(var(--jh-lh) * 1em);
}

.jh-band {
  left: 0;
  right: 0;
  background: rgba(255, 255, 255, 0.05);
}

.jh-gutter-dot {
  left: 10px;
  width: 6px;
  height: 6px;
  margin-top: calc((var(--jh-lh) * 1em - 6px) / 2);
  border-radius: 50%;
  background: var(--jh-group);
}

.jh-mark {
  left: calc(var(--jh-pad-x) + var(--jh-num) + var(--jh-num-gap) + (var(--c) - 1) * 1ch);
  width: calc(var(--n) * 1ch);
  border-radius: 3px;
  pointer-events: auto;
  transition: background-color 0.15s;
}

.jh-mark::after {
  content: "";
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 4px;
  background: var(--jh-group);
  -webkit-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='6' height='4'%3E%3Cpath d='M0 3.2 1.5 1 3 3.2 4.5 1 6 3.2' fill='none' stroke='%23000' stroke-width='1.2'/%3E%3C/svg%3E")
    0 0 / 6px 4px repeat-x;
  mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='6' height='4'%3E%3Cpath d='M0 3.2 1.5 1 3 3.2 4.5 1 6 3.2' fill='none' stroke='%23000' stroke-width='1.2'/%3E%3C/svg%3E")
    0 0 / 6px 4px repeat-x;
}

.jh-mark.is-active {
  background: color-mix(in srgb, var(--jh-group) 22%, transparent);
}

.jh-findings {
  margin: 0;
  padding: 8px 0;
  list-style: none;
  font-size: 13px;
  line-height: 20px;
}

/* The columns are measured in the monospace font of the position and the rule */
.jh-finding {
  display: grid;
  grid-template-columns: 8px 5ch calc(var(--jh-rule-width) * 1ch) minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  padding: 4px 18px;
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  transition: background-color 0.15s;
}

.jh-finding.is-active {
  background: rgba(255, 255, 255, 0.05);
}

.jh-finding-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--jh-group);
}

.jh-finding-pos {
  color: var(--jh-win-muted);
  text-align: right;
}

.jh-finding-rule {
  color: var(--jh-group);
  font-weight: 500;
  white-space: nowrap;
}

.jh-finding-rule:hover {
  text-decoration: underline;
}

.jh-finding-msg {
  overflow: hidden;
  color: var(--jh-win-text);
  font-family: var(--vp-font-family-base);
  font-size: 13px;
  opacity: 0.8;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 639px) {
  .jh-finding {
    grid-template-columns: 8px auto minmax(0, 1fr);
    row-gap: 0;
  }

  .jh-finding-msg {
    grid-column: 2 / -1;
  }
}

.jh-terminal {
  position: absolute;
  inset: 0;
  overflow: auto;
  padding: 14px 18px;
}

.jh-terminal pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.jh-terminal code {
  width: auto;
  font-size: 12.5px;
  line-height: 1.7;
}

.jh-window-foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 16px;
  padding: 10px 18px;
  border-top: 1px solid var(--jh-win-line);
  background: var(--jh-win-bar);
  font-size: 12.5px;
}

.jh-window-foot a {
  color: #6ee7b7;
  font-weight: 500;
}

.jh-window-foot a:hover {
  text-decoration: underline;
}

.jh-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin: 0;
  padding: 0;
  color: var(--jh-win-muted);
  list-style: none;
}

.jh-legend li::before {
  content: "";
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: 7px;
  border-radius: 50%;
  background: var(--jh-group);
}

/* Numbers */

.jh-stats {
  max-width: var(--jh-max);
  margin: 0 auto;
  padding: 0 var(--jh-gutter);
}

.jh-stats ul {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--jh-border);
  border-radius: 14px;
  background: var(--jh-border);
  list-style: none;
}

@media (min-width: 768px) {
  .jh-stats ul {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

.jh-stats li {
  background: var(--vp-c-bg);
}

.jh-stats a {
  display: block;
  height: 100%;
  padding: 20px 22px;
  transition: background-color 0.2s;
}

.jh-stats a:hover {
  background: var(--vp-c-bg-soft);
}

.jh-stat-value {
  display: block;
  color: var(--vp-c-text-1);
  font-size: 34px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1.1;
  letter-spacing: -0.03em;
}

.jh-stats a:hover .jh-stat-value {
  color: var(--vp-c-brand-1);
}

.jh-stat-label {
  display: block;
  margin-top: 6px;
  color: var(--vp-c-text-2);
  font-size: 14px;
  line-height: 1.45;
}

/* Sections */

.jh-section {
  max-width: var(--jh-max);
  margin: 0 auto;
  padding: 88px var(--jh-gutter) 0;
}

.jh-kicker {
  margin: 0 0 12px;
  color: var(--vp-c-brand-1);
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  font-weight: 600;
  letter-spacing: 0.08em;
  line-height: 20px;
  text-transform: uppercase;
}

.jh-section h2,
.jh-cta h2 {
  margin: 0;
  color: var(--vp-c-text-1);
  font-size: clamp(28px, 3.6vw, 40px);
  font-weight: 700;
  line-height: 1.12;
  letter-spacing: -0.028em;
  text-wrap: balance;
}

.jh-section-lede {
  max-width: 44em;
  margin: 16px 0 0;
  color: var(--vp-c-text-2);
  font-size: 17px;
  line-height: 1.65;
}

.jh-section a:not(.jh-button) {
  color: var(--vp-c-brand-1);
}

.jh-section a:not(.jh-button):hover {
  text-decoration: underline;
}

.jh-cards {
  display: grid;
  gap: 16px;
  margin-top: 40px;
}

@media (min-width: 768px) {
  .jh-cards-2 {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 960px) {
  .jh-cards-3 {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

.jh-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 26px;
  border: 1px solid var(--jh-border);
  border-radius: 14px;
  background: var(--vp-c-bg-soft);
}

.jh-card h3 {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0;
  color: var(--vp-c-text-1);
  font-size: 19px;
  font-weight: 650;
  line-height: 1.3;
}

.jh-card-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--jh-group);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--jh-group) 18%, transparent);
}

.jh-card p {
  margin: 0;
  color: var(--vp-c-text-2);
  font-size: 15px;
  line-height: 1.65;
}

.jh-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.jh .jh-chips a {
  display: inline-block;
  padding: 1px 8px;
  border: 1px solid color-mix(in srgb, var(--jh-group) 28%, transparent);
  border-radius: 6px;
  background: color-mix(in srgb, var(--jh-group) 9%, transparent);
  color: var(--jh-group);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  font-weight: 500;
  line-height: 22px;
}

.jh .jh-chips a:hover {
  border-color: var(--jh-group);
  text-decoration: none;
}

.jh-card-link,
.jh-more {
  margin-top: auto;
  font-size: 14.5px;
  font-weight: 600;
}

.jh-more {
  display: inline-block;
  margin-top: 24px;
}

/* A text and a window side by side */

.jh-split {
  display: grid;
  gap: 40px;
  align-items: center;
}

@media (min-width: 960px) {
  .jh-split {
    grid-template-columns: minmax(0, 0.85fr) minmax(0, 1.15fr);
    gap: 56px;
  }

}

.jh-points {
  margin: 20px 0 0;
  padding: 0;
  list-style: none;
}

.jh-points li {
  position: relative;
  margin-top: 10px;
  padding-left: 26px;
  color: var(--vp-c-text-2);
  font-size: 15px;
  line-height: 1.6;
}

.jh-points li::before {
  content: "";
  position: absolute;
  top: 0.45em;
  left: 2px;
  width: 12px;
  height: 7px;
  border-bottom: 2px solid var(--vp-c-brand-1);
  border-left: 2px solid var(--vp-c-brand-1);
  transform: rotate(-45deg);
}

.jh-code {
  padding: 16px 0 18px;
}

.jh-code > .jh-term-command {
  padding: 0 18px;
}

.jh-code pre {
  padding: 0 18px;
  overflow-x: auto;
}

.jh-code code {
  font-size: 12.5px;
  line-height: 1.7;
}

.jh-diff .line {
  display: inline-block;
  box-sizing: border-box;
  min-width: calc(100% + 36px);
  margin: 0 -18px;
  padding: 0 18px;
}

.jh-diff .line.diff-line-add {
  background: rgba(46, 160, 67, 0.16);
}

.jh-diff .line.diff-line-remove {
  background: rgba(248, 81, 73, 0.15);
}

.jh-window-note {
  margin: 0;
  padding: 10px 18px;
  border-top: 1px solid var(--jh-win-line);
  background: var(--jh-win-bar);
  color: var(--jh-win-muted);
  font-size: 12.5px;
}

/* Adoption steps */

.jh-steps {
  display: grid;
  gap: 16px;
  margin: 40px 0 0;
  padding: 0;
  list-style: none;
  counter-reset: jh-step;
}

@media (min-width: 960px) {
  .jh-steps {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

.jh-steps li {
  position: relative;
  padding: 24px 26px 26px;
  border: 1px solid var(--jh-border);
  border-radius: 14px;
  background: var(--vp-c-bg-soft);
  counter-increment: jh-step;
}

.jh-steps li::before {
  content: counter(jh-step);
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  margin-bottom: 16px;
  border-radius: 50%;
  background: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-size: 14px;
  font-weight: 700;
}

.jh .jh-step-command {
  display: block;
  padding: 8px 12px;
  overflow-x: auto;
  border: 1px solid var(--jh-border);
  border-radius: 8px;
  background: var(--vp-c-bg);
  font-size: 13px;
  white-space: nowrap;
}

.jh-steps h3 {
  margin: 18px 0 8px;
  color: var(--vp-c-text-1);
  font-size: 17px;
  font-weight: 650;
  line-height: 1.35;
}

.jh-steps p {
  margin: 0;
  color: var(--vp-c-text-2);
  font-size: 15px;
  line-height: 1.6;
}

.jh-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 28px;
  margin: 28px 0 0;
  font-size: 14.5px;
  font-weight: 600;
}

/* Integrations: the CI workflow and the other ways to run jactionlint */

.jh-ci {
  display: grid;
  gap: 32px;
  align-items: start;
  margin-top: 40px;
}

@media (min-width: 960px) {
  .jh-ci {
    grid-template-columns: minmax(0, 1.7fr) minmax(0, 1fr);
    gap: 48px;
  }
}

.jh-integrations {
  margin: 0;
  padding: 0;
  list-style: none;
}

.jh-integrations li + li {
  border-top: 1px solid var(--jh-border);
}

.jh .jh-integrations a {
  display: block;
  padding: 16px 0;
  color: inherit;
}

.jh-integrations li:first-child a {
  padding-top: 4px;
}

.jh .jh-integrations a:hover {
  text-decoration: none;
}

.jh-integrations strong {
  display: block;
  color: var(--vp-c-text-1);
  font-size: 16px;
  font-weight: 600;
  line-height: 1.4;
  transition: color 0.2s;
}

.jh-integrations strong::after {
  content: " →";
  color: var(--vp-c-brand-1);
  opacity: 0;
  transition: opacity 0.2s;
}

.jh-integrations a:hover strong {
  color: var(--vp-c-brand-1);
}

.jh-integrations a:hover strong::after {
  opacity: 1;
}

.jh-integrations span {
  display: block;
  margin-top: 4px;
  color: var(--vp-c-text-2);
  font-size: 15px;
  line-height: 1.55;
}

/* Closing call to action */

.jh-cta {
  position: relative;
  isolation: isolate;
  max-width: calc(var(--jh-max) - 2 * var(--jh-gutter));
  margin: 96px auto;
  padding: 56px 16px;
  overflow: hidden;
  border: 1px solid var(--jh-border);
  border-radius: 20px;
  background: var(--vp-c-bg-soft);
  text-align: center;
}

@media (max-width: 1199px) {
  .jh-cta {
    margin: 96px var(--jh-gutter);
  }
}

.jh-cta::before {
  content: "";
  position: absolute;
  z-index: -1;
  inset: 0;
  background: radial-gradient(60% 80% at 50% 0%, rgba(52, 211, 153, 0.16), transparent 70%);
}

.jh-cta > p {
  max-width: 36em;
  margin: 14px auto 0;
  color: var(--vp-c-text-2);
  font-size: 17px;
  line-height: 1.6;
}

.jh-cta .jh-install {
  margin: 28px auto 0;
  text-align: left;
}

@media (max-width: 419px) {
  .jh-install {
    gap: 10px;
    padding-left: 12px;
    font-size: 13px;
  }
}

.jh-cta .jh-install-note {
  margin: 10px auto 0;
}

.jh-cta .jh-actions {
  justify-content: center;
}
</style>
