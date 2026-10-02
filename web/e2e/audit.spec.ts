import { expect, test, type Page, type Route } from "@playwright/test";
import { selectAppOption } from "./app-select";

const selectedGroupID = "-1009000010001";
const otherGroupID = "-1009000000001";
const actorID = "741928306";
const availableAuditEntry = {
  id: `${selectedGroupID}:528106774:audit-ban`,
  kind: "challenge",
  user: "@undo_target",
  group_key: selectedGroupID,
  result: { state: "banned", reason: null },
  settled_at: "2026-08-31T14:09:00+08:00",
  settled_by: actorID,
  undo_state: "available"
} as const;
const completedAuditEntry = {
  ...availableAuditEntry,
  undo_state: "completed"
} as const;
const otherActorAuditEntry = {
  ...availableAuditEntry,
  id: `${selectedGroupID}:528106775:other-actor-ban`,
  user: "@other_actor_target",
  settled_by: "17",
  undo_state: "unavailable"
} as const;
const declinedAuditEntry = {
  id: `${selectedGroupID}:528106776:wrong-answer`,
  kind: "challenge",
  user: "@wrong_answer",
  group_key: selectedGroupID,
  result: { state: "declined", reason: "wrong_answer" },
  settled_at: "2026-08-31T13:58:00+08:00",
  settled_by: null,
  undo_state: "unavailable"
} as const;

type AuditReadHandler = (route: Route, chatID: string) => Promise<void>;
type AuditUndoHandler = (route: Route) => Promise<void>;

async function mockAuditTransport(
  page: Page,
  readAudit: AuditReadHandler,
  undoAudit: AuditUndoHandler
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: { telegram_id: actorID, role: "manager" },
          expires_at: "2026-09-01T02:00:00Z",
          is_owner: false,
          csrf_token: "audit-csrf"
        })
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: selectedGroupID, title: "Gentoo-zh Community", owner: null, administrators: [], administrators_status: "unavailable" }, { id: otherGroupID, title: "Arch Linux Community", owner: null, administrators: [], administrators_status: "unavailable" }] })
      });
      return;
    }
    if (
      (path === `/api/chats/${selectedGroupID}/audit` ||
        path === `/api/chats/${otherGroupID}/audit`) &&
      request.method() === "GET"
    ) {
      const chatID =
        path === `/api/chats/${selectedGroupID}/audit` ? selectedGroupID : otherGroupID;
      await readAudit(route, chatID);
      return;
    }
    if (
      path.startsWith(`/api/chats/${selectedGroupID}/audit/`) && path.endsWith("/undo") &&
      request.method() === "POST"
    ) {
      await undoAudit(route);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openLiveAudit(
  page: Page,
  undoAudit: AuditUndoHandler,
  readAudit: AuditReadHandler = async (route) => {
    await route.fulfill({
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ items: [availableAuditEntry, otherActorAuditEntry, declinedAuditEntry], next_cursor: null })
    });
  }
): Promise<void> {
  await mockAuditTransport(page, readAudit, undoAudit);
  await page.goto(`/audit?group=${selectedGroupID}`);
  await expect(page.locator("[data-audit-page]")).toHaveAttribute(
    "data-audit-state",
    "populated"
  );
  const groupSwitcher = page.locator("[data-group-switcher]").getByRole("button");
  await expect(groupSwitcher).toBeEnabled();
  await expect(groupSwitcher).toContainText("Gentoo-zh Community");
}

