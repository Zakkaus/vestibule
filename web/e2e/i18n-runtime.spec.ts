import { expect, test } from "@playwright/test";
import type { Locator, Page, TestInfo } from "@playwright/test";

import { selectAppOption } from "./app-select";

const selectedGroupID = "-1009123456789";
const localeStorageKey = "verify-console-locale";

type PreferenceControls = Readonly<{
  locale: Locator;
}>;

function consoleURL(testInfo: TestInfo, path: string): string {
  const baseURL = testInfo.project.use.baseURL;
  if (typeof baseURL !== "string") {
    throw new Error("The console journey needs a configured base URL");
  }
  return new URL(path, baseURL).toString();
}

async function mockConsoleSession(page: Page): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/session" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: { telegram_id: "741928306", role: "manager" },
          expires_at: "2026-09-03T02:00:00Z",
          is_owner: false,
          csrf_token: "locale-runtime-csrf"
        })
      });
      return;
    }

    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: selectedGroupID, title: "Gentoo-zh Community", owner: null, administrators: [], administrators_status: "unavailable" }] })
      });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function waitForPreferences(page: Page): Promise<void> {
  await expect(page.locator("[data-preferences-page]")).toBeVisible();
  await expect(page.locator("[data-group-switcher]").getByRole("button")).toBeEnabled();
}

async function preferenceControls(page: Page): Promise<PreferenceControls> {
  const controls = page.locator("[data-preference-local] [data-utility-controls]");
  await expect(controls).toHaveCount(1);
  const triggers = controls.locator("[data-slot=\"select-trigger\"]");
  await expect(triggers).toHaveCount(2);
  return { locale: triggers.nth(1) };
}

// The control reports the preference, which is "system" until somebody chooses a
// language, the same way the theme control reports "follow the system". The
// catalogue that is actually active is on <html lang> and in the rendered text.
async function expectActiveLocale(page: Page, locale: string, preference = locale): Promise<void> {
  const controls = await preferenceControls(page);
  await expect(page.locator("html"), "the console would select the wrong catalogue").toHaveAttribute(
    "lang",
    locale
  );
  await expect(controls.locale, "the application would activate a different catalogue than the document language").toHaveAttribute(
    "data-value",
    preference
  );
}

test("browser and stored language choices select an available console catalogue", async ({ browser }, testInfo) => {
  const cases = [
    { browserLocale: "zh-TW", storedLocale: null, want: "zh-TW" },
    { browserLocale: "zh-Hant-HK", storedLocale: null, want: "zh-TW" },
    { browserLocale: "en-US", storedLocale: null, want: "en" },
    { browserLocale: "ja-JP", storedLocale: null, want: "ja" },
    { browserLocale: "ru-RU", storedLocale: null, want: "ru" },
    { browserLocale: "en-US", storedLocale: "zh-CN", want: "zh-CN" },
    { browserLocale: "en-US", storedLocale: "zh-TW", want: "zh-TW" },
    { browserLocale: "zh-TW", storedLocale: "en", want: "en" }
  ] as const;
  // Each case opens its own browser context; on the CI runner that is several seconds apiece.
  testInfo.setTimeout(cases.length * 15_000);

  for (const localeCase of cases) {
    await test.step(`${localeCase.browserLocale} with ${localeCase.storedLocale ?? "no stored preference"}`, async () => {
      const context = await browser.newContext({ locale: localeCase.browserLocale });
      try {
        const page = await context.newPage();
        await page.addInitScript(({ key, storedLocale }) => {
          if (storedLocale === null) {
            localStorage.removeItem(key);
            return;
          }
          localStorage.setItem(key, storedLocale);
        }, { key: localeStorageKey, storedLocale: localeCase.storedLocale });
        await mockConsoleSession(page);
        await page.goto(consoleURL(testInfo, "/preferences"));
        await waitForPreferences(page);
        await expectActiveLocale(page, localeCase.want, localeCase.storedLocale ?? "system");
      } finally {
        await context.close();
      }
    });
  }
});

test("Japanese and Russian preferences set the first document language", async ({ browser }, testInfo) => {
  const cases = [
    { browserLocale: "ja-JP", storedLocale: null, want: "ja" },
    { browserLocale: "ru-RU", storedLocale: null, want: "ru" },
    { browserLocale: "en-US", storedLocale: "ja", want: "ja" },
    { browserLocale: "en-US", storedLocale: "ru", want: "ru" }
  ] as const;

  for (const localeCase of cases) {
    await test.step(`${localeCase.browserLocale}/${localeCase.storedLocale ?? "system"}`, async () => {
      const context = await browser.newContext({ locale: localeCase.browserLocale });
      try {
        const page = await context.newPage();
        await page.addInitScript(({ key, value }) => {
          if (value !== null) {
            localStorage.setItem(key, value);
          }
        }, { key: localeStorageKey, value: localeCase.storedLocale });
        // Stop React after the document's inline boot script has run; the DOM
        // language at DOMContentLoaded is the value available to first paint.
        await page.route("**/src/main.tsx", (route) =>
          route.fulfill({ status: 200, contentType: "text/javascript", body: "" })
        );
        await page.goto(consoleURL(testInfo, "/preferences"));
        await page.waitForLoadState("domcontentloaded");
        await expect(
          page.locator("html"),
          "the inline boot must apply the locale preference before React mounts"
        ).toHaveAttribute("lang", localeCase.want);
      } finally {
        await context.close();
      }
    });
  }
});

