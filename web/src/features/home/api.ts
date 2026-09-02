import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";
import {
  settingSources,
  type SettingSource
} from "../verification/api";

export type HomeSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

export type HomeSettings = Readonly<{
  enabled: HomeSetting<boolean>;
  deliveryMode: HomeSetting<"group" | "dm" | "both">;
  verifyMode: HomeSetting<"kernel" | "quiz" | "mixed">;
  timeoutSeconds: HomeSetting<number>;
  questionCount: HomeSetting<number>;
  fallbackQuestionCount: HomeSetting<number>;
  trustedGroupCount: HomeSetting<number>;
  channelWhitelistCount: HomeSetting<number>;
  antispamEnabled: HomeSetting<boolean>;
  warnLimit: HomeSetting<number>;
}>;

function sourceFromPayload(value: unknown): SettingSource | undefined {
  return typeof value === "string" && settingSources.includes(value as SettingSource)
    ? (value as SettingSource)
    : undefined;
}

function settingFromPayload<T>(
  value: unknown,
  parseValue: (candidate: unknown) => T | undefined
): HomeSetting<T> | undefined {
  const setting = objectFromPayload(value);
  if (!setting) {
    return undefined;
  }

  const parsedValue = parseValue(setting.value);
  const source = sourceFromPayload(setting.source);
  return parsedValue === undefined || source === undefined
    ? undefined
    : { value: parsedValue, source };
}

function nonNegativeIntegerFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
    ? value
    : undefined;
}

function arrayLengthFromPayload(value: unknown): number | undefined {
  return Array.isArray(value) ? value.length : undefined;
}

function choiceFromPayload<T extends readonly string[]>(
  value: unknown,
  choices: T
): T[number] | undefined {
  return typeof value === "string" && choices.includes(value)
    ? (value as T[number])
    : undefined;
}

export function homeSettingsFromPayload(payload: unknown): HomeSettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const enabled = settingFromPayload(response.enabled, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const deliveryMode = settingFromPayload(response.delivery_mode, (value) =>
    choiceFromPayload(value, ["group", "dm", "both"] as const)
  );
  const verifyMode = settingFromPayload(response.verify_mode, (value) =>
    choiceFromPayload(value, ["kernel", "quiz", "mixed"] as const)
  );
  const timeoutSeconds = settingFromPayload(
    response.timeout_seconds,
    nonNegativeIntegerFromPayload
  );
  const questionCount = settingFromPayload(response.questions, arrayLengthFromPayload);
  const fallbackQuestionCount = settingFromPayload(
    response.fallback_questions,
    arrayLengthFromPayload
  );
  const trustedGroupCount = settingFromPayload(
    response.trusted_member_group_ids,
    arrayLengthFromPayload
  );
  const channelWhitelistCount = settingFromPayload(
    response.channel_whitelist,
    arrayLengthFromPayload
  );
  const antispamEnabled = settingFromPayload(response.antispam_enabled, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const warnLimit = settingFromPayload(response.warn_limit, nonNegativeIntegerFromPayload);

  if (
    !enabled ||
    !deliveryMode ||
    !verifyMode ||
    !timeoutSeconds ||
    !questionCount ||
    !fallbackQuestionCount ||
    !trustedGroupCount ||
    !channelWhitelistCount ||
    !antispamEnabled ||
    !warnLimit
  ) {
    return undefined;
  }

  return {
    enabled,
    deliveryMode,
    verifyMode,
    timeoutSeconds,
    questionCount,
    fallbackQuestionCount,
    trustedGroupCount,
    channelWhitelistCount,
    antispamEnabled,
    warnLimit
  };
}

export function loadHomeSettings(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<HomeSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    parse: homeSettingsFromPayload
  });
}
