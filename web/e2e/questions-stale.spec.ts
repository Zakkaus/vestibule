import { expect, test, type Page, type Route } from "@playwright/test";
import { selectAppOption } from "./app-select";

const groupAID = "-1009000000005";
const groupBID = "-1009000000006";
const actorID = "741928306";

type SettingSource = "factory default" | "user file" | "chat override";
type Question = Readonly<{
  q: string;
  options: readonly string[];
  answer: number;
}>;

const groupAQuestion: Question = {
  q: "Which package manager belongs to group A?",
  options: ["Portage", "apt"],
  answer: 0
};
const groupBQuestion: Question = {
  q: "Which package manager belongs to group B?",
  options: ["emerge", "dnf"],
  answer: 0
};

function sourced<T>(value: T, source: SettingSource = "factory default") {
  return { value, source };
}

function settingsPayload(question: Question, revision: number) {
  return {
    revision,
    questions: sourced([question], "chat override"),
    fallback_questions: sourced([]),
    fallback_builtin: sourced(true),
    lang: sourced("zh" as const)
  };
}

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockQuestionTransport(
  page: Page,
  readSettings: (route: Route, groupID: string) => Promise<void>,
  patchSettings: (route: Route) => Promise<void>
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "manager" },
        expires_at: "2026-09-01T02:00:00Z",
        csrf_token: "questions-stale-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: groupAID }, { id: groupBID }] });
      return;
    }
    if (
      (path === `/api/chats/${groupAID}/settings` || path === `/api/chats/${groupBID}/settings`) &&
      request.method() === "GET"
    ) {
      await readSettings(route, path.includes(groupAID) ? groupAID : groupBID);
      return;
    }
    if (path === `/api/chats/${groupAID}/settings` && request.method() === "PATCH") {
      await patchSettings(route);
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

test("questions discard a previous group's delayed settings response", async ({ page }) => {
  let markSettingsRequested!: () => void;
  let releaseSettings!: () => void;
  const settingsRequested = new Promise<void>((resolve) => {
    markSettingsRequested = resolve;
  });
  const firstSettingsResponse = new Promise<void>((resolve) => {
    releaseSettings = resolve;
  });
  let markSettingsResponseSettled!: () => void;
  const settingsResponseSettled = new Promise<void>((resolve) => {
    markSettingsResponseSettled = resolve;
  });

  await mockQuestionTransport(
    page,
    async (route, groupID) => {
      if (groupID === groupAID) {
        markSettingsRequested();
        await firstSettingsResponse;
        await fulfillJSON(route, settingsPayload(groupAQuestion, 7));
        markSettingsResponseSettled();
        return;
      }
      await fulfillJSON(route, settingsPayload(groupBQuestion, 8));
    },
    async () => {
      throw new Error("A delayed read must not write question settings");
    }
  );

  await page.goto(`/questions?group=${groupAID}`, { waitUntil: "domcontentloaded" });
  await settingsRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), groupBID);
  await expect(page).toHaveURL(new RegExp(`/questions\\?group=${groupBID}$`));
  await expect(page.getByLabel("题面").first()).toHaveValue(groupBQuestion.q);

  releaseSettings();
  await settingsResponseSettled;
  await expect(page.getByLabel("题面").first()).toHaveValue(groupBQuestion.q);
});

test("questions ignore a previous group's delayed settings save", async ({ page }) => {
  let markPatchRequested!: () => void;
  let releasePatch!: () => void;
  const patchRequested = new Promise<void>((resolve) => {
    markPatchRequested = resolve;
  });
  const patchResponse = new Promise<void>((resolve) => {
    releasePatch = resolve;
  });
  let markPatchResponseSettled!: () => void;
  const patchResponseSettled = new Promise<void>((resolve) => {
    markPatchResponseSettled = resolve;
  });

  await mockQuestionTransport(
    page,
    async (route, groupID) => {
      await fulfillJSON(
        route,
        groupID === groupAID ? settingsPayload(groupAQuestion, 7) : settingsPayload(groupBQuestion, 8)
      );
    },
    async (route) => {
      markPatchRequested();
      await patchResponse;
      await fulfillJSON(route, settingsPayload({ ...groupAQuestion, q: "Saved for group A" }, 8));
      markPatchResponseSettled();
    }
  );

  await page.goto(`/questions?group=${groupAID}`);
  await expect(page.locator("[data-questions-page]")).toHaveAttribute("data-questions-state", "loaded");
  await page.getByLabel("题面").first().fill("Saved for group A");
  await page.getByRole("button", { name: "保存更改" }).click();
  await patchRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), groupBID);
  await expect(page).toHaveURL(new RegExp(`/questions\\?group=${groupBID}$`));
  await expect(page.getByLabel("题面").first()).toHaveValue(groupBQuestion.q);

  releasePatch();
  await patchResponseSettled;
  await expect(page.getByLabel("题面").first()).toHaveValue(groupBQuestion.q);
});
