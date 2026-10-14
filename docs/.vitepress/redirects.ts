// Old "page#id" locations that moved when the usage and zizmor pages were split into several pages.
//
// Anchors cannot be redirected on the server (the fragment is never sent), so the site redirects in the
// browser: config.mts injects this map into a head script for the first load, and theme/index.ts applies it
// to navigation inside the site. Released binaries and external sites link to these locations, so an entry is
// never removed. A key must be a "/page#id" that existed before the split; a value is the "/page#id" (or
// "/page") where the section lives now. scripts/verify-docs.mjs checks that every target exists.
export const redirects: Record<string, string> = {
  "/usage#action": "/ci#action",
  "/usage#adopt-a-stricter-configuration-with-a-baseline": "/baseline#adopt-a-stricter-configuration-with-a-baseline",
  "/usage#baseline": "/baseline#baseline",
  "/usage#command-line-options": "/cli#command-line-options",
  "/usage#docker": "/ci#docker",
  "/usage#docker-image": "/ci#docker-image",
  "/usage#emacs": "/hooks#editors-and-other-tools",
  "/usage#events-and-token-permissions-of-a-finding": "/output#events-and-token-permissions-of-a-finding",
  "/usage#example-error-annotation-on-github-actions": "/output#example-error-annotation-on-github-actions",
  "/usage#example-markdown": "/output#example-markdown",
  "/usage#example-sarif-format": "/output#example-sarif-format",
  "/usage#example-serialized-in-json-lines": "/output#example-serialized-in-json-lines",
  "/usage#example-serialized-into-json": "/output#example-serialized-into-json",
  "/usage#fix-errors-automatically": "/fix#fix-errors-automatically",
  "/usage#format": "/output#format",
  "/usage#format-error-messages": "/output#format-error-messages",
  "/usage#formatting-syntax": "/output#formatting-syntax",
  "/usage#github-event": "/checks#check-contextual-github-event",
  "/usage#hk": "/hooks#hk",
  "/usage#how-fixing-converges-and-what-it-checks": "/fix#how-fixing-converges-and-what-it-checks",
  "/usage#ignore-some-errors": "/ignore#ignore-some-errors",
  "/usage#json-schema-of-the-output": "/output#json-schema-of-the-output",
  "/usage#lines-and-columns": "/output#lines-and-columns",
  "/usage#megalinter": "/hooks#editors-and-other-tools",
  "/usage#mise": "/ci#mise",
  "/usage#nova": "/hooks#editors-and-other-tools",
  "/usage#on-github-actions": "/ci#on-github-actions",
  "/usage#online-checks": "/online#online-checks",
  "/usage#online-playground": "/playground",
  "/usage#pin-container-images-to-digests-with-online-fix-unsafe": "/online#pin-container-images-to-digests-with-online-fix-unsafe",
  "/usage#pin-tags-to-commits-with-online-fix": "/online#pin-tags-to-commits-with-online-fix",
  "/usage#pre-commit": "/hooks#pre-commit",
  "/usage#problem-matchers": "/ci#problem-matchers",
  "/usage#pulsar-edit": "/hooks#editors-and-other-tools",
  "/usage#reviewdog": "/hooks#editors-and-other-tools",
  "/usage#super-linter": "/hooks#editors-and-other-tools",
  "/usage#the-action": "/ci#the-action",
  "/usage#tools-integ": "/hooks#editors-and-other-tools",
  "/usage#tools-integration": "/hooks#editors-and-other-tools",
  "/usage#trunk": "/hooks#editors-and-other-tools",
  "/usage#typos-in-github-event": "/checks#check-contextual-github-event",
  "/usage#use-jactionlint-on-github-actions": "/ci#use-jactionlint-on-github-actions",
  "/usage#using-jactionlint-from-go-program": "/api#go-api",
  "/usage#vim-and-neovim": "/hooks#editors-and-other-tools",
  "/usage#vs-code": "/hooks#editors-and-other-tools",
  "/usage#zizmor-ignore-comments": "/ignore#zizmor-ignore-comments",
  "/zizmor-parity#cache-poisoning-against-zizmor": "/zizmor-measurements#cache-poisoning-against-zizmor",
  "/zizmor-parity#decisions-kept": "/zizmor-measurements#decisions-kept",
  "/zizmor-parity#differential-harness": "/zizmor-measurements#differential-harness",
  "/zizmor-parity#effect-on-the-findings-that-were-reported-before": "/zizmor-measurements#effect-on-the-findings-that-were-reported-before",
  "/zizmor-parity#excessive-permissions": "/zizmor-measurements#excessive-permissions",
  "/zizmor-parity#false-positive-review": "/zizmor-measurements#false-positive-review",
  "/zizmor-parity#findings-only-jactionlint-reports": "/zizmor-measurements#findings-only-jactionlint-reports",
  "/zizmor-parity#lessons-applied": "/zizmor-measurements#lessons-applied",
  "/zizmor-parity#permissions-against-zizmor": "/zizmor-measurements#permissions-against-zizmor",
  "/zizmor-parity#undocumented-permissions": "/zizmor-measurements#undocumented-permissions",
};

/** The redirect target for a location like "/usage#hk" (a trailing slash or ".html" is ignored), or undefined. */
export function redirectFor(pathname: string, hash: string): string | undefined {
  const path = pathname.replace(/\.html$/, "").replace(/\/$/, "") || "/";
  return redirects[path + hash];
}
