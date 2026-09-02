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


test("theme preference persists after a reload", async ({ page }) => {
  const requests = await mockConsoleSession(page);
  await resetStoredPreferences(page);

  await page.goto("/preferences");
  await waitForPreferences(page);
  const controls = await preferenceControls(page);

  await selectAppOption(controls.theme, "dark");
  await page.waitForFunction(() => {
    const root = document.documentElement;
    return root.dataset.themePreference === "dark" && root.dataset.theme === "dark";
  });
  await expect(controls.theme).toHaveAttribute("data-value", "dark");
  await expect(
    page.locator("[data-header-controls] [data-utility-controls] [data-slot=\"select-trigger\"]").first()
  ).toHaveAttribute("data-value", "dark");

  await page.reload();
  await waitForPreferences(page);
  const reloadedControls = await preferenceControls(page);
  await expect(reloadedControls.theme).toHaveAttribute("data-value", "dark");
  await page.waitForFunction(() => {
    const root = document.documentElement;
    return root.dataset.themePreference === "dark" && root.dataset.theme === "dark";
  });
  expect(requests).toEqual(expectedShellRequests);
});

test("language preference persists after a reload", async ({ page }) => {
  const requests = await mockConsoleSession(page);
  await resetStoredPreferences(page);

  await page.goto("/preferences");
  await waitForPreferences(page);
  const controls = await preferenceControls(page);

  await selectAppOption(controls.locale, "en");
  await expect(page.locator("[data-preferences-page] h1")).toHaveText("Preferences");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");

  await page.reload();
  await waitForPreferences(page);
  const reloadedControls = await preferenceControls(page);
  await expect(page.locator("[data-preferences-page] h1")).toHaveText("Preferences");
  await expect(reloadedControls.locale).toHaveAttribute("data-value", "en");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  expect(requests).toEqual(expectedShellRequests);
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
