import { expect, test, type Page } from "@playwright/test";

import {
  mockSpectrumTransport,
  openSpectrumRoute,
  operatorNavigationGroups
} from "./spectrum-fixtures";

const selectedGroupTitle = "Gentoo-zh Community";

const localeLabels = {
  "zh-CN": `群名称：${selectedGroupTitle}`,
  "zh-TW": `群組名稱：${selectedGroupTitle}`,
  en: `Group name: ${selectedGroupTitle}`
} as const;

const operatorPaths = operatorNavigationGroups.flatMap((group) => [...group.paths]);
const bareGroupName = new RegExp(`(?:群|群組|Group)[\\t \\u00a0]+${selectedGroupTitle}`);

async function expectNoBareGroupName(page: Page, route: string, locale: string): Promise<void> {
  const visibleText = await page.locator("body").innerText();
  expect(visibleText, `${locale} ${route} must not render a bare group-name prefix`).not.toMatch(
    bareGroupName
  );
}

for (const [locale, expected] of Object.entries(localeLabels)) {
  for (const route of operatorPaths) {
    test(`${locale} ${route} renders labelled group names`, async ({ page }) => {
      await page.addInitScript((value) => localStorage.setItem("verify-console-locale", value), locale);
      await mockSpectrumTransport(page, { role: "operator" });
      await openSpectrumRoute(page, route);
      await page.waitForLoadState("networkidle");
      await expect(page.locator("html")).toHaveAttribute("lang", locale);
      await expectNoBareGroupName(page, route, locale);
      if (route === "/home") {
        const title = page.locator("#home-context-title");
        await expect(title).toBeVisible();
        await expect(title).toHaveText(expected);
      }
    });
  }
}

