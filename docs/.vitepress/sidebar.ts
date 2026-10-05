import type { DefaultTheme } from "vitepress";

export type SidebarItem = DefaultTheme.SidebarItem;

export const sidebar: SidebarItem[] = [
  {
    text: "Guide",
    items: [
      { text: "Installation", link: "/install" },
      { text: "Usage", link: "/usage" },
      { text: "Configuration", link: "/config" },
    ],
  },
  {
    text: "Reference",
    items: [
      { text: "Checks", link: "/checks" },
      { text: "Go API", link: "/api" },
      { text: "References", link: "/reference" },
    ],
  },
  {
    text: "Playground",
    items: [{ text: "Online playground", link: "/playground" }],
  },
];
