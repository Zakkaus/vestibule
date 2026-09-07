import { expect, type Locator } from "@playwright/test";

async function appOption(trigger: Locator, value: string): Promise<Locator> {
  await expect(trigger).toHaveAttribute("aria-controls", /\S+/);
  const listboxId = await trigger.getAttribute("aria-controls");
  const page = trigger.page();
  return page.getByRole("listbox")
    .and(page.locator(`[id=${JSON.stringify(listboxId)}]`))
    .getByRole("option")
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
  await expect(option).toHaveAttribute("aria-selected", "true");
  await option.press("Escape");
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
}
