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
      { text: "Rules", link: "/rules" },
      { text: "Go API", link: "/api" },
      { text: "References", link: "/reference" },
    ],
  },
  {
    text: "v2 roadmap",
    items: [
      { text: "v2 migration (planned)", link: "/v2-migration" },
      { text: "zizmor parity", link: "/zizmor-parity" },
    ],
  },
  {
    text: "Playground",
    items: [{ text: "Online playground", link: "/playground" }],
  },
];
