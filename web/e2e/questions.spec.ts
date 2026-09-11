import { expect, test, type Locator, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1009000010001";
const actorID = "741928306";

type SettingSource = "factory default" | "user file" | "chat override";
type QuestionLanguage = "zh" | "zh-Hant" | "en";

type SourcedSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

type Question = Readonly<{
  q: string;
  options: readonly string[];
  answer: number;
}>;

type ShortQuestion = Readonly<{
  q: string;
  answers: readonly string[];
}>;

type SettingsResponse = Readonly<{
  revision: number;
  questions: SourcedSetting<readonly Question[]>;
  fallback_questions: SourcedSetting<readonly ShortQuestion[]>;
  fallback_builtin: SourcedSetting<boolean>;
  lang: SourcedSetting<QuestionLanguage>;
}>;

type SettingsReadHandler = (route: Route, requestNumber: number) => Promise<void>;
type SettingsPatchHandler = (route: Route) => Promise<void>;

type Deferred = Readonly<{
  promise: Promise<void>;
  resolve: () => void;
}>;

const firstQuestion: Question = {
  q: "Which package manager belongs to Gentoo?",
  options: ["Portage", "apt"],
  answer: 0
};

const secondQuestion: Question = {
  q: "Which command shows the kernel release?",
  options: ["uname -r", "hostname"],
  answer: 0
};

function sourced<T>(value: T, source: SettingSource = "factory default"): SourcedSetting<T> {
  return { value, source };
}

function settingsResponse(overrides: Partial<SettingsResponse> = {}): SettingsResponse {
  return {
    revision: 7,
    questions: sourced([firstQuestion], "user file"),
    fallback_questions: sourced([{ q: "2 + 2 = ?", answers: ["4"] }]),
    fallback_builtin: sourced(true),
    lang: sourced("zh"),
    ...overrides
  };
}

function busyControlsSettings() {
  return {
    ...settingsResponse({
      fallback_questions: sourced([
        { q: "Name a Gentoo package manager", answers: ["Portage", "emerge"] }
      ]),
      fallback_builtin: sourced(false)
    }),
    delivery_mode: sourced("both"),
    verify_mode: sourced("kernel"),
    timeout_seconds: sourced(240),
    verify_max_fails: sourced(3),
    verify_retry_seconds: sourced(180),
    ban_seconds: sourced(0),
    mute_seconds: sourced(3600),
    verify_invited: sourced(true)
  };
}

function deferred(): Deferred {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
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
  readSettings: SettingsReadHandler,
  patchSettings: SettingsPatchHandler
): Promise<void> {
  let reads = 0;
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);
    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "manager" },
        expires_at: "2026-09-01T02:00:00Z",
        csrf_token: "questions-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID, title: "Gentoo-zh Community" }] });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "GET") {
      reads += 1;
      await readSettings(route, reads);
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "PATCH") {
      await patchSettings(route);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openQuestions(
  page: Page,
  readSettings: SettingsReadHandler,
  patchSettings: SettingsPatchHandler
): Promise<void> {
  await mockQuestionTransport(page, readSettings, patchSettings);
  await page.goto(`/questions?group=${selectedGroupID}`);
  await expect(page.locator("[data-questions-page]")).toHaveAttribute(
    "data-questions-state",
    "loaded"
  );
}

async function expectControlsFocusable(controls: Locator): Promise<void> {
  const count = await controls.count();
  expect(count).toBeGreaterThan(0);
  for (let index = 0; index < count; index += 1) {
    const control = controls.nth(index);
    await control.focus();
    await expect(control).toBeFocused();
  }
}

async function exerciseBusyQuestionControls(
  page: Page,
  patchRequested: Promise<void>,
  patchCalls: () => number
): Promise<void> {
  await page.getByLabel("题面").first().fill("Edited while testing the busy state");
  const addQuestion = page.getByRole("button", { name: "添加选择题" });
  const editor = page.locator("form").filter({ has: addQuestion });
  await addQuestion.focus();
  await editor.evaluate((form) => {
    (form as HTMLFormElement).requestSubmit();
  });
  await patchRequested;

  await expect(addQuestion).toBeFocused();
  await expect(addQuestion).toHaveAttribute("aria-disabled", "true");
  const controls = editor.locator(
    [
      "#questions-language-select",
      "[data-question-list-heading] > button",
      "[data-fallback-mode-options] > button",
      "[data-question-bank-editor] button",
      "[data-question-bank-editor] input",
      "[data-question-bank-editor] textarea",
      "[data-fallback-question-editor] button",
      "[data-fallback-question-editor] input",
      "[data-fallback-question-editor] textarea"
    ].join(", ")
  );
  await expectControlsFocusable(controls);
  const editorTextControls = editor.locator(
    "[data-question-bank-editor] input, [data-question-bank-editor] textarea, " +
      "[data-fallback-question-editor] input, [data-fallback-question-editor] textarea"
  );
  expect(
    await editorTextControls.evaluateAll((fields) =>
      fields.every(
        (field) => (field as HTMLInputElement | HTMLTextAreaElement).readOnly
      )
    )
  ).toBe(true);
  const guardedButtons = editor.locator(
    [
      "[data-question-list-heading] > button",
      "[data-fallback-mode-options] > button",
      "[data-question-bank-editor] button",
      "[data-fallback-question-editor] button"
    ].join(", ")
  );
  expect(
    await guardedButtons.evaluateAll((buttons) =>
      buttons.every((button) => button.getAttribute("aria-disabled") === "true")
    )
  ).toBe(true);

  const questionCount = await page.locator("[data-question-bank-editor] [data-question-item]").count();
  const optionCount = await page.locator("[data-question-bank-editor] [data-question-option-row]").count();
  const answerCount = await page.locator("[data-fallback-question-editor] [data-fallback-answer-row]").count();
  await addQuestion.dispatchEvent("click");
  await page.getByRole("button", { name: "添加选项" }).dispatchEvent("click");
  await page.getByRole("button", { name: "添加答案" }).dispatchEvent("click");
  await page.locator("#questions-language-select").dispatchEvent("click");
  await page.getByRole("button", { name: "继承的部署默认题库" }).dispatchEvent("click");
  expect(await page.locator("[data-question-bank-editor] [data-question-item]").count()).toBe(questionCount);
  expect(await page.locator("[data-question-bank-editor] [data-question-option-row]").count()).toBe(optionCount);
  expect(await page.locator("[data-fallback-question-editor] [data-fallback-answer-row]").count()).toBe(answerCount);
  await expect(page.locator("#questions-language-select")).toHaveAttribute("aria-expanded", "false");
  await expect(page.locator("[data-fallback-question-editor]")).toBeVisible();

  await page.getByRole("button", { name: "正在保存…" }).dispatchEvent("click");
  await page.waitForTimeout(100);
  expect(patchCalls()).toBe(1);
}

async function exerciseBusyVerificationControls(
  page: Page,
  patchRequested: Promise<void>,
  patchCalls: () => number
): Promise<void> {
  await page.locator("#verification-timeout-seconds").fill("241");
  const focusedSelect = page.locator("#verification-mode");
  await focusedSelect.focus();
  await page.locator("[data-verification-form]").evaluate((form) => {
    (form as HTMLFormElement).requestSubmit();
  });
  await patchRequested;

  await expect(focusedSelect).toBeFocused();
  await expect(focusedSelect).toHaveAttribute("aria-disabled", "true");
  const controls = page.locator(
    "#verification-delivery-mode, #verification-mode, [data-verification-number], #verification-invited-members"
  );
  await expectControlsFocusable(controls);
  expect(
    await page.locator("[data-verification-number]").evaluateAll((inputs) =>
      inputs.every((input) => (input as HTMLInputElement).readOnly)
    )
  ).toBe(true);

  await focusedSelect.dispatchEvent("click");
  await expect(focusedSelect).toHaveAttribute("aria-expanded", "false");
  const invited = page.locator("#verification-invited-members");
  await invited.evaluate((input) => (input as HTMLInputElement).click());
  await expect(invited).toBeChecked();

  await page.getByRole("button", { name: "正在保存…" }).dispatchEvent("click");
  await page.waitForTimeout(100);
  expect(patchCalls()).toBe(2);
}

test("question bank adds and edits an item, then sends only the complete questions array with CSRF", async ({
  page
}) => {
  let markPatchRequested!: () => void;
  let releasePatch!: () => void;
  const patchRequested = new Promise<void>((resolve) => {
    markPatchRequested = resolve;
  });
  const patchResponse = new Promise<void>((resolve) => {
    releasePatch = resolve;
  });
  const savedQuestions: readonly Question[] = [
    { ...firstQuestion, q: "Which package manager does Gentoo use?" },
    { q: "Pick the rolling-release distribution", options: ["Debian", "Gentoo"], answer: 1 }
  ];

  await openQuestions(
    page,
    async (route) => fulfillJSON(route, settingsResponse()),
    async (route) => {
      expect(route.request().headers()["x-csrf-token"]).toBe("questions-csrf");
      expect(route.request().postDataJSON()).toEqual({
        expected_revision: 7,
        changes: { questions: savedQuestions }
      });
      markPatchRequested();
      await patchResponse;
      await fulfillJSON(
        route,
        settingsResponse({ revision: 8, questions: sourced(savedQuestions, "chat override") })
      );
    }
  );

  await page.getByLabel("题面").first().fill("Which package manager does Gentoo use?");
  await page.getByRole("button", { name: "添加选择题" }).click();
  const added = page.locator("[data-question-bank-editor] [data-question-item]").last();
  await added.getByLabel("题面").fill("Pick the rolling-release distribution");
  await added.getByLabel("选项 1", { exact: true }).fill("Debian");
  await added.getByLabel("选项 2", { exact: true }).fill("Gentoo");
  await added.getByRole("button", { name: "将选项 2 设为正确答案" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await patchRequested;
  await expect(page.getByRole("button", { name: "正在保存…" })).toHaveAttribute(
    "aria-disabled",
    "true"
  );

  releasePatch();
  await expect(page.locator('[data-questions-feedback="saved"]')).toContainText(
    "已保存题库设置"
  );
  await expect(page.locator("[data-questions-page]")).toHaveAttribute(
    "data-questions-state",
    "loaded"
  );
  await expect(page.getByText("来源：此群覆盖").first()).toBeVisible();
});
test("inherited fallback questions are visible in a read-only preview without creating an override", async ({
  page
}) => {
  let patchRequests = 0;
  const inheritedFallbackQuestions = [
    { q: "Name one neutral category", answers: ["books", "music"] }
  ];

  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          fallback_questions: sourced(inheritedFallbackQuestions),
          fallback_builtin: sourced(true)
        })
      ),
    async () => {
      patchRequests += 1;
      throw new Error("A read-only inherited preview must not submit a chat override");
    }
  );

  const preview = page.locator("[data-fallback-preview]");
  await expect(preview).toContainText("内置的中性示例题库");
  await expect(preview.locator("[data-fallback-preview-prompt]")).toHaveText(
    "Name one neutral category"
  );
  await expect(preview).toContainText("books");
  await expect(preview).toContainText("music");
  await expect(preview.locator("[data-fallback-preview-item]")).toHaveCount(1);
  await expect(preview.locator("button, input, textarea")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "保存更改" })).toBeDisabled();
  expect(patchRequests).toBe(0);
});
test("fallback custom questions restore to the saved inherited bank", async ({ page }) => {
  const inheritedFallback = {
    q: "Name one inherited topic",
    answers: ["books", "music"]
  };
  const customFallback = {
    q: "Name one custom topic",
    answers: ["games", "films"]
  };
  let patchCalls = 0;

  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          revision: 40,
          fallback_questions: sourced([inheritedFallback], "factory default"),
          fallback_builtin: sourced(true, "factory default")
        })
      ),
    async (route) => {
      patchCalls += 1;
      if (patchCalls === 1) {
        expect(route.request().postDataJSON()).toEqual({
          expected_revision: 40,
          changes: {
            fallback_questions: [customFallback],
            fallback_builtin: false
          }
        });
        await fulfillJSON(
          route,
          settingsResponse({
            revision: 41,
            fallback_questions: sourced([customFallback], "chat override"),
            fallback_builtin: sourced(false, "chat override")
          })
        );
        return;
      }
      expect(patchCalls).toBe(2);
      expect(route.request().postDataJSON()).toEqual({
        expected_revision: 41,
        changes: {
          fallback_questions: null,
          fallback_builtin: null
        }
      });
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 42,
          fallback_questions: sourced([inheritedFallback], "factory default"),
          fallback_builtin: sourced(true, "factory default")
        })
      );
    }
  );

  const fallback = page.getByRole("region", { name: "备用填空题", exact: true });
  await expect(fallback.locator("[data-fallback-preview-prompt]")).toHaveText(
    inheritedFallback.q
  );
  await fallback.getByRole("button", { name: "自定义备用题" }).click();
  await fallback.getByLabel("题面").fill(customFallback.q);
  await fallback.getByLabel("答案 1", { exact: true }).fill(customFallback.answers[0]);
  await fallback.getByLabel("答案 2", { exact: true }).fill(customFallback.answers[1]);
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(fallback.locator("[data-fallback-preview]")).toHaveCount(0);
  await expect(fallback).toContainText(customFallback.q);
  await expect(fallback).toContainText("备用题库来源：此群覆盖");

  await fallback.getByRole("button", { name: "恢复备用题继承值" }).click();
  await expect(fallback.locator("[data-fallback-preview-pending]")).toBeVisible();
  await expect(fallback.locator("[data-fallback-preview]")).toHaveCount(0);
  await expect(fallback).toContainText(customFallback.q);
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(fallback.locator("[data-fallback-preview-prompt]")).toHaveText(
    inheritedFallback.q
  );
  await expect(fallback).toContainText("备用题库来源：出厂默认");
  expect(patchCalls).toBe(2);
});

