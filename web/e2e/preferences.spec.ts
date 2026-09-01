import { expect, test, type Locator, type Page } from "@playwright/test";

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
  await expect(page.locator("[data-group-switcher] select")).not.toHaveAttribute(
    "aria-busy",
    "true"
  );
}

async function preferenceControls(page: Page): Promise<PreferenceControls> {
  const controls = page.locator("[data-preference-local] [data-utility-controls]");
  await expect(controls).toHaveCount(1);

  const selects = controls.locator("select");
  await expect(selects).toHaveCount(2);

  return {
    theme: selects.nth(0),
    locale: selects.nth(1)
  };
}


test("theme preference persists after a reload", async ({ page }) => {
  const requests = await mockConsoleSession(page);
  await resetStoredPreferences(page);

  await page.goto("/preferences");
  await waitForPreferences(page);
  const controls = await preferenceControls(page);

  await controls.theme.selectOption("dark");
  await page.waitForFunction(() => {
    const root = document.documentElement;
    return root.dataset.themePreference === "dark" && root.dataset.theme === "dark";
  });
  await expect(controls.theme).toHaveValue("dark");
  await expect(
    page.locator("[data-header-controls] [data-utility-controls] select").first()
  ).toHaveValue("dark");

  await page.reload();
  await waitForPreferences(page);
  const reloadedControls = await preferenceControls(page);
  await expect(reloadedControls.theme).toHaveValue("dark");
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

  await controls.locale.selectOption("en");
  await expect(page.locator("[data-preferences-page] h1")).toHaveText("Preferences");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");

  await page.reload();
  await waitForPreferences(page);
  const reloadedControls = await preferenceControls(page);
  await expect(page.locator("[data-preferences-page] h1")).toHaveText("Preferences");
  await expect(reloadedControls.locale).toHaveValue("en");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  expect(requests).toEqual(expectedShellRequests);
});