test("audit renders settled history and waits for confirmed undo", async ({ page }) => {
  let markUndoRequested!: () => void;
  let resolveUndo!: () => void;
  const undoRequested = new Promise<void>((resolve) => {
    markUndoRequested = resolve;
  });
  const undoResponse = new Promise<void>((resolve) => {
    resolveUndo = resolve;
  });

  await openLiveAudit(page, async (route) => {
    const request = route.request();
    expect(request.headers()["x-csrf-token"]).toBe("audit-csrf");
    expect(request.postData()).toBeNull();
    markUndoRequested();
    await undoResponse;
    await route.fulfill({
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(completedAuditEntry)
    });
  });

  const availableRow = page.locator("[data-audit-row]", { hasText: "@undo_target" });
  const otherActorRow = page.locator("[data-audit-row]", {
    hasText: "@other_actor_target"
  });
  const declinedRow = page.locator("[data-audit-row]", { hasText: "@wrong_answer" });
  await expect(availableRow).toContainText("已封禁");
  await expect(otherActorRow.getByRole("button")).toHaveCount(0);
  await expect(declinedRow).toContainText("已拒绝");
  await expect(declinedRow).toContainText("回答错误");
  await expect(declinedRow).toContainText("系统");

  const action = availableRow.getByRole("button", { name: "撤销对 @undo_target 的封禁" });
  await action.click();
  await undoRequested;
  await expect(availableRow).toHaveAttribute("data-undo-state", "submitting");
  await expect(action).toHaveAttribute("aria-disabled", "true");
  await expect(page.locator("[data-audit-feedback]")).toHaveCount(0);

  resolveUndo();
  await expect(availableRow).toHaveAttribute("data-undo-state", "completed");
  await expect(availableRow.getByRole("button")).toHaveCount(0);
  await expect(availableRow).toContainText("已撤销");
  await expect(page.locator("[data-audit-feedback]")).toContainText(
    "已撤销对 @undo_target 的封禁"
  );
});

test("audit loading does not masquerade as an empty history", async ({ page }) => {
  let markReadRequested!: () => void;
  let resolveRead!: () => void;
  const readRequested = new Promise<void>((resolve) => {
    markReadRequested = resolve;
  });
  const readResponse = new Promise<void>((resolve) => {
    resolveRead = resolve;
  });
  await mockAuditTransport(
    page,
    async (route) => {
      markReadRequested();
      await readResponse;
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [], next_cursor: null })
      });
    },
    async () => {
      throw new Error("Undo must not run while loading audit records");
    }
  );

  await page.goto(`/audit?group=${selectedGroupID}`, { waitUntil: "domcontentloaded" });
  await readRequested;
  await expect(page.locator("[data-audit-page]")).toHaveAttribute("data-audit-state", "loading");
  await expect(page.getByRole("heading", { name: "尚无已结算的验证判定" })).toHaveCount(0);
  resolveRead();
  await expect(page.locator("[data-audit-page]")).toHaveAttribute("data-audit-state", "empty");
  await expect(page.getByRole("heading", { name: "尚无已结算的验证判定" })).toBeVisible();
});

test("audit discards group A's delayed read after the visible group switcher selects group B", async ({
  page
}) => {
  let resolveARead!: () => void;
  const aReadResponse = new Promise<void>((resolve) => {
    resolveARead = resolve;
  });
  let markAReadRequested!: () => void;
  const aReadRequested = new Promise<void>((resolve) => {
    markAReadRequested = resolve;
  });
  const groupBAuditEntry = {
    ...availableAuditEntry,
    id: `${otherGroupID}:528106774:audit-ban`,
    group_key: otherGroupID,
    user: "@audit_group_b"
  } as const;

  await mockAuditTransport(
    page,
    async (route, chatID) => {
      if (chatID === selectedGroupID) {
        markAReadRequested();
        await aReadResponse;
        await route.fulfill({
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ items: [availableAuditEntry], next_cursor: null })
        });
        return;
      }
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [groupBAuditEntry], next_cursor: null })
      });
    },
    async () => {
      throw new Error("Undo must not run while a delayed audit read is pending");
    }
  );

  await page.goto(`/audit?group=${selectedGroupID}`, { waitUntil: "domcontentloaded" });
  await aReadRequested;

  const groupSwitcher = page.getByRole("button", { name: "当前群组" });
  await selectAppOption(groupSwitcher, otherGroupID);
  await expect(page).toHaveURL(new RegExp(`/audit\\?group=${otherGroupID}$`));
  await expect(groupSwitcher).toContainText("Arch Linux Community");
  await expect(groupSwitcher).not.toContainText(/-100\d+/);
  await expect(page.locator("[data-audit-row]", { hasText: "@audit_group_b" })).toBeVisible();
  await expect(page.locator("[data-audit-row]", { hasText: "@audit_group_b" })).toContainText("Arch Linux Community");
  await expect(page.locator("[data-audit-page]")).not.toContainText(/-100\d+/);

  const staleReadResponse = page.waitForResponse(
    (response) =>
      decodeURIComponent(new URL(response.url()).pathname) ===
        `/api/chats/${selectedGroupID}/audit` && response.request().method() === "GET"
  );
  resolveARead();
  await staleReadResponse;
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
  await expect(page.locator("[data-audit-row]", { hasText: "@undo_target" })).toHaveCount(0);
  await expect(page.locator("[data-audit-row]", { hasText: "@audit_group_b" })).toBeVisible();
});

