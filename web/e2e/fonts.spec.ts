import { expect, test } from "@playwright/test";
import { mockSpectrumTransport, openSpectrumRoute } from "./spectrum-fixtures";

for (const locale of ["zh-CN", "zh-TW", "en"] as const) {
  for (const width of [390, 1280]) {
    test(`${locale} shares its loaded sans face across controls at ${width}px`, async ({ page }) => {
      const family = locale === "en" ? "system-ui" : locale === "zh-CN" ? "Noto Sans SC" : "Noto Sans TC";
      await page.setViewportSize({ width, height: 1000 });
      await page.addInitScript((value) => localStorage.setItem("verify-console-locale", value), locale);
      await mockSpectrumTransport(page);
      await openSpectrumRoute(page, "/moderation");
      await expect(page.locator("[data-moderation-page]")).toHaveAttribute("data-moderation-state", "loaded");
      await page.locator('[data-moderation-field="warnLimit"] input').fill("4");
      await expect(page.locator("[data-moderation-savebar]")).toBeVisible();
      await expect(page.locator("html")).toHaveAttribute("lang", locale);
      const picker = page.locator("[data-group-switcher] button");
      const value = page.locator('[data-group-switcher] [data-slot="label"]');
      await expect(picker).toBeEnabled();
      await expect(picker).not.toHaveAttribute("aria-disabled", "true");
      await expect(value).toHaveText("Gentoo-zh Community");
      await picker.evaluate(async (node) => {
        await Promise.all(node.getAnimations({ subtree: true }).map((animation) => animation.finished));
      });

      const selectors = [
        '[data-group-switcher] [data-slot="label"]',
        '[data-moderation-page] p',
        '[data-moderation-field="warnLimit"] input',
        '[data-moderation-field="antispamEnabled"] button',
        '[data-moderation-savebar] button'
      ];
      for (const selector of selectors) {
        const element = page.locator(selector).first();
        await expect(element).toBeVisible();
        const stack = await element.evaluate((node) => getComputedStyle(node).fontFamily);
        expect(stack.split(",")[0]!.replace(/["']/g, "").trim(), selector).toBe(family);
        expect(stack, selector).toBe(await page.locator("body").evaluate((node) => getComputedStyle(node).fontFamily));
      }

      if (locale !== "en") {
        await expect.poll(() => page.evaluate(async (face) => {
          await document.fonts.ready;
          return {
            loaded: [...document.fonts].some((font) => font.family === face && font.status === "loaded"),
            chinese: document.fonts.check(`15px "${face}"`, document.querySelector("[data-moderation-page] p")!.textContent!)
          };
        }, family)).toEqual({ loaded: true, chinese: true });
      }

      const widths = await value.evaluate(async (node, face) => {
        await document.fonts.ready;
        const style = getComputedStyle(node);
        const reference = document.createElement("span");
        reference.textContent = "Gentoo-zh Community";
        reference.style.cssText = "position:fixed;visibility:hidden;white-space:pre";
        reference.style.font = style.font;
        reference.style.fontFamily = `"${face}", system-ui, sans-serif`;
        reference.style.letterSpacing = style.letterSpacing;
        document.body.append(reference);
        const range = document.createRange();
        range.selectNodeContents(node);
        const actual = range.getBoundingClientRect().width;
        const sans = reference.getBoundingClientRect().width;
        const canvas = document.createElement("canvas");
        const context = canvas.getContext("2d")!;
        context.font = getComputedStyle(reference).font;
        const measuredSans = context.measureText(reference.textContent).width;
        reference.remove();
        return { actual, sans, measuredSans };
      }, family);
      expect(Math.abs(widths.actual - widths.sans), "Picker Latin glyphs match a known sans span").toBeLessThanOrEqual(1);
      expect(Math.abs(widths.sans - widths.measuredSans), "sans span matches canvas metrics").toBeLessThanOrEqual(1);
    });
  }
}
