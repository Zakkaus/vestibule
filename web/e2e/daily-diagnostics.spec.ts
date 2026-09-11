import { expect, test } from "@playwright/test";

import {
  clickSidebarLink,
  fulfillJSON,
  mockDiagnosticsTransport,
  unmeasuredDiagnostics
} from "./diagnostics-fixtures";

test("mocked daily save errors keep the prior value and show no false success", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await page.setViewportSize({ width: 320, height: 900 });
  await mockDiagnosticsTransport(
    page,
    "operator",
    async (route) => {
      await fulfillJSON(route, unmeasuredDiagnostics);
    },
    async (route) => {
      if (route.request().method() === "PATCH") {
        await fulfillJSON(route, { error: { code: "csrf_invalid" } }, 403);
        return;
      }
      await fulfillJSON(route, { enabled: true, time: "09:00", timezone: "Asia/Shanghai" });
    }
  );

  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  const toggle = screen.locator("[data-diagnostics-daily-toggle]");
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await toggle.click();

  const feedback = screen.locator('[data-diagnostics-daily-feedback="error"]');
  await expect(feedback).toBeVisible();
  await expect(screen.locator('[data-diagnostics-daily-feedback="saved"]')).toHaveCount(0);
  await expect(toggle).toHaveAttribute("aria-checked", "true");

  const layout = await feedback.evaluate((element) => {
    const parent = element.parentElement;
    const container = element.closest("[data-diagnostics-content]");
    if (!parent || !container) {
      throw new Error("The daily feedback must stay inside diagnostics content");
    }
    const style = getComputedStyle(element);
    const parentStyle = getComputedStyle(parent);
    const containerStyle = getComputedStyle(container);
    const box = element.getBoundingClientRect();
    const containerBox = container.getBoundingClientRect();
    const contentLeft =
      containerBox.left +
      Number.parseFloat(containerStyle.borderLeftWidth) +
      Number.parseFloat(containerStyle.paddingLeft);
    const contentRight =
      containerBox.right -
      Number.parseFloat(containerStyle.borderRightWidth) -
      Number.parseFloat(containerStyle.paddingRight);
    return {
      foreground: style.color,
      parentForeground: parentStyle.color,
      background: style.backgroundColor,
      left: box.left,
      right: box.right,
      contentLeft,
      contentRight,
      scrollWidth: element.scrollWidth,
      clientWidth: element.clientWidth
    };
  });
  expect(layout.foreground).not.toBe(layout.parentForeground);
  expect(layout.background).not.toBe("rgba(0, 0, 0, 0)");
  expect(layout.left).toBeGreaterThanOrEqual(layout.contentLeft - 1);
  expect(layout.right).toBeLessThanOrEqual(layout.contentRight + 1);
  expect(layout.scrollWidth).toBeLessThanOrEqual(layout.clientWidth + 1);
});

test("daily status rereads after SPA navigation while a save is pending", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  let backingEnabled = true;
  const patchStarted = Promise.withResolvers<void>();
  const patchReleased = Promise.withResolvers<void>();
  await mockDiagnosticsTransport(page, "operator", async (route) => fulfillJSON(route, unmeasuredDiagnostics), async (route) => {
    if (route.request().method() === "PATCH") {
      patchStarted.resolve();
      await patchReleased.promise;
      backingEnabled = route.request().postDataJSON().enabled;
    }
    await fulfillJSON(route, { enabled: backingEnabled, time: "09:00", timezone: "Asia/Shanghai" });
  });

  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  const toggle = screen.locator("[data-diagnostics-daily-toggle]");
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await toggle.click();
  await patchStarted.promise;
  await clickSidebarLink(page, "/groups");
  await expect(page).toHaveURL(/\/groups(?:\?|$)/);
  await expect(page.locator("[data-groups-page]")).toBeVisible();
  await clickSidebarLink(page, "/diagnostics");
  const remountedScreen = page.locator("[data-diagnostics-page]");
  await expect(remountedScreen.locator("[data-diagnostics-daily-loading]")).toBeVisible();

  patchReleased.resolve();
  await expect(remountedScreen.locator("[data-diagnostics-daily-toggle]")).toHaveAttribute("aria-checked", "false");
  await remountedScreen.locator("[data-diagnostics-daily-toggle]").click();
  await expect(remountedScreen.locator("[data-diagnostics-daily-toggle]")).toHaveAttribute("aria-checked", "true");
});