test("factory question bank overrides and restores the inherited factory bank", async ({ page }) => {
  const factoryQuestion: Question = {
    q: "Which activity fits this neutral group?",
    options: ["Reading", "Unrelated option"],
    answer: 0
  };
  const overriddenQuestion: Question = {
    ...factoryQuestion,
    q: "Which activity did this group choose?",
    options: ["Reading", "Music"],
    answer: 1
  };
  let patchCalls = 0;

  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          revision: 20,
          questions: sourced([factoryQuestion], "factory default")
        })
      ),
    async (route) => {
      patchCalls += 1;
      if (patchCalls === 1) {
        expect(route.request().postDataJSON()).toEqual({
          expected_revision: 20,
          changes: { questions: [overriddenQuestion] }
        });
        await fulfillJSON(
          route,
          settingsResponse({
            revision: 21,
            questions: sourced([overriddenQuestion], "chat override")
          })
        );
        return;
      }
      expect(patchCalls).toBe(2);
      expect(route.request().postDataJSON()).toEqual({
        expected_revision: 21,
        changes: { questions: null }
      });
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 22,
          questions: sourced([factoryQuestion], "factory default")
        })
      );
    }
  );

  const bank = page.getByRole("region", { name: "选择题", exact: true });
  const prompt = bank.getByLabel("题面").first();
  await expect(prompt).toHaveValue(factoryQuestion.q);
  await expect(bank).toContainText("来源：出厂默认");
  await prompt.fill(overriddenQuestion.q);
  await bank.getByLabel("选项 1", { exact: true }).fill(overriddenQuestion.options[0]);
  await bank.getByLabel("选项 2", { exact: true }).fill(overriddenQuestion.options[1]);
  await bank.getByRole("button", { name: "将选项 2 设为正确答案" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(prompt).toHaveValue(overriddenQuestion.q);
  await expect(bank).toContainText("来源：此群覆盖");

  await bank.getByRole("button", { name: "恢复继承值" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(prompt).toHaveValue(factoryQuestion.q);
  await expect(bank).toContainText("来源：出厂默认");
  expect(patchCalls).toBe(2);
});

test("configuration-file question bank overrides and restores the configuration bank", async ({ page }) => {
  const configuredQuestion: Question = {
    q: "Which topic does this configured group discuss?",
    options: ["Books", "Unrelated option"],
    answer: 0
  };
  const overriddenQuestion: Question = {
    ...configuredQuestion,
    q: "Which topic did this group choose?",
    options: ["Books", "Music"],
    answer: 1
  };
  const configuredFallback = {
    q: "Name one configured fallback topic",
    answers: ["books", "music"]
  };
  let patchCalls = 0;

  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          revision: 30,
          questions: sourced([configuredQuestion], "user file"),
          fallback_questions: sourced([configuredFallback], "user file"),
          fallback_builtin: sourced(true, "user file")
        })
      ),
    async (route) => {
      patchCalls += 1;
      if (patchCalls === 1) {
        expect(route.request().postDataJSON()).toEqual({
          expected_revision: 30,
          changes: { questions: [overriddenQuestion] }
        });
        await fulfillJSON(
          route,
          settingsResponse({
            revision: 31,
            questions: sourced([overriddenQuestion], "chat override"),
            fallback_questions: sourced([configuredFallback], "user file"),
            fallback_builtin: sourced(true, "user file")
          })
        );
        return;
      }
      expect(patchCalls).toBe(2);
      expect(route.request().postDataJSON()).toEqual({
        expected_revision: 31,
        changes: { questions: null }
      });
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 32,
          questions: sourced([configuredQuestion], "user file"),
          fallback_questions: sourced([configuredFallback], "user file"),
          fallback_builtin: sourced(true, "user file")
        })
      );
    }
  );

  const fallbackPreview = page.locator("[data-fallback-preview]");
  await expect(fallbackPreview).toContainText("Name one configured fallback topic");
  await expect(page.locator("[data-fallback-sources]")).toContainText("备用题库来源：配置文件");

  const bank = page.getByRole("region", { name: "选择题", exact: true });
  const prompt = bank.getByLabel("题面").first();
  await expect(prompt).toHaveValue(configuredQuestion.q);
  await expect(bank).toContainText("来源：配置文件");
  await prompt.fill(overriddenQuestion.q);
  await bank.getByLabel("选项 1", { exact: true }).fill(overriddenQuestion.options[0]);
  await bank.getByLabel("选项 2", { exact: true }).fill(overriddenQuestion.options[1]);
  await bank.getByRole("button", { name: "将选项 2 设为正确答案" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(prompt).toHaveValue(overriddenQuestion.q);
  await expect(bank).toContainText("来源：此群覆盖");

  await bank.getByRole("button", { name: "恢复继承值" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(prompt).toHaveValue(configuredQuestion.q);
  await expect(bank).toContainText("来源：配置文件");
  expect(patchCalls).toBe(2);
});


test("busy question and verification controls keep focus and ignore interaction", async ({ page }) => {
  const questionPatchRequested = deferred();
  const questionPatchResponse = deferred();
  const verificationPatchRequested = deferred();
  const verificationPatchResponse = deferred();
  let patchCalls = 0;
  let currentSettings = busyControlsSettings();

  await openQuestions(
    page,
    async (route) => fulfillJSON(route, currentSettings),
    async (route) => {
      patchCalls += 1;
      const requested =
        patchCalls === 1
          ? questionPatchRequested
          : patchCalls === 2
            ? verificationPatchRequested
            : undefined;
      const response =
        patchCalls === 1
          ? questionPatchResponse
          : patchCalls === 2
            ? verificationPatchResponse
            : undefined;
      if (!requested || !response) {
        throw new Error(`Unexpected extra settings PATCH request ${patchCalls}`);
      }

      requested.resolve();
      await response.promise;
      currentSettings = { ...currentSettings, revision: currentSettings.revision + 1 };
      await fulfillJSON(route, currentSettings);
    }
  );

  await exerciseBusyQuestionControls(
    page,
    questionPatchRequested.promise,
    () => patchCalls
  );
  questionPatchResponse.resolve();
  await expect(page.locator('[data-questions-feedback="saved"]')).toBeVisible();

  await page.goto(`/verification?group=${selectedGroupID}`);
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "loaded"
  );
  await exerciseBusyVerificationControls(
    page,
    verificationPatchRequested.promise,
    () => patchCalls
  );
  verificationPatchResponse.resolve();
  await expect(page.locator("[data-verification-feedback]")).toContainText(
    "已保存验证设置"
  );
});

test("question deletion requires confirmation and language restoration writes null", async ({ page }) => {
  let requestBody: unknown;
  const remainingQuestions = [secondQuestion];
  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          revision: 9,
          questions: sourced([firstQuestion, secondQuestion], "chat override"),
          lang: sourced("en", "chat override")
        })
      ),
    async (route) => {
      requestBody = route.request().postDataJSON();
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 10,
          questions: sourced(remainingQuestions, "chat override"),
          lang: sourced("zh")
        })
      );
    }
  );

  const firstDelete = page.locator("[data-question-bank-editor] [data-question-item]").first().getByRole("button", {
    name: "删除题目"
  });
  let dismissedMessage: string | undefined;
  page.once("dialog", async (dialog) => {
    dismissedMessage = dialog.message();
    await dialog.dismiss();
  });
  await firstDelete.click();
  expect(dismissedMessage).toContain("删除选择题 1");
  await expect(page.locator("[data-question-bank-editor] [data-question-item]")).toHaveCount(2);

  page.once("dialog", (dialog) => dialog.accept());
  await firstDelete.click();
  await expect(page.locator("[data-question-bank-editor] [data-question-item]")).toHaveCount(1);
  await page
    .locator('[data-question-setting="lang"]')
    .getByRole("button", { name: "恢复继承值" })
    .click();
  await page.getByRole("button", { name: "保存更改" }).click();

  await expect(page.locator('[data-questions-feedback="saved"]')).toBeVisible();
  expect(requestBody).toEqual({
    expected_revision: 9,
    changes: { questions: remainingQuestions, lang: null }
  });
  await expect(page.locator("#questions-language-select")).toHaveAttribute("data-value", "zh");
});