test("audit discards group A's delayed undo after the visible group switcher selects group B", async ({
  page
}) => {
  let resolveUndo!: () => void;
  const undoResponse = new Promise<void>((resolve) => {
    resolveUndo = resolve;
  });
  let markUndoRequested!: () => void;
  const undoRequested = new Promise<void>((resolve) => {
    markUndoRequested = resolve;
  });
  const groupBAuditEntry = {
    ...availableAuditEntry,
    id: `${otherGroupID}:528106775:audit-ban`,
    group_key: otherGroupID,
    user: "@audit_group_b"
  } as const;

  await mockAuditTransport(
    page,
    async (route, chatID) => {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [chatID === selectedGroupID ? availableAuditEntry : groupBAuditEntry], next_cursor: null })
      });
    },
    async (route) => {
      const request = route.request();
      expect(request.headers()["x-csrf-token"]).toBe("audit-csrf");
      expect(request.postData()).toBeNull();
      markUndoRequested();
      await undoResponse;
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(completedAuditEntry)
      });
    }
  );

  await page.goto(`/audit?group=${selectedGroupID}`);
  const groupARow = page.locator("[data-audit-row]", { hasText: "@undo_target" });
  await expect(groupARow).toBeVisible();
  await groupARow.getByRole("button", { name: "撤销对 @undo_target 的封禁" }).click();
  await undoRequested;
  await expect(groupARow).toHaveAttribute("data-undo-state", "submitting");

  const groupSwitcher = page.getByRole("button", { name: "当前群组" });
  await selectAppOption(groupSwitcher, otherGroupID);
  await expect(page).toHaveURL(new RegExp(`/audit\\?group=${otherGroupID}$`));
  await expect(groupSwitcher).toContainText("Arch Linux Community");
  await expect(groupSwitcher).not.toContainText(/-100\d+/);
  await expect(page.locator("[data-audit-row]", { hasText: "@audit_group_b" })).toBeVisible();
  await expect(page.locator("[data-audit-row]", { hasText: "@audit_group_b" })).toContainText("Arch Linux Community");
  await expect(page.locator("[data-audit-page]")).not.toContainText(/-100\d+/);

  const staleUndoResponse = page.waitForResponse(
    (response) =>
      decodeURIComponent(new URL(response.url()).pathname) ===
        `/api/chats/${selectedGroupID}/audit/${availableAuditEntry.id}/undo` &&
      response.request().method() === "POST"
  );
  resolveUndo();
  await staleUndoResponse;
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));

  const groupBRow = page.locator("[data-audit-row]", { hasText: "@audit_group_b" });
  await expect(groupARow).toHaveCount(0);
  await expect(groupBRow).toHaveAttribute("data-undo-state", "available");
  await expect(page.locator("[data-audit-feedback]")).toHaveCount(0);
});