test("daily auth expiration asks to reopen from Telegram without a local retry", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await mockDiagnosticsTransport(page, "operator", async (route) => fulfillJSON(route, unmeasuredDiagnostics), async (route) => {
    if (route.request().method() === "PATCH") {
      await fulfillJSON(route, { error: { code: "authentication_expired" } }, 401);
      return;
    }
    await fulfillJSON(route, { enabled: true, time: "09:00", timezone: "Asia/Shanghai" });
  });
  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  const toggle = screen.locator("[data-diagnostics-daily-toggle]");
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await toggle.click();
  const unavailable = screen.locator('[data-diagnostics-daily-state-card="unavailable"]');
  await expect(unavailable).toBeVisible();
  await expect(unavailable).toContainText("Telegram");
  await expect(unavailable.getByRole("button")).toHaveCount(0);
});

test("daily status rejects a schedule other than fixed 09:00", async ({ page }) => {
  await mockDiagnosticsTransport(page, "operator",
    async (route) => fulfillJSON(route, unmeasuredDiagnostics),
    async (route) => fulfillJSON(route, { enabled: true, time: "10:00", timezone: "Asia/Shanghai" })
  );
  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen.locator('[data-diagnostics-daily-state-card="unavailable"]')).toBeVisible();
  await expect(screen.locator("[data-diagnostics-daily-toggle]")).toHaveCount(0);
});

test("daily GET failure recovers with a full-width retry at 320px", async ({ page }) => {
  let available = false;
  await mockDiagnosticsTransport(
    page,
    "operator",
    async (route) => fulfillJSON(route, unmeasuredDiagnostics),
    async (route) => {
      if (route.request().method() !== "GET") {
        throw new Error(`Unexpected daily write: ${route.request().method()}`);
      }
      if (!available) {
        await fulfillJSON(route, { error: { code: "diagnostics_unavailable" } }, 503);
        return;
      }
      await fulfillJSON(route, { enabled: true, time: "09:00", timezone: "Asia/Shanghai" });
    }
  );

  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  const stateCard = screen.locator('[data-diagnostics-daily-state-card="unavailable"]');
  await expect(stateCard).toBeVisible();
  const retry = stateCard.getByRole("button", { name: "重试" });
  await expect(retry).toBeVisible();

  const geometry = await retry.evaluate((element) => {
    const card = element.closest("[data-diagnostics-daily-state-card]");
    if (!card) {
      throw new Error("The daily retry button must stay inside its unavailable card");
    }
    const style = getComputedStyle(card);
    const cardBox = card.getBoundingClientRect();
    const contentLeft =
      cardBox.left +
      Number.parseFloat(style.borderLeftWidth) +
      Number.parseFloat(style.paddingLeft);
    const contentRight =
      cardBox.right -
      Number.parseFloat(style.borderRightWidth) -
      Number.parseFloat(style.paddingRight);
    const buttonBox = element.getBoundingClientRect();
    return {
      buttonLeft: buttonBox.left,
      buttonRight: buttonBox.right,
      buttonWidth: buttonBox.width,
      contentLeft,
      contentRight,
      contentWidth: contentRight - contentLeft
    };
  });
  expect(geometry.buttonLeft).toBeGreaterThanOrEqual(geometry.contentLeft - 1);
  expect(geometry.buttonRight).toBeLessThanOrEqual(geometry.contentRight + 1);
  expect(Math.abs(geometry.buttonWidth - geometry.contentWidth)).toBeLessThanOrEqual(1);

  available = true;
  await retry.click();
  await expect(screen.locator("[data-diagnostics-daily-toggle]")).toHaveAttribute("aria-checked", "true");
  await expect(stateCard).toHaveCount(0);
});