test("switching custom fallback questions to built-ins clears the array override", async ({ page }) => {
  let requestBody: unknown;
  const fallbackQuestions = [{ q: "Name a Gentoo package manager", answers: ["Portage", "emerge"] }];
  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          fallback_questions: sourced(fallbackQuestions, "chat override"),
          fallback_builtin: sourced(false, "chat override")
        })
      ),
    async (route) => {
      requestBody = route.request().postDataJSON();
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 8,
          fallback_builtin: sourced(true, "chat override")
        })
      );
    }
  );

  await page.getByRole("button", { name: "继承的部署默认题库" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(page.locator('[data-questions-feedback="saved"]')).toBeVisible();
  expect(requestBody).toEqual({
    expected_revision: 7,
    changes: { fallback_builtin: true, fallback_questions: null }
  });
  await expect(page.locator("[data-fallback-preview-prompt]")).toHaveText("2 + 2 = ?");
});

test("question validation rejects an empty prompt and a blank custom fallback answer before PATCH", async ({
  page
}) => {
  let patchRequests = 0;
  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          fallback_questions: sourced([{ q: "Package manager?", answers: ["Portage"] }]),
          fallback_builtin: sourced(false)
        })
      ),
    async () => {
      patchRequests += 1;
      throw new Error("Client validation must prevent this PATCH request");
    }
  );

  await page.getByLabel("题面").first().fill("");
  await page.getByLabel("答案 1", { exact: true }).fill("   ");
  await page.getByRole("button", { name: "保存更改" }).click();

  await expect(page.getByLabel("题面").first()).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByText("请输入题面。")).toBeVisible();
  await expect(page.getByText("可接受答案不能留空。")).toBeVisible();
  expect(patchRequests).toBe(0);
});

