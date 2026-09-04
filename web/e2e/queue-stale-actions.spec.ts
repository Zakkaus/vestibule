import { expect, test, type Page, type Route } from "@playwright/test";
import { selectAppOption } from "./app-select";

const groupAID = "-1009000000101";
const groupBID = "-1009000000102";
const actorID = "741928306";
const groupAQueueEntry = {
  id: `${groupAID}:528106774:queue-nonce`,
  user: "@queue_group_a",
  group_key: groupAID,
  result: { state: "pending", reason: null },
  occurred_at: "2026-08-31T14:09:00+08:00",
  expires_at: "2026-08-31T14:11:41+08:00",
  remaining_seconds: 161
} as const;
const approvedGroupAQueueEntry = {
  ...groupAQueueEntry,
  result: { state: "approved", reason: null },
  remaining_seconds: null
} as const;
const groupBQueueEntry = {
  ...groupAQueueEntry,
  id: `${groupBID}:528106775:queue-nonce`,
  user: "@queue_group_b",
  group_key: groupBID
} as const;

type QueueReadHandler = (route: Route, chatID: string) => Promise<void>;
type QueueReleaseHandler = (route: Route) => Promise<void>;

async function mockQueueTransport(
  page: Page,
  readQueue: QueueReadHandler,
  releaseQueue: QueueReleaseHandler
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
          csrf_token: "queue-stale-csrf"
        })
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: groupAID }, { id: groupBID }] })
      });
      return;
    }
    if (
      (path === `/api/chats/${groupAID}/queue` || path === `/api/chats/${groupBID}/queue`) &&
      request.method() === "GET"
    ) {
      const chatID =
        path === `/api/chats/${groupAID}/queue` ? groupAID : groupBID;
      await readQueue(route, chatID);
      return;
    }
    if (
      path === `/api/chats/${groupAID}/queue/${groupAQueueEntry.id}` &&
      request.method() === "POST"
    ) {
      await releaseQueue(route);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

function queueRow(page: Page, user: string) {
  return page.locator("[data-queue-row]", { hasText: user });
}

async function selectGroupB(page: Page): Promise<void> {
  const groupSwitcher = page.getByRole("button", { name: "当前群" });
  await selectAppOption(groupSwitcher, groupBID);
  await expect(page).toHaveURL(new RegExp(`/queue\\?group=${groupBID}$`));
  await expect(groupSwitcher).toContainText(groupBID);
}

test("queue discards group A's delayed read after the visible group switcher selects group B", async ({
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

  await mockQueueTransport(
    page,
    async (route, chatID) => {
      if (chatID === groupAID) {
        markAReadRequested();
        await aReadResponse;
        await route.fulfill({
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ items: [groupAQueueEntry] })
        });
        return;
      }
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [groupBQueueEntry] })
      });
    },
    async () => {
      throw new Error("Release must not run while a delayed queue read is pending");
    }
  );

  await page.goto(`/queue?group=${groupAID}`, { waitUntil: "domcontentloaded" });
  await aReadRequested;
  await selectGroupB(page);
  await expect(queueRow(page, "@queue_group_b")).toBeVisible();

  const staleReadResponse = page.waitForResponse(
    (response) =>
      decodeURIComponent(new URL(response.url()).pathname) ===
        `/api/chats/${groupAID}/queue` && response.request().method() === "GET"
  );
  resolveARead();
  await staleReadResponse;
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
  await expect(queueRow(page, "@queue_group_a")).toHaveCount(0);
  await expect(queueRow(page, "@queue_group_b")).toBeVisible();
});

