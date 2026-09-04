import { expect, test, type Page, type Route } from "@playwright/test";

const actorID = "9000000501";
const groupID = "-1009000000502";
const csrfToken = "version-csrf-token";

type ConsoleRole = "manager" | "operator";
type APIHandler = (route: Route, observations: VersionObservations) => Promise<void>;

type VersionObservations = {
  statusRequests: number;
  releaseRequests: number;
  upgradeRequests: number;
  upgradeBodies: unknown[];
  upgradeCSRF: string[];
};

type VersionMocks = Readonly<{
  role: ConsoleRole;
  status?: APIHandler;
  release?: APIHandler;
  upgrade?: APIHandler;
}>;

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

function versionStatus(unitAvailable: boolean, lastResult: unknown = null): unknown {
  return {
    version: "v5.1.0",
    replacement: {
      unit_available: unitAvailable,
      last_result: lastResult
    }
  };
}

function latestRelease(rollback: Readonly<Record<string, unknown>>): unknown {
  return {
    version: "v5.2.0",
    url: "https://github.com/Zakkaus/vestibule/releases/tag/v5.2.0",
    notes: "Safer replacement\n\nReview the migration notes.",
    published_at: "2026-09-01T00:00:00Z",
    update_available: true,
    rollback
  };
}

const safeRollback = {
  available: true,
  reason: "",
  target_schema_version: 2,
  retained_schema_version: 2,
  minimum_rollback_schema_version: 1
} as const;

const blockedRollback = {
  available: false,
  reason: "schema_incompatible",
  target_schema_version: 3,
  retained_schema_version: 2,
  minimum_rollback_schema_version: 3
} as const;

async function mockVersionTransport(page: Page, mocks: VersionMocks): Promise<VersionObservations> {
  const observations: VersionObservations = {
    statusRequests: 0,
    releaseRequests: 0,
    upgradeRequests: 0,
    upgradeBodies: [],
    upgradeCSRF: []
  };
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);
    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: mocks.role },
        expires_at: "2026-09-02T12:00:00Z",
        csrf_token: csrfToken
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: groupID }] });
      return;
    }
    if (path === "/api/status" && request.method() === "GET" && mocks.status) {
      observations.statusRequests += 1;
      await mocks.status(route, observations);
      return;
    }
    if (path === "/api/status/release" && request.method() === "GET" && mocks.release) {
      observations.releaseRequests += 1;
      await mocks.release(route, observations);
      return;
    }
    if (path === "/api/status/upgrade" && request.method() === "POST" && mocks.upgrade) {
      observations.upgradeRequests += 1;
      observations.upgradeBodies.push(request.postDataJSON() as unknown);
      observations.upgradeCSRF.push(request.headers()["x-csrf-token"] ?? "");
      await mocks.upgrade(route, observations);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
  return observations;
}

test("version screen hides from group managers and performs no instance-status request", async ({ page }) => {
  const observations = await mockVersionTransport(page, { role: "manager" });
  await page.goto("/version");

  await expect(page).toHaveURL(/\/groups$/);
  await expect(page.getByRole("link", { name: "版本", exact: true })).toHaveCount(0);
  expect(observations.statusRequests).toBe(0);
  expect(observations.releaseRequests).toBe(0);
  expect(observations.upgradeRequests).toBe(0);
});

test("version screen with no host unit never renders an upgrade button", async ({ page }) => {
  const observations = await mockVersionTransport(page, {
    role: "operator",
    status: async (route) => fulfillJSON(route, versionStatus(false)),
    release: async (route) => fulfillJSON(route, latestRelease(safeRollback))
  });
  await page.goto("/version");
  const screen = page.locator("[data-version-page]");
  await expect(screen.locator("[data-version-current]")).toHaveText("v5.1.0");
  expect(observations.releaseRequests).toBe(0);

  await screen.getByRole("button", { name: "检查更新" }).click();

  await expect(screen.locator("[data-version-latest]")).toHaveText("v5.2.0");
  await expect(screen.locator("[data-version-action=\"upgrade\"]")).toHaveCount(0);
  await expect(screen.locator("[data-version-manual-upgrade]")).toBeVisible();
  await expect(screen.locator("[data-version-manual-upgrade]")).toContainText(
    "并非应用故障"
  );
  await expect(screen.locator("[data-version-manual-image]")).toHaveText(
    "VESTIBULE_APP_IMAGE=ghcr.io/zakkaus/vestibule:v5.2.0"
  );
  await expect(screen.locator("[data-version-manual-command]")).toContainText(
    "docker compose up -d --no-deps app"
  );
});

