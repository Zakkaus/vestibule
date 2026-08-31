import { expect, test, type Page } from "@playwright/test";

const selectedGroupId = "-1001163306055";

function rowFor(page: Page, user: string) {
  return page.locator("[data-queue-row]", { hasText: user });
}

async function openFixtureQueue(page: Page): Promise<void> {
  await page.goto("/queue");
  await expect(page.locator("[data-queue-page]")).toHaveAttribute(
    "data-queue-state",
    "populated"
  );
}

test("fixture session reaches a successful release", async ({ page }) => {
  await test.step("enter through the fixture session path", async () => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/$/);
    await expect(page.locator("[data-entry-page]")).toHaveAttribute(
      "data-entry-state",
      "no-session"
    );
    await expect(
      page.getByRole("heading", { name: "从 Telegram 打开控制台" })
    ).toBeVisible();

    // Without the backend, /groups is the successful fixture-session result.
    await page.goto("/groups");
    await expect(page.locator("[data-groups-page]")).toBeVisible();
  });

  await test.step("select a managed group", async () => {
    const groupSwitcher = page.getByRole("combobox", { name: "当前群" });
    await groupSwitcher.selectOption(selectedGroupId);
    await expect(groupSwitcher).toHaveValue(selectedGroupId);
    await expect(page.locator("[data-group-row][data-selected]")).toContainText(
      "Gentoo 中文社区"
    );
  });

  await test.step("open the waiting queue", async () => {
    await page.getByRole("link", { name: "等待队列" }).click();
    await expect(page.locator("[data-queue-page]")).toHaveAttribute(
      "data-queue-state",
      "populated"
    );
  });

  await test.step("release one waiting applicant", async () => {
    const row = rowFor(page, "@another");
    await expect(row).toHaveAttribute("data-result", "pending");
    await expect(row.locator("[data-queue-result]")).toContainText("等待中 2:41");

    await row.getByRole("button", { name: "放行 @another" }).click();

    await expect(row).toHaveAttribute("data-result", "approved");
    const feedback = page.locator(
      '[data-queue-feedback][data-feedback-kind="releaseSuccess"]'
    );
    await expect(feedback).toContainText("已放行 @another");
    await expect(row).toHaveAttribute("data-action-state", "idle");
    await expect(row.locator("[data-queue-result]")).toHaveText("已通过");
  });
});

test("a failed release restores pending state and remaining time", async ({
  page
}) => {
  await openFixtureQueue(page);

  const row = rowFor(page, "@retry_release");
  const result = row.locator("[data-queue-result]");
  await expect(row).toHaveAttribute("data-result", "pending");
  await expect(result).toContainText("等待中 3:39");

  await row.getByRole("button", { name: "放行 @retry_release" }).click();
  await expect(row).toHaveAttribute("data-result", "approved");

  const feedback = page.locator(
    '[data-queue-feedback][data-feedback-kind="releaseFailure"]'
  );
  await expect(feedback).toContainText("没能放行 @retry_release");
  await expect(row).toHaveAttribute("data-action-state", "idle");
  await expect(row).toHaveAttribute("data-result", "pending");
  await expect(result).toContainText("等待中 3:39");
});

test("revoke does nothing without confirmation", async ({ page }) => {
  await openFixtureQueue(page);

  const row = rowFor(page, "@spam_ad_01");
  await expect(row).toHaveAttribute("data-result", "banned");

  await row
    .getByRole("button", { name: "撤销 @spam_ad_01 的封禁" })
    .click();

  const dialog = page.getByRole("dialog", {
    name: "撤销 @spam_ad_01 的封禁"
  });
  await expect(dialog).toBeVisible();
  await expect(row).toHaveAttribute("data-action-state", "idle");
  await expect(row).toHaveAttribute("data-result", "banned");
  await expect(page.locator("[data-queue-feedback]")).toHaveCount(0);

  await dialog.getByRole("button", { name: "取消" }).click();
  await expect(dialog).not.toBeVisible();
  await page.waitForTimeout(800);
  await expect(row).toHaveAttribute("data-action-state", "idle");
  await expect(row).toHaveAttribute("data-result", "banned");
  await expect(page.locator("[data-queue-feedback]")).toHaveCount(0);
});