test("Japanese and Russian choices survive a page refresh", async ({ page }) => {
  await mockConsoleSession(page);
  await page.goto("/preferences");
  await waitForPreferences(page);

  for (const locale of ["ja", "ru"] as const) {
    await test.step(locale, async () => {
      const controls = await preferenceControls(page);
      await selectAppOption(controls.locale, locale);
      await expectActiveLocale(page, locale);
      await page.reload();
      await waitForPreferences(page);
      await expectActiveLocale(page, locale);
      await expect(page.evaluate((key) => localStorage.getItem(key), localeStorageKey)).resolves.toBe(locale);
    });
  }
});

test("i18next selects the Japanese and Russian cardinal categories", async ({ page }) => {
  await mockConsoleSession(page);
  await page.goto("/preferences");
  await waitForPreferences(page);

  const pluralResults = await page.evaluate(async () => {
    // The browser singleton must be exercised inside the page, not imported by the test runner.
    const runtime = await import("/src/i18n/index.ts");
    const counts = [1, 2, 5, 21, 1.5];
    const results: Record<string, Array<{
      count: number;
      category: string;
      text: string;
      expected: string;
      usedLng: string;
    }>> = {};

    for (const locale of ["ja", "ru"]) {
      await runtime.default.changeLanguage(locale);
      const rule = runtime.default.services.pluralResolver.getRule(locale);
      results[locale] = counts.map((count) => {
        const category = rule.select(count);
        const key = "questions.questionBank.count";
        const details = runtime.default.t(key, { count, returnDetails: true });
        const expected = runtime.default.t(`${key}_${category}`, { count, returnDetails: true });
        return {
          count,
          category,
          text: details.res,
          expected: expected.res,
          usedLng: details.usedLng
        };
      });
    }
    return results;
  });

  expect(pluralResults.ja.map((entry) => entry.category)).toEqual(["other", "other", "other", "other", "other"]);
  expect(pluralResults.ru.map((entry) => entry.category)).toEqual(["one", "few", "many", "one", "other"]);
  for (const locale of ["ja", "ru"] as const) {
    for (const result of pluralResults[locale]) {
      expect(result.usedLng, `${locale} plural lookup fell back to another locale`).toBe(locale);
      expect(result.text, `${locale} count ${result.count} did not resolve a catalogue key`).toBe(result.expected);
      expect(result.text).toContain(String(result.count));
      expect(result.text).not.toContain("questions.questionBank.count");
    }
  }
});

test("every offered console language switches and persists", async ({ page }) => {
  await mockConsoleSession(page);
  await page.goto("/preferences");
  await waitForPreferences(page);

  for (const locale of ["zh-TW", "en", "zh-CN", "ja", "ru"] as const) {
    await test.step(locale, async () => {
      const controls = await preferenceControls(page);
      await selectAppOption(controls.locale, locale);
      await expectActiveLocale(page, locale);
      await expect(page.evaluate((key) => localStorage.getItem(key), localeStorageKey)).resolves.toBe(locale);
    });
  }
});

test("unsupported document language starts the default console catalogue", async ({ page }, testInfo) => {
  await page.route((url) => url.pathname === "/preferences", async (route) => {
    await route.fulfill({
      contentType: "text/html",
      body: "<!doctype html><html lang=\"fr-CA\"><head><meta charset=\"UTF-8\" /></head><body><div id=\"root\"></div><script type=\"module\" src=\"/src/main.tsx\"></script></body></html>"
    });
  });
  await mockConsoleSession(page);

  await page.goto(consoleURL(testInfo, "/preferences"));
  await waitForPreferences(page);
  const controls = await preferenceControls(page);

  await expect(page.locator("html")).toHaveAttribute("lang", "fr-CA");
  await expect(controls.locale, "an unsupported document language would start the wrong console catalogue").toHaveAttribute(
    "data-value",
    "system"
  );
});

test("language switching keeps working when locale storage rejects persistence", async ({ page }) => {
  await page.addInitScript((key) => {
    const setItem = Storage.prototype.setItem;
    Storage.prototype.setItem = function(name: string, value: string): void {
      if (this === localStorage && name === key) {
        throw new DOMException("locale storage is unavailable", "SecurityError");
      }
      setItem.call(this, name, value);
    };
  }, localeStorageKey);
  await mockConsoleSession(page);

  await page.goto("/preferences");
  await waitForPreferences(page);
  const controls = await preferenceControls(page);
  await selectAppOption(controls.locale, "en");

  await expectActiveLocale(page, "en", "system");
  await expect(page.evaluate((key) => localStorage.getItem(key), localeStorageKey)).resolves.toBeNull();
});

test("missing active-locale text falls back to the default catalogue", async ({ page }) => {
  await mockConsoleSession(page);
  await page.goto("/preferences");
  await waitForPreferences(page);

  const translated = await page.evaluate(async () => {
    // The browser singleton starts with the page, so Node cannot statically import this module.
    const locale = await import("/src/i18n/index.ts");
    const key = "runtimeFallbackProbe";
    locale.default.addResource(locale.DEFAULT_LOCALE, "translation", key, "from default catalogue");
    await locale.default.changeLanguage("en");
    return locale.default.t(key);
  });

  expect(translated, "a missing active-locale key would render raw text instead of the default translation").toBe(
    "from default catalogue"
  );
});
