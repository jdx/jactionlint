import { defineConfig } from "vitepress";
import { generateLlms } from "./llms.ts";
import { sidebar } from "./sidebar.ts";

const siteUrl = "https://jactionlint.jdx.dev";
const siteDescription =
  "jactionlint is a static checker for GitHub Actions workflow files: syntax, type-checked expressions, action inputs, reusable workflows, shellcheck and security checks.";
const socialTitle = "jactionlint — static checker for GitHub Actions workflow files";

export default defineConfig({
  title: "jactionlint",
  description: siteDescription,
  cleanUrls: true,
  lastUpdated: true,
  sitemap: { hostname: siteUrl },
  // docs/README.md is the index page shown on GitHub; the site uses index.md.
  srcExclude: ["README.md"],

  markdown: {
    config(md) {
      // The docs talk about GitHub's ${{ }} expression syntax in plain text.
      // Keep Vue from treating it as an interpolation.
      const escape = (s: string) =>
        md.utils.escapeHtml(s).replace(/\{\{/g, "&#123;&#123;");
      md.renderer.rules.text = (tokens, idx) => escape(tokens[idx].content);
      md.renderer.rules.code_inline = (tokens, idx, _options, _env, self) =>
        `<code${self.renderAttrs(tokens[idx])}>${escape(tokens[idx].content)}</code>`;
    },
  },

  head: [
    [
      "script",
      {},
      `(function () {
  try {
    var d = document.documentElement;
    var c = JSON.parse(localStorage.getItem("jdx-banner-cache") || "null");
    var expires = c && c.expires ? Date.parse(c.expires) : NaN;
    var now = Date.now();
    var metadataValid =
      c &&
      typeof c.id === "string" &&
      typeof c.height === "string" &&
      /^[1-9]\\d*(?:\\.\\d+)?px$/.test(c.height) &&
      Number.isFinite(c.width) &&
      typeof c.fontSize === "string" &&
      Number.isFinite(c.pixelRatio) &&
      Number.isFinite(c.cachedAt) &&
      c.cachedAt <= now &&
      now - c.cachedAt < 300000 &&
      (!c.expires || (typeof c.expires === "string" && Number.isFinite(expires) && now < expires));
    var contextMatches =
      metadataValid &&
      c.width === innerWidth &&
      c.fontSize === getComputedStyle(d).fontSize &&
      c.pixelRatio === devicePixelRatio;
    if (contextMatches && localStorage.getItem("jdx-banner-dismissed") !== c.id)
      d.style.setProperty("--vp-layout-top-height", c.height);
    else if (c && !metadataValid)
      localStorage.removeItem("jdx-banner-cache");
  } catch (e) {}
})();`,
    ],
    [
      "script",
      {},
      // The playground used to live at the site root; permalinks (docs/checks.md has many) look like
      // /#<state> and the old ?s=<source> / ?u=<url> parameters were also supported. Keep them working.
      `(function () {
  var p = location.pathname;
  if (p !== "/" && p !== "/index.html") return;
  if (location.hash.length > 1 || /[?&][su]=/.test(location.search)) {
    location.replace("/playground" + location.search + location.hash);
  }
})();`,
    ],
    ["link", { rel: "icon", href: "/favicon.ico", sizes: "48x48" }],
    ["link", { rel: "icon", href: "/favicon.svg", type: "image/svg+xml" }],
    ["link", { rel: "apple-touch-icon", href: "/apple-touch-icon.png" }],
    ["link", { rel: "manifest", href: "/site.webmanifest" }],
    ["meta", { name: "theme-color", content: "#0f766e" }],
    ["meta", { property: "og:title", content: socialTitle }],
    ["meta", { property: "og:description", content: siteDescription }],
    ["meta", { property: "og:type", content: "website" }],
    ["meta", { property: "og:site_name", content: "jactionlint" }],
    ["meta", { property: "og:locale", content: "en_US" }],
    ["meta", { property: "og:image", content: `${siteUrl}/og.png` }],
    ["meta", { property: "og:image:width", content: "1200" }],
    ["meta", { property: "og:image:height", content: "630" }],
    ["meta", { property: "og:image:alt", content: socialTitle }],
    ["meta", { name: "twitter:card", content: "summary_large_image" }],
    ["meta", { name: "twitter:site", content: "@jdxcode" }],
    ["meta", { name: "twitter:image", content: `${siteUrl}/og.png` }],
    ["meta", { name: "twitter:image:alt", content: socialTitle }],
  ],

  buildEnd({ srcDir, outDir }) {
    generateLlms(srcDir, outDir);
  },

  transformHead({ pageData, title, description }) {
    const url = new URL(
      pageData.relativePath.replace(/index\.md$/, "").replace(/\.md$/, ""),
      `${siteUrl}/`,
    ).toString();

    return [
      ["link", { rel: "canonical", href: url }],
      ["meta", { property: "og:url", content: url }],
      ["meta", { property: "og:title", content: title }],
      ["meta", { property: "og:description", content: description }],
      ["meta", { name: "twitter:title", content: title }],
      ["meta", { name: "twitter:description", content: description }],
      [
        "script",
        { type: "application/ld+json" },
        JSON.stringify({
          "@context": "https://schema.org",
          "@type": "WebPage",
          name: title,
          description,
          url,
          isPartOf: { "@type": "WebSite", name: "jactionlint", url: siteUrl },
        }),
      ],
    ];
  },

  themeConfig: {
    logo: "/logo.svg",

    nav: [
      { text: "Guide", link: "/install" },
      { text: "Checks", link: "/checks" },
      { text: "Go API", link: "/api" },
      { text: "Playground", link: "/playground" },
    ],

    sidebar,

    socialLinks: [
      { icon: "github", link: "https://github.com/jdx/jactionlint" },
    ],

    editLink: {
      pattern: "https://github.com/jdx/jactionlint/edit/main/docs/:path",
      text: "Edit this page on GitHub",
    },

    search: {
      provider: "local",
    },

    footer: false,
  },
});
