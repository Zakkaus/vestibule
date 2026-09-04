import { expect, test } from "@playwright/test";

import { selectAppOption } from "./app-select";

const localeStorageKey = "verify-console-locale";

async function localeControl(page: import("@playwright/test").Page) {
  const controls = page.locator("[data-utility-controls]").first();
  await expect(controls).toBeVisible();
  return controls.locator('[data-slot="select-trigger"]').nth(1);
}

// The theme control has always offered "follow the system". The language control
// offered three languages and nothing else, so picking one was a one-way door:
// a reader who tried the others out of curiosity had no way back to the language
// their browser asks for.
test.describe("a chosen language can be handed back to the browser", () => {
  test.use({ locale: "zh-TW" });

  test("choosing the browser again clears the stored choice", async ({ page }) => {
    await page.goto("/?state=expired");

    const control = await localeControl(page);
    await selectAppOption(control, "en");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    expect(await page.evaluate((key) => localStorage.getItem(key), localeStorageKey)).toBe("en");

    await selectAppOption(control, "system");
    await expect(page.locator("html")).toHaveAttribute("lang", "zh-TW");
    expect(await page.evaluate((key) => localStorage.getItem(key), localeStorageKey)).toBeNull();
    await expect(control).toHaveAttribute("data-value", "system");
  });
});

// The browser sends an ordered list, and reading only its first entry threw away
// every fallback the reader had ranked after it.
test.describe("the browser's second language still counts", () => {
  test.use({ locale: "ja-JP" });

  test("an unsupported first language falls to the next one the browser asks for", async ({
    page
  }) => {
    await page.addInitScript(() => {
      Object.defineProperty(navigator, "languages", {
        configurable: true,
        get: () => ["ja-JP", "zh-TW", "en-US"]
      });
    });
    await page.goto("/?state=expired");

    await expect(page.locator("html")).toHaveAttribute("lang", "zh-TW");
  });
});

// Hong Kong and Macau read traditional characters, and were being answered in
// simplified because only the exact zh-TW tag and the zh-Hant prefix matched.
test.describe("traditional-reading regions get traditional characters", () => {
  test.use({ locale: "zh-HK" });

  test("a Hong Kong browser is answered in Traditional Chinese", async ({ page }) => {
    await page.goto("/?state=expired");

    await expect(page.locator("html")).toHaveAttribute("lang", "zh-TW");
  });
});