test("version screen explains an incompatible rollback with schema versions", async ({ page }) => {
  await mockVersionTransport(page, {
    role: "operator",
    status: async (route) => fulfillJSON(route, versionStatus(true)),
    release: async (route) => fulfillJSON(route, latestRelease(blockedRollback))
  });
  await page.goto("/version");
  const screen = page.locator("[data-version-page]");
  await screen.getByRole("button", { name: "检查更新" }).click();

  await expect(screen.locator('[data-version-rollback="blocked"]')).toContainText(
    "目标 schema v3 要求保留版本至少支持 schema v3。当前保留版本只支持 schema v2"
  );
  await expect(screen.locator("[data-version-action=\"upgrade\"]")).toHaveCount(0);
});

test("version screen confirms one upgrade request and resumes through the host result", async ({ page }) => {
  let upgradeAccepted = false;
  let reportApplied = false;
  const observations = await mockVersionTransport(page, {
    role: "operator",
    status: async (route) => fulfillJSON(route, versionStatus(true, upgradeAccepted && reportApplied ? {
      status: "applied",
      requested_version: "v5.2.0",
      reason: "complete"
    } : null)),
    release: async (route) => fulfillJSON(route, latestRelease(safeRollback)),
    upgrade: async (route) => {
      const { promise, resolve } = Promise.withResolvers<void>();
      setTimeout(resolve, 100);
      await promise;
      upgradeAccepted = true;
      await fulfillJSON(route, { status: "requested" }, 202);
    }
  });
  await page.goto("/version");
  const screen = page.locator("[data-version-page]");
  await screen.getByRole("button", { name: "检查更新" }).click();
  await expect(screen.locator("[data-version-release-notes]")).toContainText("Safer replacement");
  await expect(screen.locator('[data-version-rollback="available"]')).toBeVisible();

  await screen.getByRole("button", { name: "升级到此版本" }).click();
  await screen.getByRole("button", { name: "确认升级" }).click();
  const busyUpgrade = screen.locator('[data-version-action="upgrade"]');
  await expect(busyUpgrade).toHaveAttribute("aria-disabled", "true");
  await busyUpgrade.click({ force: true });
  reportApplied = true;

  await expect(screen.locator('[data-version-upgrade-outcome="applied"]')).toBeVisible();
  expect(observations.upgradeRequests).toBe(1);
  expect(observations.upgradeBodies).toEqual([{ version: "v5.2.0" }]);
  expect(observations.upgradeCSRF).toEqual([csrfToken]);
  expect(observations.statusRequests).toBeGreaterThan(1);
});

test("release lookup failure keeps the current version visible and retries", async ({ page }) => {
  const observations = await mockVersionTransport(page, {
    role: "operator",
    status: async (route) => fulfillJSON(route, versionStatus(true)),
    release: async (route, current) => {
      if (current.releaseRequests === 1) {
        await fulfillJSON(route, { error: { code: "release_lookup_unavailable" } }, 503);
        return;
      }
      await fulfillJSON(route, latestRelease(safeRollback));
    }
  });
  await page.goto("/version");
  const screen = page.locator("[data-version-page]");
  await screen.getByRole("button", { name: "检查更新" }).click();

  await expect(screen.locator("[data-version-current]")).toHaveText("v5.1.0");
  await expect(screen.locator("[data-version-release-error]")).toContainText(
    "这不表示当前版本已是最新"
  );
  await screen.locator("[data-version-release-error]").getByRole("button", { name: "重试" }).click();
  await expect(screen.locator("[data-version-latest]")).toHaveText("v5.2.0");
  expect(observations.releaseRequests).toBe(2);
});

test("version screen shows the previous replacement failure reason", async ({ page }) => {
  await mockVersionTransport(page, {
    role: "operator",
    status: async (route) => fulfillJSON(route, versionStatus(true, {
      status: "rolled_back",
      requested_version: "v5.2.0",
      reason: "healthcheck_failed"
    }))
  });
  await page.goto("/version");
  const result = page.locator('[data-version-replacement-result="rolled_back"]');

  await expect(result).toContainText("新版本未通过存活或就绪探针");
  await expect(result).toContainText("healthcheck_failed");
});
