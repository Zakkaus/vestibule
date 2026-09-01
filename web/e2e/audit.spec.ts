import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1001163306055";
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

type AuditReadHandler = (route: Route) => Promise<void>;
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
          csrf_token: "audit-csrf"
        })
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: selectedGroupID }] })
      });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/audit` && request.method() === "GET") {
      await readAudit(route);
      return;
    }
    if (
      path === `/api/chats/${selectedGroupID}/audit/${availableAuditEntry.id}/undo` &&
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
      body: JSON.stringify({
        items: [availableAuditEntry, otherActorAuditEntry, declinedAuditEntry]
      })
    });
  }
): Promise<void> {
  await mockAuditTransport(page, readAudit, undoAudit);
  await page.goto(`/audit?group=${selectedGroupID}`);
  await expect(page.locator("[data-audit-page]")).toHaveAttribute(
    "data-audit-state",
    "populated"
  );
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
  await expect(declinedRow).toContainText("已退回");
  await expect(declinedRow).toContainText("答错");
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
        body: JSON.stringify({ items: [] })
      });
    },
    async () => {
      throw new Error("Undo must not run while loading audit records");
    }
  );

  await page.goto(`/audit?group=${selectedGroupID}`, { waitUntil: "domcontentloaded" });
  await readRequested;
  await expect(page.locator("[data-audit-page]")).toHaveAttribute("data-audit-state", "loading");
  await expect(page.getByRole("heading", { name: "还没有已结算的判定" })).toHaveCount(0);
  resolveRead();
  await expect(page.locator("[data-audit-page]")).toHaveAttribute("data-audit-state", "empty");
  await expect(page.getByRole("heading", { name: "还没有已结算的判定" })).toBeVisible();
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
