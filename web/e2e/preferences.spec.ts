import { expect, test, type Locator, type Page } from "@playwright/test";
import { selectAppOption } from "./app-select";

const selectedGroupId = "-1001163306055";
const resetMarker = "preferences-test-reset";
const expectedShellRequests = [
  "GET /api/session",
  "GET /api/chats",
  "GET /api/session",
  "GET /api/chats"
];

type PreferenceControls = Readonly<{
  theme: Locator;
  locale: Locator;
}>;

async function mockConsoleSession(page: Page): Promise<string[]> {
  const requests: string[] = [];

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    requests.push(`${request.method()} ${path}`);

    if (path === "/api/session" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: { telegram_id: "741928306", role: "manager" },
          expires_at: "2026-09-01T02:00:00Z",
          csrf_token: "manager-csrf"
        })
      });
      return;
    }

    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: selectedGroupId }] })
      });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  return requests;
}

async function resetStoredPreferences(page: Page): Promise<void> {
  await page.addInitScript(({ marker }) => {
    if (sessionStorage.getItem(marker)) {
      return;
    }

    localStorage.removeItem("verify-console-theme");
    localStorage.removeItem("verify-console-locale");
    sessionStorage.setItem(marker, "done");
  }, { marker: resetMarker });
}

type BootstrapThemeState = Readonly<{
  theme: string | null;
  preference: string | null;
}>;

async function pauseAppEntry(page: Page): Promise<{
  waitForRequest: () => Promise<void>;
  resume: () => void;
}> {
  let markRequest!: () => void;
  const requested = new Promise<void>((resolve) => {
    markRequest = resolve;
  });
  let releaseEntry!: () => void;
  const entryReleased = new Promise<void>((resolve) => {
    releaseEntry = resolve;
  });

  await page.route("**/src/main.tsx", async (route) => {
    markRequest();
    await entryReleased;
    await route.continue();
  });

  return {
    waitForRequest: () => requested,
    resume: releaseEntry
  };
}

async function bootstrapThemeState(page: Page): Promise<BootstrapThemeState> {
  return page.evaluate(() => ({
    theme: document.documentElement.getAttribute("data-theme"),
    preference: document.documentElement.getAttribute("data-theme-preference")
  }));
}

async function expectThemePreferenceAfterReload(
  page: Page,
  preference: "light" | "dark"
): Promise<void> {
  const requests = await mockConsoleSession(page);
  await resetStoredPreferences(page);

  const initialEntry = await pauseAppEntry(page);
  const initialNavigation = page.goto("/preferences", { waitUntil: "commit" });
  await initialEntry.waitForRequest();
  expect(await bootstrapThemeState(page)).toEqual({ theme: null, preference: "system" });
  await page.unroute("**/src/main.tsx");
  initialEntry.resume();
  await initialNavigation;

  await waitForPreferences(page);
  const controls = await preferenceControls(page);
  expect(await page.evaluate(() => localStorage.getItem("verify-console-theme"))).toBeNull();
  await expect(controls.theme).toHaveAttribute("data-value", "system");

  await selectAppOption(controls.theme, preference);
  await page.waitForFunction((expectedPreference) => {
    const root = document.documentElement;
    return (
      root.dataset.themePreference === expectedPreference &&
      root.dataset.theme === expectedPreference
    );
  }, preference);
  await expect(controls.theme).toHaveAttribute("data-value", preference);
  await expect(
    page.locator("[data-header-controls] [data-utility-controls] [data-slot=\"select-trigger\"]").first()
  ).toHaveAttribute("data-value", preference);
  expect(await page.evaluate(() => localStorage.getItem("verify-console-theme"))).toBe(preference);

  const reloadedEntry = await pauseAppEntry(page);
  const reloadedNavigation = page.reload({ waitUntil: "commit" });
  await reloadedEntry.waitForRequest();
  expect(await bootstrapThemeState(page)).toEqual({
    theme: preference,
    preference
  });
  await page.unroute("**/src/main.tsx");
  reloadedEntry.resume();
  await reloadedNavigation;

  await waitForPreferences(page);
  const reloadedControls = await preferenceControls(page);
  await expect(reloadedControls.theme).toHaveAttribute("data-value", preference);
  await page.waitForFunction((expectedPreference) => {
    const root = document.documentElement;
    return (
      root.dataset.themePreference === expectedPreference &&
      root.dataset.theme === expectedPreference
    );
  }, preference);
  expect(requests).toEqual(expectedShellRequests);
}

async function waitForPreferences(page: Page): Promise<void> {
  await expect(page.locator("[data-preferences-page]")).toBeVisible();
  await expect(page.locator("[data-group-switcher] [data-slot=\"select-trigger\"]")).not.toHaveAttribute(
    "aria-busy",
    "true"
  );
}

async function preferenceControls(page: Page): Promise<PreferenceControls> {
  const controls = page.locator("[data-preference-local] [data-utility-controls]");
  await expect(controls).toHaveCount(1);

  const triggers = controls.locator("[data-slot=\"select-trigger\"]");
  await expect(triggers).toHaveCount(2);

  return {
    theme: triggers.nth(0),
    locale: triggers.nth(1)
  };
}


test("an explicitly selected light theme persists after a real reload", async ({ page }) => {
  await expectThemePreferenceAfterReload(page, "light");
});