test("question conflict reloads the newer arrays and explains why different-item edits collide", async ({
  page
}) => {
  let reads = 0;
  let markLatestSettingsSettled!: () => void;
  const latestSettingsSettled = new Promise<void>((resolve) => {
    markLatestSettingsSettled = resolve;
  });
  const latestQuestions = [firstQuestion, { ...secondQuestion, q: "Changed by another administrator" }];

  await openQuestions(
    page,
    async (route, requestNumber) => {
      reads = requestNumber;
      if (requestNumber === 1) {
        await fulfillJSON(
          route,
          settingsResponse({ revision: 12, questions: sourced([firstQuestion, secondQuestion]) })
        );
        return;
      }
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 13,
          questions: sourced(latestQuestions, "chat override")
        })
      );
      markLatestSettingsSettled();
    },
    async (route) => {
      expect(route.request().postDataJSON()).toEqual({
        expected_revision: 12,
        changes: {
          questions: [
            { ...firstQuestion, q: "Edited in this draft" },
            secondQuestion
          ]
        }
      });
      await fulfillJSON(route, { error: { code: "settings_conflict" } }, 409);
    }
  );

  await page.getByLabel("题面").first().fill("Edited in this draft");
  await page.getByRole("button", { name: "保存更改" }).click();
  await latestSettingsSettled;
  await expect(page.locator("[data-questions-page]")).toHaveAttribute(
    "data-questions-state",
    "loaded"
  );
  await expect(page.getByLabel("题面").first()).toHaveValue(firstQuestion.q);
  await expect(page.getByLabel("题面").nth(1)).toHaveValue("Changed by another administrator");
  const conflict = page.locator('[data-questions-feedback="conflict"]');
  await expect(conflict).toContainText("另一位管理员");
  await expect(conflict).toContainText("不同题目");
  await expect(conflict).toContainText("版本冲突");
  expect(reads).toBe(2);
});

