import { expect, test, type Page, type Route } from "@playwright/test";
import { mockSpectrumTransport, selectedGroupID } from "./spectrum-fixtures";

const sourced = <T,>(value: T) => ({ value, source: "chat override" });
const baseline = {
  revision: 7, delivery_mode: sourced("both"), verify_mode: sourced("mixed"),
  timeout_seconds: sourced(300), verify_max_fails: sourced(3), verify_retry_seconds: sourced(180),
  ban_seconds: sourced(0), mute_seconds: sourced(3600), verify_invited: sourced(true)
};
const latest = { ...baseline, revision: 8, timeout_seconds: sourced(500), mute_seconds: sourced(7200) };
async function json(route: Route, body: unknown, status = 200) {
  await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
}
async function openVerification(page: Page, patch: (route: Route) => Promise<void>) {
  await mockSpectrumTransport(page);
  let reads = 0;
  await page.route("**/settings", async route => {
    if (route.request().method() === "PATCH") await patch(route);
    else await json(route, ++reads === 1 ? baseline : latest);
  });
  await page.goto(`/verification?group=${selectedGroupID}`);
  await expect(page.locator("#verification-timeout-seconds")).toHaveValue("300");
}

test.use({ locale: "en" });
test("dirty question bank blocks navigation and cancel preserves URL and draft", async ({ page }) => {
  await mockSpectrumTransport(page);
  await page.goto(`/questions?group=${selectedGroupID}`);
  const prompt = page.locator("[data-question-bank-editor] textarea").first();
  await prompt.fill("Keep this unsaved question");
  const url = page.url();
  await page.locator('[data-navigation-item="/verification"]').click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page).toHaveURL(url);
  await expect(prompt).toHaveValue("Keep this unsaved question");
  await page.locator('[data-navigation-item="/verification"]').click();
  await dialog.getByRole("button", { name: "Discard and continue" }).click();
  await expect(page).toHaveURL(new RegExp(`/verification\\?group=${selectedGroupID}`));
});

test("verification conflict preserves delta, reviews overlapping fields and reapplies against latest revision", async ({ page }) => {
  const writes: unknown[] = [];
  await openVerification(page, async route => {
    writes.push(route.request().postDataJSON());
    if (writes.length === 1) await json(route, { error: { code: "settings_conflict" } }, 409);
    else await json(route, { ...latest, revision: 9, timeout_seconds: sourced(360) });
  });
  await page.locator("#verification-timeout-seconds").fill("360");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  const conflict = page.locator("[data-verification-conflict]");
  await expect(conflict).toContainText("500");
  await expect(conflict).toContainText("360");
  await expect(conflict.getByRole("button", { name: "Discard my changes" })).toBeVisible();
  await expect(page.locator("#verification-timeout-seconds")).toHaveValue("360");
  await conflict.getByRole("button", { name: "Reapply my changes" }).click();
  await expect(page.locator("#verification-mute-seconds")).toHaveValue("7200");
  expect(writes).toHaveLength(1);
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect.poll(() => writes).toEqual([
    { expected_revision: 7, changes: { timeout_seconds: 360 } },
    { expected_revision: 8, changes: { timeout_seconds: 360 } }
  ]);
});

test("conflict discard adopts latest values without writing again", async ({ page }) => {
  let writes = 0;
  await openVerification(page, async route => { writes++; await json(route, { error: { code: "settings_conflict" } }, 409); });
  await page.locator("#verification-timeout-seconds").fill("360");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await page.locator("[data-verification-conflict]").getByRole("button", { name: "Discard my changes" }).click();
  await expect(page.locator("#verification-timeout-seconds")).toHaveValue("500");
  await expect(page.getByRole("button", { name: "Save changes", exact: true })).toBeDisabled();
  expect(writes).toBe(1);
});

