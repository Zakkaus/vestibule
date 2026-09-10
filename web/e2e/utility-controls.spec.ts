import { expect, test } from "@playwright/test";

test("the theme menu is at least as wide as its trigger and marks the chosen row", async ({ page }) => {
  await page.goto("/?state=expired");

  const trigger = page.locator('[data-utility-control] button').first();
  await expect(trigger).toBeVisible();
  const triggerBox = await trigger.boundingBox();
  if (!triggerBox) {
    throw new Error("the theme control did not lay out");
  }

  await trigger.click();
  const menu = page.getByRole("listbox");
  await expect(menu).toBeVisible();
  const menuBox = await menu.boundingBox();
  if (!menuBox) {
    throw new Error("the open menu did not lay out");
  }
  expect(menuBox.width).toBeGreaterThanOrEqual(triggerBox.width - 1);

  const options = menu.getByRole("option");
  await expect(options).toHaveCount(3);
  const chosen = options.filter({ has: page.locator("svg") }).and(
    menu.locator('[aria-selected="true"]')
  );
  await expect(chosen).toHaveCount(1);
  await expect(menu.getByRole("option", { name: "浅色", exact: true })).not.toHaveAttribute(
    "aria-selected", "true"
  );
});

// The glyph says which theme is chosen without reading the label, and it has to
// follow the value: a control that keeps the sun after the reader picks dark is
// worse than one with no glyph at all.
test("each utility control carries a glyph, and the theme glyph follows the value", async ({ page }) => {
  await page.goto("/?state=expired");

  const glyphs = page.locator('[data-utility-control] button [data-icon]');
  await expect(glyphs).toHaveCount(2);
  await expect(glyphs.nth(0)).toHaveAttribute("data-icon-name", "monitor");
  await expect(glyphs.nth(1)).toHaveAttribute("data-icon-name", "languages");

  const trigger = page.locator('[data-utility-control] button').first();
  await trigger.click();
  await page.getByRole("option", { name: "深色", exact: true }).click();
  await expect(glyphs.nth(0)).toHaveAttribute("data-icon-name", "moon");
});

// These controls sit in the top corner, so a menu that grows from its trigger's
// start edge grows off the screen. English is the case that shows it: the widest
// option, "Chinese, Simplified", is several times the trigger's width.
test("an open menu in the widest locale stays inside the viewport", async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 470, height: 330 }, locale: "en-US" });
  const page = await context.newPage();
  await page.goto("/?state=expired");

  await page.locator('[data-utility-control] button').nth(1).click();
  const menu = page.getByRole("listbox");
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
