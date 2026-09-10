import { expect, test, type Page } from "@playwright/test";
import { mockSpectrumTransport, openSpectrumRoute } from "./spectrum-fixtures";

const locales = ["zh-CN", "zh-TW", "en"] as const;

async function holdExternalRequests(page: Page, baseURL: string): Promise<string[]> {
  const attempted: string[] = [];
  const baseOrigin = new URL(baseURL).origin;

  await page.route((url) => {
    return (
      (url.protocol === "http:" || url.protocol === "https:") &&
      url.origin !== baseOrigin
    );
  }, (route) => {
    attempted.push(route.request().url());
    // Leave the request unresolved to model an outbound connection that never completes.
  });

  return attempted;
}


for (const locale of locales) {
  test(`renders stats and home without external assets in ${locale}`, async ({ page }, testInfo) => {
    await page.addInitScript((value) => localStorage.setItem("verify-console-locale", value), locale);
    await mockSpectrumTransport(page);
    const attemptedExternalRequests = await holdExternalRequests(
      page,
      testInfo.project.use.baseURL as string
    );

    await openSpectrumRoute(page, "/stats");
    await expect(page.locator("[data-stats-page]")).toHaveAttribute("data-stats-state", "loaded");
    await expect(page.locator("[data-stats-summary]")).toBeVisible();
    await expect(page.locator("[data-stats-summary]")).toContainText("70");
    await expect(page.locator("[data-stats-chart-svg]")).toBeVisible();
    expect(attemptedExternalRequests).toEqual([]);

    await openSpectrumRoute(page, "/home");
    await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");
    await expect(page.locator("[data-home-chart-reading]")).toBeVisible();
    const chart = page.getByTestId("home-combined-chart");
    await expect(chart).toBeVisible();
    await expect(chart.locator("svg")).toBeVisible();
    await expect(chart.locator('[aria-label^="X-axis"] .role-axis-label text')).toHaveCount(7);

    expect(attemptedExternalRequests).toEqual([]);
  });
}