test("switching to custom fallback preserves the displayed bank over a hidden file baseline", async ({ page }) => {
  const displayedBank: ShortQuestion[] = [{ q: "2 + 2 = ?", answers: ["4"] }];
  const fileBank: ShortQuestion[] = [{ q: "How many minutes are in an hour?", answers: ["60"] }];
  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(route, settingsResponse({
        fallback_builtin: sourced(true, "chat override"),
        fallback_questions: sourced(displayedBank, "factory default")
      })),
    async (route) => {
      const changes = route.request().postDataJSON().changes as {
        fallback_builtin?: boolean;
        fallback_questions?: ShortQuestion[] | null;
      };
      await fulfillJSON(route, settingsResponse({
        revision: 8,
        fallback_builtin: sourced(changes.fallback_builtin ?? true, "user file"),
        fallback_questions: sourced(
          changes.fallback_questions ?? fileBank,
          changes.fallback_questions ? "chat override" : "user file"
        )
      }));
    }
  );

  const fallback = page.getByRole("region", { name: "备用填空题", exact: true });
  await expect(fallback.locator("[data-fallback-preview-prompt]")).toHaveText(displayedBank[0].q);
  await fallback.getByRole("button", { name: "自定义备用题", exact: true }).click();
  await expect(fallback.getByLabel("题面")).toHaveValue(displayedBank[0].q);
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(page.locator('[data-questions-feedback="saved"]')).toBeVisible();
  await expect(fallback.getByLabel("题面")).toHaveValue(displayedBank[0].q);
  await expect(fallback.getByLabel("答案 1", { exact: true })).toHaveValue(displayedBank[0].answers[0]);
  await expect(fallback).toContainText("题库来源：此群覆盖");
});
