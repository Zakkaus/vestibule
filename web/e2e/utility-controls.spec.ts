import { expect, test } from "@playwright/test";
import { selectAppOption } from "./app-select";

test("the theme menu fits its trigger and identifies the selected preference", async ({ page }) => {
  await page.goto("/?state=expired");
  const trigger = page.locator('[data-control="theme"]');
  await trigger.click();
  const menu = page.getByRole("listbox", { name: "主题" });
  await expect(menu).toBeVisible();
  const triggerBox = await trigger.boundingBox();
  const menuBox = await menu.locator("xpath=..").boundingBox();
  if (!triggerBox || !menuBox) throw new Error("Theme control or popup has no geometry");
  expect(menuBox.width).toBeGreaterThanOrEqual(triggerBox.width - 1);
  const chosen = menu.getByRole("option", { selected: true });
  await expect(chosen).toHaveText("跟随系统");
  await expect(chosen.locator('[data-icon-name="circleCheck"]')).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("the theme glyph follows the selected preference", async ({ page }) => {
  await page.goto("/?state=expired");
  const controls = page.locator('[data-library-utilities]');
  await expect(controls.locator('[data-icon-name="monitor"]')).toBeVisible();
  await expect(controls.locator('[data-icon-name="languages"]')).toBeVisible();
  await selectAppOption(controls.locator('[data-control="theme"]'), "dark");
  await expect(controls.locator('[data-icon-name="moon"]')).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-mantine-color-scheme", "dark");
});

test("an open menu in the widest locale stays inside the viewport", async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 470, height: 330 }, locale: "en-US" });
  const page = await context.newPage();
  await page.goto("/?state=expired");
  await page.locator('[data-control="locale"]').click();
  const menu = page.getByRole("listbox", { name: "Language" });
  await expect(menu).toBeVisible();
  const box = await menu.locator("xpath=..").boundingBox();
  if (!box) throw new Error("Language popup has no geometry");
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(471);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBe(0);
  await context.close();
});
