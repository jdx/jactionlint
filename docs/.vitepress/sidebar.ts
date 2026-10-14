import type { DefaultTheme } from "vitepress";

export type SidebarItem = DefaultTheme.SidebarItem;

export const sidebar: SidebarItem[] = [
  {
    text: "Get started",
    items: [
      { text: "Installation", link: "/install" },
      { text: "Run jactionlint", link: "/usage" },
    ],
  },
  {
    text: "Guides",
    items: [
      { text: "Ignore findings", link: "/ignore" },
      { text: "Adopt with a baseline", link: "/baseline" },
      { text: "Fix findings", link: "/fix" },
      { text: "Run in CI", link: "/ci" },
      { text: "Git hooks and editors", link: "/hooks" },
      { text: "Online checks", link: "/online" },
    ],
  },
  {
    text: "Reference",
    items: [
      { text: "Configuration", link: "/config" },
      { text: "Command line", link: "/cli" },
      { text: "Output formats", link: "/output" },
      { text: "Checks", link: "/checks" },
      { text: "Rules", link: "/rules" },
      { text: "Go API", link: "/api" },
      { text: "References", link: "/reference" },
    ],
  },
  {
    text: "Moving to jactionlint",
    items: [
      { text: "Coming from actionlint", link: "/actionlint" },
      { text: "Migrating to v2", link: "/v2-migration" },
      { text: "jactionlint and zizmor", link: "/zizmor-parity" },
      { text: "zizmor comparison details", link: "/zizmor-measurements" },
    ],
  },
  {
    text: "Playground",
    items: [{ text: "Online playground", link: "/playground" }],
  },
];