test("an explicitly selected dark theme persists after a real reload", async ({ page }) => {
  await expectThemePreferenceAfterReload(page, "dark");
});

test("an explicitly selected app locale persists after a real reload", async ({ page }) => {
  const requests = await mockConsoleSession(page);
  await resetStoredPreferences(page);

  await page.goto("/preferences");
  await waitForPreferences(page);
  const controls = await preferenceControls(page);
  const browserLanguage = await page.evaluate(() => navigator.language);
  expect(browserLanguage.startsWith("zh")).toBe(true);
  expect(await page.evaluate(() => localStorage.getItem("verify-console-locale"))).toBeNull();
  await expect(controls.locale).toHaveAttribute("data-value", "zh-CN");
  await expect(page.locator("html")).toHaveAttribute("lang", "zh-CN");

  await selectAppOption(controls.locale, "en");
  await expect(page.locator("[data-preferences-page] h1")).toHaveText("Preferences");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  expect(await page.evaluate(() => localStorage.getItem("verify-console-locale"))).toBe("en");

  await page.reload();
  await waitForPreferences(page);
  const reloadedControls = await preferenceControls(page);
  await expect(page.locator("[data-preferences-page] h1")).toHaveText("Preferences");
  await expect(reloadedControls.locale).toHaveAttribute("data-value", "en");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  expect(await page.evaluate(() => localStorage.getItem("verify-console-locale"))).toBe("en");
  expect(requests).toEqual(expectedShellRequests);
});

test.describe("browser-language locale selection", () => {
  test.use({ locale: "en-US" });

  test("the first visit uses the browser language when no locale is stored", async ({ page }) => {
    const requests = await mockConsoleSession(page);
    await resetStoredPreferences(page);

    await page.goto("/preferences");
    const browserLanguage = await page.evaluate(() => navigator.language);
    expect(browserLanguage.startsWith("en")).toBe(true);
    expect(await page.evaluate(() => localStorage.getItem("verify-console-locale"))).toBeNull();
    await waitForPreferences(page);
    const controls = await preferenceControls(page);

    await expect(page.locator("[data-preferences-page] h1")).toHaveText("Preferences");
    await expect(controls.locale).toHaveAttribute("data-value", "en");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    expect(await page.evaluate(() => localStorage.getItem("verify-console-locale"))).toBeNull();
    expect(requests).toEqual(expectedShellRequests.slice(0, 2));
  });
});

test.describe("Traditional Chinese browser-language locale selection", () => {
  test.use({ locale: "zh-TW" });

  test("the first visit uses Traditional Chinese when the browser reports zh-TW", async ({
    page
  }) => {
    const requests = await mockConsoleSession(page);
    await resetStoredPreferences(page);

    await page.goto("/preferences");
    expect(await page.evaluate(() => navigator.language)).toBe("zh-TW");
    await waitForPreferences(page);
    const controls = await preferenceControls(page);

    await expect(controls.locale).toHaveAttribute("data-value", "zh-TW");
    await expect(page.locator("html")).toHaveAttribute("lang", "zh-TW");
    expect(await page.evaluate(() => localStorage.getItem("verify-console-locale"))).toBeNull();
    expect(requests).toEqual(expectedShellRequests.slice(0, 2));
  });
});

test("theme preference listbox supports keyboard selection and dismissal", async ({ page }) => {
  await mockConsoleSession(page);
  await resetStoredPreferences(page);
  await page.goto("/preferences");
  await waitForPreferences(page);
  const { theme } = await preferenceControls(page);

  await theme.focus();
  await page.keyboard.press("Enter");
  await expect(theme).toHaveAttribute("aria-expanded", "true");
  await expect(theme.locator("xpath=..").locator('[role="option"][data-value="system"]')).toBeFocused();

  await page.keyboard.press("ArrowDown");
  await expect(theme.locator("xpath=..").locator('[role="option"][data-value="light"]')).toBeFocused();
  await page.keyboard.press("End");
  await expect(theme.locator("xpath=..").locator('[role="option"][data-value="dark"]')).toBeFocused();
  await page.keyboard.press("Enter");

  await expect(theme).toHaveAttribute("aria-expanded", "false");
  await expect(theme).toHaveAttribute("data-value", "dark");
  await expect(theme).toBeFocused();

  await page.keyboard.press(" ");
  await expect(theme).toHaveAttribute("aria-expanded", "true");
  await page.keyboard.press("Escape");
  await expect(theme).toHaveAttribute("aria-expanded", "false");
  await expect(theme).toBeFocused();

  // Home and End are listed twice in this component: once for the closed trigger
  // and once for a focused option. Disabling the trigger pair left the whole
  // suite green, because every existing press happened with an option focused.
  const options = theme.locator("xpath=..");
  await page.keyboard.press("Home");
  await expect(theme).toHaveAttribute("aria-expanded", "true");
  await expect(options.locator('[role="option"][data-value="system"]')).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(theme).toHaveAttribute("aria-expanded", "false");
  await expect(theme).toBeFocused();

  await page.keyboard.press("End");
  await expect(theme).toHaveAttribute("aria-expanded", "true");
  await expect(options.locator('[role="option"][data-value="dark"]')).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(theme).toHaveAttribute("aria-expanded", "false");
});