test("audit sends one undo for a forced second click and shows the confirmed result", async ({
  page
}) => {
  let undoRequests = 0;
  let resolveUndo!: () => void;
  const undoResponse = new Promise<void>((resolve) => {
    resolveUndo = resolve;
  });
  let markUndoRequested!: () => void;
  const undoRequested = new Promise<void>((resolve) => {
    markUndoRequested = resolve;
  });

  await mockAuditTransport(
    page,
    async (route) => {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [availableAuditEntry], next_cursor: null })
      });
    },
    async (route) => {
      undoRequests += 1;
      const request = route.request();
      expect(request.headers()["x-csrf-token"]).toBe("audit-csrf");
      expect(request.postData()).toBeNull();
      markUndoRequested();
      await undoResponse;
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(completedAuditEntry)
      });
    }
  );

  await page.goto(`/audit?group=${selectedGroupID}`);
  const row = page.locator("[data-audit-row]", { hasText: "@undo_target" });
  const action = row.locator("[data-audit-action='undo']");
  await expect(row).toHaveAttribute("data-undo-state", "available");
  await action.click();
  await undoRequested;
  await expect(action).toHaveAttribute("aria-disabled", "true");
  await action.click({ force: true });
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
  expect(undoRequests).toBe(1);

  const undoSuccessResponse = page.waitForResponse(
    (response) =>
      decodeURIComponent(new URL(response.url()).pathname) ===
        `/api/chats/${selectedGroupID}/audit/${availableAuditEntry.id}/undo` &&
      response.request().method() === "POST"
  );
  resolveUndo();
  await undoSuccessResponse;
  await expect(row).toHaveAttribute("data-undo-state", "completed");
  await expect(row.getByRole("button")).toHaveCount(0);
  await expect(page.locator("[data-audit-feedback]")).toContainText(
    "已撤销对 @undo_target 的封禁"
  );
  expect(undoRequests).toBe(1);
});

test("narrow audit card completes undo from the keyboard", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 900 });
  await openLiveAudit(page, async (route) => {
    await route.fulfill({
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(completedAuditEntry)
    });
  });

  await expect(page.locator("[data-audit-table-scroll]")).toBeHidden();
  const card = page.locator("[data-audit-card-row]", { hasText: "@undo_target" });
  const action = card.getByRole("button", { name: "撤销对 @undo_target 的封禁" });
  await expect(action).toBeVisible();
  await action.focus();
  await expect(action).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(card).toHaveAttribute("data-undo-state", "completed");
  await expect(card.getByRole("button")).toHaveCount(0);
});

test("an interrupted undo provides the activity-log reload it names", async ({ page }) => {
  let auditReads = 0;
  await openLiveAudit(
    page,
    async (route) => route.abort("failed"),
    async (route) => {
      auditReads += 1;
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [availableAuditEntry], next_cursor: null })
      });
    }
  );

  await page.getByRole("button", { name: "撤销对 @undo_target 的封禁" }).click();
  const feedback = page.locator("[data-audit-feedback]");
  await expect(feedback).toContainText("连接已中断");
  await feedback.getByRole("button", { name: "重新读取" }).click();
  await expect.poll(() => auditReads).toBe(2);
  await expect(feedback).toHaveCount(0);
});

test("audit loads older pages on demand, retries that page, and undoes its ban", async ({ page }) => {
  let reads = 0;
  let olderReads = 0;
  await openLiveAudit(page, async (route) => {
    expect(route.request().headers()["x-csrf-token"]).toBe("audit-csrf");
    await route.fulfill({ json: completedAuditEntry });
  }, async (route) => {
    reads++;
    const cursor = new URL(route.request().url()).searchParams.get("cursor");
    if (!cursor) {
      await route.fulfill({ json: { items: [declinedAuditEntry], next_cursor: "opaque/older+page" } });
      return;
    }
    expect(cursor).toBe("opaque/older+page");
    if (++olderReads === 1) {
      await route.abort("failed");
      return;
    }
    await route.fulfill({ json: { items: [availableAuditEntry], next_cursor: null } });
  });
  const older = page.getByRole("button", { name: "加载更多" });
  await expect(older).toBeVisible();
  expect(reads).toBe(1);
  await expect(page.locator("[data-audit-row]", { hasText: "@undo_target" })).toHaveCount(0);
  await older.click();
  await expect(page.getByRole("alert")).toContainText("连接已中断");
  await expect(page.locator("[data-audit-row]", { hasText: "@wrong_answer" })).toBeVisible();
  await older.click();
  const ban = page.locator("[data-audit-row]", { hasText: "@undo_target" });
  await expect(ban).toBeVisible();
  await expect(older).toHaveCount(0);
  await expect(page.locator("[data-audit-row]", { hasText: "@wrong_answer" })).toBeVisible();
  await ban.getByRole("button", { name: "撤销对 @undo_target 的封禁" }).click();
  await expect(ban).toHaveAttribute("data-undo-state", "completed");
});

