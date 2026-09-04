import { expect, test } from "@playwright/test";

// The theme and language controls sit above every screen a person can reach
// without a session, so what goes wrong here goes wrong on the first thing they
// see. Both defects below shipped: the menu was pinned to both edges of its
// trigger, which sized it to whichever value happened to be selected and left a
// longer option wrapping onto a second line, and selection was drawn with the
// same background hover uses, so a pointer resting on a neighbour made two rows
// look chosen.
test("the theme menu is at least as wide as its trigger and marks the chosen row", async ({ page }) => {
  await page.goto("/?state=expired");

  const trigger = page.locator('[data-utility-control] [data-slot="select-trigger"]').first();
  await expect(trigger).toBeVisible();
  const triggerBox = await trigger.boundingBox();
  if (!triggerBox) {
    throw new Error("the theme control did not lay out");
  }

  await trigger.click();
  const menu = page.locator('[data-slot="select-content"]');
  await expect(menu).toBeVisible();
  const menuBox = await menu.boundingBox();
  if (!menuBox) {
    throw new Error("the open menu did not lay out");
  }
  expect(menuBox.width).toBeGreaterThanOrEqual(triggerBox.width - 1);

  const options = menu.locator('[data-slot="option"]');
  await expect(options).toHaveCount(3);
  for (const option of await options.all()) {
    // A range over the label reports one rectangle per visual line, which is the
    // only reading that survives a row taller than its own text: the option
    // carries a touch-target minimum height, so dividing the box by the line
    // height counts two lines for text that never wrapped.
    const lines = await option.evaluate((element) => {
      const label = element.lastChild;
      if (!label) {
        return 0;
      }
      const range = document.createRange();
      range.selectNodeContents(label);
      return range.getClientRects().length;
    });
    expect(lines).toBe(1);
  }

  // Weight, not background: hover paints the same background, so a pointer
  // resting on a neighbour used to make two rows look chosen.
  const weights = await options.evaluateAll((elements) =>
    elements.map((element) => ({
      selected: element.getAttribute("aria-selected") === "true",
      weight: Number.parseInt(window.getComputedStyle(element).fontWeight, 10)
    }))
  );
  const chosen = weights.filter((row) => row.selected);
  expect(chosen).toHaveLength(1);
  for (const row of weights.filter((entry) => !entry.selected)) {
    expect(chosen[0].weight).toBeGreaterThan(row.weight);
  }
});

// The glyph says which theme is chosen without reading the label, and it has to
// follow the value: a control that keeps the sun after the reader picks dark is
// worse than one with no glyph at all.
test("each utility control carries a glyph, and the theme glyph follows the value", async ({ page }) => {
  await page.goto("/?state=expired");

  const glyphs = page.locator('[data-utility-control] [data-slot="select-trigger"] [data-icon]');
  await expect(glyphs).toHaveCount(2);
  await expect(glyphs.nth(0)).toHaveAttribute("data-icon-name", "monitor");
  await expect(glyphs.nth(1)).toHaveAttribute("data-icon-name", "languages");

  const trigger = page.locator('[data-utility-control] [data-slot="select-trigger"]').first();
  await trigger.click();
  await page.locator('[data-slot="option"][data-value="dark"]').click();
  await expect(glyphs.nth(0)).toHaveAttribute("data-icon-name", "moon");
});

// These controls sit in the top corner, so a menu that grows from its trigger's
// start edge grows off the screen. English is the case that shows it: the widest
// option, "Chinese, Simplified", is several times the trigger's width.
test("an open menu in the widest locale stays inside the viewport", async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 470, height: 330 }, locale: "en-US" });
  const page = await context.newPage();
  await page.goto("/?state=expired");

  await page.locator('[data-utility-control] [data-slot="select-trigger"]').nth(1).click();
  const menu = page.locator('[data-slot="select-content"]');
  await expect(menu).toBeVisible();

  const box = await menu.boundingBox();
  if (!box) {
    throw new Error("the open menu did not lay out");
  }
  const viewport = page.viewportSize();
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual((viewport?.width ?? 0) + 1);
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  ).toBe(0);

  await context.close();
});
