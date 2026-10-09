import type { DefaultTheme } from "vitepress";

export type SidebarItem = DefaultTheme.SidebarItem;

export const sidebar: SidebarItem[] = [
  {
    text: "Guide",
    items: [
      { text: "Installation", link: "/install" },
      { text: "Usage", link: "/usage" },
      { text: "Configuration", link: "/config" },
      { text: "Coming from actionlint", link: "/actionlint" },
    ],
  },
  {
    text: "Reference",
    items: [
      { text: "Checks", link: "/checks" },
      { text: "Rules", link: "/rules" },
      { text: "Go API", link: "/api" },
      { text: "References", link: "/reference" },
    ],
  },
  {
    text: "Moving to jactionlint",
    items: [
      { text: "Migrating to v2", link: "/v2-migration" },
      { text: "jactionlint and zizmor", link: "/zizmor-parity" },
    ],
  },
  {
    text: "Playground",
    items: [{ text: "Online playground", link: "/playground" }],
  },
];