test("audit ignores an older page after switching groups", async ({ page }) => {
  let release!: () => void;
  let requested!: () => void;
  const delayed = new Promise<void>((resolve) => { release = resolve; });
  const started = new Promise<void>((resolve) => { requested = resolve; });
  await openLiveAudit(page, async () => { throw new Error("Unexpected undo"); }, async (route, chatID) => {
    if (chatID === otherGroupID) {
      await route.fulfill({ json: { items: [], next_cursor: null } });
    } else if (new URL(route.request().url()).searchParams.has("cursor")) {
      requested();
      await delayed;
      await route.fulfill({ json: { items: [availableAuditEntry], next_cursor: null } });
    } else {
      await route.fulfill({ json: { items: [declinedAuditEntry], next_cursor: "older" } });
    }
  });
  await page.getByRole("button", { name: "加载更多" }).click();
  await started;
  await selectAppOption(page.getByRole("button", { name: "当前群组" }), otherGroupID);
  await expect(page.locator("[data-audit-page]")).toHaveAttribute("data-audit-state", "empty");
  const response = page.waitForResponse((item) => new URL(item.url()).searchParams.get("cursor") === "older");
  release();
  await response;
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
  await expect(page.locator("[data-audit-row]")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "加载更多" })).toHaveCount(0);
});

test("a same-group conflict reload clears other pending undo bookkeeping", async ({ page }) => {
  const second = { ...availableAuditEntry, id: `${selectedGroupID}:528106777:second-ban`, user: "@reload_target" };
  let releaseConflict!: () => void;
  let releaseOldUndo!: () => void;
  let markOldUndoStarted!: () => void;
  const conflict = new Promise<void>((resolve) => { releaseConflict = resolve; });
  const oldUndo = new Promise<void>((resolve) => { releaseOldUndo = resolve; });
  const oldUndoStarted = new Promise<void>((resolve) => { markOldUndoStarted = resolve; });
  let reads = 0;
  let secondUndos = 0;
  await openLiveAudit(page, async (route) => {
    const path = decodeURIComponent(new URL(route.request().url()).pathname);
    if (path.includes(availableAuditEntry.id)) {
      await conflict;
      await route.fulfill({ status: 409, json: { error: { code: "audit_conflict" } } });
    } else if (++secondUndos === 1) {
      markOldUndoStarted();
      await oldUndo;
      await route.abort("failed");
    } else {
      await route.fulfill({ json: { ...second, undo_state: "completed" } });
    }
  }, async (route) => {
    await route.fulfill({ json: { items: ++reads === 1 ? [availableAuditEntry, second] : [second], next_cursor: null } });
  });
  const row = page.locator("[data-audit-row]", { hasText: second.user });
  await page.locator("[data-audit-row]", { hasText: availableAuditEntry.user }).getByRole("button").click();
  await row.getByRole("button").click();
  await oldUndoStarted;
  await expect(row).toHaveAttribute("data-undo-state", "submitting");
  releaseConflict();
  await expect.poll(() => reads).toBe(2);
  await expect(row).toHaveAttribute("data-undo-state", "available");
  const staleResponse = page.waitForEvent("requestfailed", { predicate: (request) => request.url().includes(encodeURIComponent(second.id)) });
  releaseOldUndo();
  await staleResponse;
  await expect(row).toHaveAttribute("data-undo-state", "available");
  await expect(page.locator("[data-audit-feedback]")).toHaveCount(0);
  await row.getByRole("button").click();
  await expect(row).toHaveAttribute("data-undo-state", "completed");
});

