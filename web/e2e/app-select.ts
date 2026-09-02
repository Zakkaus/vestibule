import { expect, type Locator } from "@playwright/test";

export async function selectAppOption(trigger: Locator, value: string): Promise<void> {
  await trigger.click();
  const option = trigger
    .locator("xpath=..")
    .locator(`[data-slot="option"][data-value="${value}"]`);

  await expect(option).toHaveCount(1);
  await option.click();
}
