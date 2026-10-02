import { expect, type Locator, type Page } from "@playwright/test";

async function appOption(trigger: Locator, value: string): Promise<Locator> {
  await expect(trigger).toHaveAttribute("aria-controls", /\S+/);
  const listboxId = await trigger.getAttribute("aria-controls");
  const page = trigger.page();
  return page.locator('[role="listbox"], [role="menu"]')
    .and(page.locator(`[id=${JSON.stringify(listboxId)}]`))
    .locator('[role="option"], [role="menuitemradio"], [role="menuitemcheckbox"], [role="menuitem"]')
    .and(page.locator(`[data-key=${JSON.stringify(value)}], [data-value=${JSON.stringify(value)}]`));
}

export async function selectAppOption(trigger: Locator, value: string): Promise<void> {
  if (await trigger.evaluate((element) => element instanceof HTMLSelectElement)) {
    await trigger.selectOption(value);
    return;
  }

  await trigger.click();
  const option = await appOption(trigger, value);

  await expect(option).toHaveCount(1);
  await option.click();
}

export async function expectAppSelection(trigger: Locator, value: string): Promise<void> {
  if (await trigger.evaluate((element) => element instanceof HTMLSelectElement)) {
    await expect(trigger).toHaveValue(value);
    return;
  }

  await trigger.click();
  const option = await appOption(trigger, value);
  const selectedAttribute = await option.getAttribute("role") === "option" ? "aria-selected" : "aria-checked";
  await expect(option).toHaveAttribute(selectedAttribute, "true");
  await option.press("Escape");
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
}

export async function selectConsolePreference(page: Page, preference: "theme" | "locale", value: string): Promise<void> {
  const desktop = page.locator(`[data-console-utilities="desktop"] [data-preference="${preference}"]`);
  if (await desktop.isVisible()) await selectAppOption(desktop, value);
  else {
    await page.locator('[data-console-utilities="mobile"] button').click();
    await page.locator(`[role="menu"] [data-preference="${preference}"]`).hover();
    await page.locator(`[role="menu"] [data-key="${value}"]`).click();
  }
  await page.locator('[role="menu"]').first().waitFor({ state: "detached" });
}
