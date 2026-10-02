import { expect, test } from "@playwright/test";
import { mockSpectrumTransport, selectedGroupID } from "./spectrum-fixtures";

for (const locale of ["zh-CN", "ru"]) {
  test.describe(`shared save footer in ${locale}`, () => {
    test.use({ locale, viewport: { width: 390, height: 844 }, hasTouch: true });
    test("phone actions retain intrinsic widths and complete labels", async ({ page }) => {
      await mockSpectrumTransport(page);
      await page.goto(`/verification?group=${selectedGroupID}`);
      await page.locator("#verification-timeout-seconds").fill("360");
      const footer = page.locator("[data-verification-savebar]");
      await footer.scrollIntoViewIfNeeded();
      const geometry = await footer.getByRole("button").evaluateAll(buttons => buttons.map(button => {
        const bounds = button.getBoundingClientRect();
        const group = button.parentElement!.getBoundingClientRect();
        const walker = document.createTreeWalker(button, NodeFilter.SHOW_TEXT);
        const lines = new Set<number>();
        let node: Node | null;
        while ((node = walker.nextNode())) {
          if (!node.textContent?.trim()) continue;
          const range = document.createRange();
          range.selectNodeContents(node);
          for (const rect of range.getClientRects()) if (rect.width > 0) lines.add(Math.round(rect.top));
        }
        return { x: bounds.x, right: bounds.right, top: bounds.top, bottom: bounds.bottom,
          width: bounds.width, groupWidth: group.width, lines: lines.size };
      }));
      expect(geometry).toHaveLength(2);
      for (const button of geometry) {
        expect(button.lines).toBe(1);
        expect(button.width).toBeLessThanOrEqual(button.groupWidth);
        expect(button.x).toBeGreaterThanOrEqual(0);
        expect(button.right).toBeLessThanOrEqual(390);
      }
      expect(geometry[1]!.top).toBeGreaterThanOrEqual(geometry[0]!.top);
    });
  });
}
