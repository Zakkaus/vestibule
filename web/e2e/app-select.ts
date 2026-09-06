import { expect, type Locator } from "@playwright/test";

export async function selectAppOption(trigger: Locator, value: string): Promise<void> {
  await trigger.click();
  const library = await trigger.getAttribute("data-control");
  const listboxId = await trigger.getAttribute("aria-controls");
  const option = library
    ? trigger.page().locator(`[id="${listboxId}"] [role="option"][value="${value}"]`)
    : trigger.locator("xpath=..").locator(`[data-slot="option"][data-value="${value}"]`);

  await expect(option).toHaveCount(1);
  await option.click();
}
