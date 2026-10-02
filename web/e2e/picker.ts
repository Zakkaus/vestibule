import { readFileSync } from "node:fs";
import { expect, type Locator, type Page } from "@playwright/test";

const catalogues = Object.fromEntries(["en", "zh-CN", "zh-TW", "ja", "ru"].map((locale) => [
  locale,
  JSON.parse(readFileSync(new URL(`../src/i18n/locales/${locale}.json`, import.meta.url), "utf8"))
]));

export async function pickerMessage(page: Page, key: string): Promise<string> {
  const locale = await page.locator("html").getAttribute("lang");
  return key.split(".").reduce((value, segment) => value[segment], catalogues[locale ?? ""] ?? catalogues["zh-CN"]);
}

export async function selectPickerOption(trigger: Locator, name: string): Promise<void> {
  await trigger.click();
  await trigger.page().getByRole("listbox").getByRole("option", { name, exact: true }).click();
  await expect(trigger.page().getByRole("listbox")).toHaveCount(0);
}

export async function expectPickerSelection(trigger: Locator, name: string): Promise<void> {
  await trigger.click();
  const option = trigger.page().getByRole("listbox").getByRole("option", { name, exact: true });
  await expect(option).toHaveAttribute("aria-selected", "true");
  await option.press("Escape");
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expect(trigger.page().getByRole("listbox")).toHaveCount(0);
}

export async function themeOption(trigger: Locator, value: string): Promise<string> {
  return pickerMessage(trigger.page(), `theme.${value}`);
}

export async function localeOption(trigger: Locator, value: string): Promise<string> {
  const keys: Record<string, string> = { "zh-CN": "zhCN", "zh-TW": "zhTW" };
  return pickerMessage(trigger.page(), `locale.${keys[value] ?? value}`);
}