test("a failed lazy audit download keeps navigation and offers localized reload", async ({ page }) => {
  await mockAuditTransport(page, async (route) => {
    await route.fulfill({ json: { items: [availableAuditEntry], next_cursor: null } });
  }, async () => { throw new Error("Unexpected undo"); });
  const modulePattern = "**/src/features/audit/index.ts";
  await page.route(modulePattern, (route) => route.abort("failed"));
  await page.goto(`/audit?group=${selectedGroupID}`);
  const error = page.locator("[data-route-error]");
  await expect(error.getByRole("heading", { name: "无法打开此页面" })).toBeVisible();
  const navigation = page.getByRole("treegrid", { name: "控制台导航" });
  await expect(navigation).toBeVisible();
  const groupSwitcher = page.locator("[data-group-switcher]").getByRole("button");
  await expect(groupSwitcher).toBeEnabled();
  await expect(groupSwitcher).toContainText("Gentoo-zh Community");
  const auditLink = navigation.getByRole("link", { name: "验证判定记录", exact: true });
  const preferencesLink = navigation.getByRole("link", { name: "偏好", exact: true });
  await expect(auditLink).toHaveAttribute("aria-current", "page");
  await expect(page.getByText("Unexpected Application Error!", { exact: true })).toHaveCount(0);
  await page.locator(".console-brand a").focus();
  await page.keyboard.press("Tab");
  await expect(auditLink).toBeFocused();
  await preferencesLink.focus();
  await expect(preferencesLink).toBeFocused();
  await expect(navigation.getByRole("row", { name: "偏好", exact: true })).toHaveAttribute("data-focus-visible", "true");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/preferences\?group=/);
  await expect(error).toHaveCount(0);
  await expect(page.locator("[data-preferences-page]")).toBeVisible();
  await auditLink.focus();
  await expect(auditLink).toBeFocused();
  await expect(navigation.getByRole("row", { name: "验证判定记录", exact: true })).toHaveAttribute("data-focus-visible", "true");
  await page.keyboard.press("Enter");
  await expect(error).toBeVisible();
  await page.unroute(modulePattern);
  await error.getByRole("button", { name: "重新加载页面" }).click();
  await expect(page.locator("[data-audit-page]")).toHaveAttribute("data-audit-state", "populated");
  await expect(error).toHaveCount(0);
});

for (const locale of ["en", "zh-CN"]) {
  test.describe(`audit desktop layout in ${locale}`, () => {
    test.use({ locale, viewport: { width: 1280, height: 900 } });
    test("all columns and the undo action fit without horizontal scrolling", async ({ page }) => {
      await openLiveAudit(page, async (route) => { await route.fulfill({ json: completedAuditEntry }); });
      const geometry = await page.locator("[data-audit-table-scroll]").evaluate((scrollport) => {
        const bounds = scrollport.getBoundingClientRect();
        const inside = (element: Element) => {
          const rect = element.getBoundingClientRect();
          return rect.left >= bounds.left - 1 && rect.right <= bounds.right + 1;
        };
        const action = scrollport.querySelector("[data-audit-action]");
        const textBounds = (element: Element) => {
          const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
          const rectangles: DOMRect[] = [];
          while (walker.nextNode()) {
            if (!walker.currentNode.textContent?.trim()) continue;
            const range = document.createRange();
            range.selectNodeContents(walker.currentNode);
            rectangles.push(...Array.from(range.getClientRects()));
          }
          return {
            height: Math.max(...rectangles.map((rect) => rect.bottom)) - Math.min(...rectangles.map((rect) => rect.top)),
            right: Math.max(...rectangles.map((rect) => rect.right))
          };
        };
        const singleLine = Array.from(scrollport.querySelectorAll(
          "td[data-record-user], td[data-record-actor], td[data-record-result], td[data-record-time]"
        )).every((cell) => textBounds(cell).height <= parseFloat(getComputedStyle(cell).lineHeight) + 1);
        const time = action?.closest("tr")?.querySelector("[data-record-time]");
        return {
          overflow: scrollport.scrollWidth - scrollport.clientWidth,
          columnsVisible: Array.from(scrollport.querySelectorAll("th, td")).every(inside),
          actionVisible: !!action && inside(action) && action.scrollWidth <= action.clientWidth + 1,
          singleLine,
          actionGap: action && time ? action.getBoundingClientRect().left - textBounds(time).right : Infinity
        };
      });
      expect(geometry.overflow).toBe(0);
      expect(geometry.columnsVisible).toBe(true);
      expect(geometry.actionVisible).toBe(true);
      expect(geometry.singleLine).toBe(true);
      expect(geometry.actionGap).toBeGreaterThanOrEqual(0);
      expect(geometry.actionGap).toBeLessThan(48);
      await expect(page.locator("td[data-record-user] span", { hasText: "@other_actor_target" })).toHaveAttribute("title", "@other_actor_target");
    });
  });
}