test("unknown write outcome is informational and offers a refetch without losing the draft", async ({ page }) => {
  await openVerification(page, route => route.abort("failed"));
  await page.locator("#verification-timeout-seconds").fill("360");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  const feedback = page.locator("[data-verification-feedback]");
  await expect(feedback).toContainText("write outcome is unknown");
  await expect(page.locator('[data-feedback-level="info"]')).toBeVisible();
  await feedback.getByRole("button", { name: "Refetch latest state" }).click();
  await expect(page.locator("[data-verification-conflict]")).toBeVisible();
  await expect(page.locator("#verification-timeout-seconds")).toHaveValue("360");
});

test("pending writes cannot be discarded by navigation", async ({ page }) => {
  let settle!: () => void;
  const pending = new Promise<void>(resolve => { settle = resolve; });
  await openVerification(page, async route => { await pending; await json(route, { ...baseline, revision: 8, timeout_seconds: sourced(360) }); });
  await page.locator("#verification-timeout-seconds").fill("360");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await page.locator('[data-navigation-item="/questions"]').click();
  const leave = page.getByRole("alertdialog").getByRole("button", { name: "Discard and continue" });
  await expect(leave).toBeDisabled();
  settle();
  await expect(leave).toBeEnabled();
  await leave.click();
  await expect(page).toHaveURL(new RegExp("/questions\\?"));
});

test("Back and Forward cancel restore the URL and retain a question draft", async ({ page }) => {
  await mockSpectrumTransport(page);
  await page.goto(`/verification?group=${selectedGroupID}`);
  await page.locator('[data-navigation-item="/questions"]').click();
  const prompt = page.locator("[data-question-bank-editor] textarea").first();
  await prompt.fill("History draft");
  const url = page.url();
  await page.evaluate(() => history.back());
  await page.getByRole("alertdialog").getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page).toHaveURL(url);
  await expect(prompt).toHaveValue("History draft");
  await page.evaluate(() => history.back());
  await page.getByRole("alertdialog").getByRole("button", { name: "Discard and continue" }).click();
  await expect(page).toHaveURL(new RegExp("/verification\\?"));
  await page.locator("#verification-timeout-seconds").fill("360");
  const verificationURL = page.url();
  await page.evaluate(() => history.forward());
  await page.getByRole("alertdialog").getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page).toHaveURL(verificationURL);
  await expect(page.locator("#verification-timeout-seconds")).toHaveValue("360");
});

test("group switch cancellation keeps the selected group and reverting the draft removes the guard", async ({ page }) => {
  await mockSpectrumTransport(page);
  await page.goto(`/verification?group=${selectedGroupID}`);
  const timeout = page.locator("#verification-timeout-seconds");
  await timeout.fill("360");
  await page.getByRole("button", { name: "Current group" }).click();
  await page.getByRole("option", { name: "Arch Linux Community", exact: true }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`group=${selectedGroupID}`));
  await expect(timeout).toHaveValue("360");
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
  await timeout.click();
  await timeout.fill("300");
  await expect(page.getByRole("button", { name: "Save changes", exact: true })).toBeDisabled();
  await page.locator('[data-navigation-item="/questions"]').click();
  await expect(page).toHaveURL(new RegExp("/questions\\?"));
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
});

test.describe("phone draft guard", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true });
  test("touch navigation and keyboard cancellation return to an editable draft", async ({ page }) => {
    await mockSpectrumTransport(page);
    await page.goto(`/verification?group=${selectedGroupID}`);
    const timeout = page.locator("#verification-timeout-seconds");
    await timeout.fill("360");
    await page.locator('[data-navigation-item="/questions"]').filter({ visible: true }).tap();
    await expect(page.getByRole("alertdialog")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("alertdialog")).toHaveCount(0);
    await timeout.tap();
    await timeout.fill("420");
    await page.locator("[data-verification-savebar]").getByRole("button", { name: "Discard", exact: true }).tap();
    await expect(timeout).toHaveValue("300");
    await expect(page.getByRole("button", { name: "Save changes", exact: true })).toBeDisabled();
  });
});