test("queue discards group A's delayed release after the visible group switcher selects group B", async ({
  page
}) => {
  let resolveRelease!: () => void;
  const releaseResponse = new Promise<void>((resolve) => {
    resolveRelease = resolve;
  });
  let markReleaseRequested!: () => void;
  const releaseRequested = new Promise<void>((resolve) => {
    markReleaseRequested = resolve;
  });

  await mockQueueTransport(
    page,
    async (route, chatID) => {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          items: [chatID === groupAID ? groupAQueueEntry : groupBQueueEntry]
        })
      });
    },
    async (route) => {
      const request = route.request();
      expect(request.headers()["x-csrf-token"]).toBe("queue-stale-csrf");
      expect(JSON.parse(request.postData() ?? "")).toEqual({
        expected: { state: "pending", reason: "" },
        result: { state: "approved", reason: "" }
      });
      markReleaseRequested();
      await releaseResponse;
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(approvedGroupAQueueEntry)
      });
    }
  );

  await page.goto(`/queue?group=${groupAID}`);
  const groupARow = queueRow(page, "@queue_group_a");
  await expect(groupARow).toBeVisible();
  await groupARow.getByRole("button", { name: "放行 @queue_group_a" }).click();
  await releaseRequested;
  await expect(groupARow).toHaveAttribute("data-action-state", "pending");

  await selectGroupB(page);
  await expect(queueRow(page, "@queue_group_b")).toBeVisible();

  const staleReleaseResponse = page.waitForResponse(
    (response) =>
      decodeURIComponent(new URL(response.url()).pathname) ===
        `/api/chats/${groupAID}/queue/${groupAQueueEntry.id}` &&
      response.request().method() === "POST"
  );
  resolveRelease();
  await staleReleaseResponse;
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));

  const groupBRow = queueRow(page, "@queue_group_b");
  await expect(groupARow).toHaveCount(0);
  await expect(groupBRow).toHaveAttribute("data-result", "pending");
  await expect(page.locator("[data-queue-feedback]")).toHaveCount(0);
});

test("queue sends one release for a forced second click and shows the confirmed result", async ({
  page
}) => {
  let releaseRequests = 0;
  let resolveRelease!: () => void;
  const releaseResponse = new Promise<void>((resolve) => {
    resolveRelease = resolve;
  });
  let markReleaseRequested!: () => void;
  const releaseRequested = new Promise<void>((resolve) => {
    markReleaseRequested = resolve;
  });

  await mockQueueTransport(
    page,
    async (route) => {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [groupAQueueEntry] })
      });
    },
    async (route) => {
      releaseRequests += 1;
      const request = route.request();
      expect(request.headers()["x-csrf-token"]).toBe("queue-stale-csrf");
      expect(JSON.parse(request.postData() ?? "")).toEqual({
        expected: { state: "pending", reason: "" },
        result: { state: "approved", reason: "" }
      });
      markReleaseRequested();
      await releaseResponse;
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(approvedGroupAQueueEntry)
      });
    }
  );

  await page.goto(`/queue?group=${groupAID}`);
  const row = queueRow(page, "@queue_group_a");
  const action = row.locator("[data-queue-action-id='release']");
  await expect(row).toHaveAttribute("data-result", "pending");
  await action.click();
  await releaseRequested;
  await expect(action).toHaveAttribute("aria-disabled", "true");
  await action.click({ force: true });
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
  expect(releaseRequests).toBe(1);

  const releaseSuccessResponse = page.waitForResponse(
    (response) =>
      decodeURIComponent(new URL(response.url()).pathname) ===
        `/api/chats/${groupAID}/queue/${groupAQueueEntry.id}` &&
      response.request().method() === "POST"
  );
  resolveRelease();
  await releaseSuccessResponse;
  await expect(row).toHaveAttribute("data-result", "approved");
  await expect(row.locator("[data-queue-action-id='release']")).toHaveCount(0);
  await expect(page.locator("[data-queue-feedback]")).toContainText(
    "已放行 @queue_group_a"
  );
  expect(releaseRequests).toBe(1);
});

test("an interrupted release provides the queue reload it names", async ({ page }) => {
  let queueReads = 0;
  await mockQueueTransport(
    page,
    async (route) => {
      queueReads += 1;
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [groupAQueueEntry] })
      });
    },
    async (route) => route.abort("failed")
  );
  await page.goto(`/queue?group=${groupAID}`);

  await queueRow(page, "@queue_group_a").getByRole("button").click();
  const feedback = page.locator("[data-queue-feedback]");
  await expect(feedback).toContainText("连接已中断");
  await feedback.getByRole("button", { name: "重新读取" }).click();
  await expect.poll(() => queueReads).toBe(2);
  await expect(feedback).toHaveCount(0);
});
