import DefaultTheme from "vitepress/theme";
import type { Theme } from "vitepress";
import { h, onMounted, onUnmounted } from "vue";
import { initBanner } from "./banner";
import EndevFooter from "./EndevFooter.vue";
import EndevSponsors from "./EndevSponsors.vue";
import Playground from "./playground/Playground.vue";
import { data as starsData } from "../stars.data";
import "./custom.css";

export default {
  extends: DefaultTheme,
  Layout() {
    return h(DefaultTheme.Layout, null, {
      "layout-bottom": () => [h(EndevSponsors), h(EndevFooter)],
    });
  },
  enhanceApp({ app }) {
    initBanner();
    app.component("Playground", Playground);
  },
  setup() {
    // Add the GitHub star count to the nav bar link. The nav is rendered by the default theme,
    // so patch it in after mount (and again if the nav re-renders, e.g. on the mobile menu).
    let observer: MutationObserver | undefined;
    onMounted(() => {
      const addStarCount = () => {
        if (!starsData.stars) return false;
        const links = document.querySelectorAll('.VPSocialLinks a[href*="github.com/jdx/jactionlint"]');
        links.forEach((link) => {
          if (link.querySelector(".star-count")) return;
          const badge = document.createElement("span");
          badge.className = "star-count";
          badge.title = "GitHub Stars";
          const glyph = document.createElement("span");
          glyph.className = "star-glyph";
          glyph.textContent = "★";
          glyph.setAttribute("aria-hidden", "true");
          badge.append(glyph, starsData.stars);
          link.appendChild(badge);
        });
        return links.length > 0 && Array.from(links).every((l) => l.querySelector(".star-count"));
      };

      if (addStarCount()) return;
      observer = new MutationObserver(() => {
        if (addStarCount()) observer?.disconnect();
      });
      observer.observe(document.querySelector(".VPNav") || document.body, { childList: true, subtree: true });
    });
    onUnmounted(() => observer?.disconnect());
  },
} satisfies Theme;
