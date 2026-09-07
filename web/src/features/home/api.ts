import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";
import {
  deliveryModes,
  settingSources,
  verifyModes,
  type DeliveryMode,
  type SettingSource,
  type VerifyMode
} from "../verification/api";
import {
  questionLanguages,
  type QuestionLanguage
} from "../questions/api";

export type HomeSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

export type HomeSettings = Readonly<{
  revision: number;
  enabled: HomeSetting<boolean>;
  deliveryMode: HomeSetting<DeliveryMode>;
  verifyMode: HomeSetting<VerifyMode>;
  nameSpoiler: HomeSetting<boolean>;
  timeoutSeconds: HomeSetting<number>;
  verifyMaxFails: HomeSetting<number>;
  verifyRetrySeconds: HomeSetting<number>;
  banSeconds: HomeSetting<number>;
  muteSeconds: HomeSetting<number>;
  verifyInvited: HomeSetting<boolean>;
  questionCount: HomeSetting<number>;
  fallbackQuestionCount: HomeSetting<number>;
  fallbackBuiltin: HomeSetting<boolean>;
  questionLanguage: HomeSetting<QuestionLanguage>;
  trustedGroupCount: HomeSetting<number>;
  requiredChannelID: HomeSetting<number>;
  requiredChannelFailOpen: HomeSetting<boolean>;
  channelDisplay: HomeSetting<string>;
  channelInviteURL: HomeSetting<string>;
  channelWhitelistCount: HomeSetting<number>;
  antispamEnabled: HomeSetting<boolean>;
  warnLimit: HomeSetting<number>;
  adminLogChatID: HomeSetting<number>;
}>;

type VerificationHomeSettings = Pick<
  HomeSettings,
  | "enabled"
  | "deliveryMode"
  | "verifyMode"
  | "nameSpoiler"
  | "timeoutSeconds"
  | "verifyMaxFails"
  | "verifyRetrySeconds"
  | "banSeconds"
  | "muteSeconds"
  | "verifyInvited"
>;

type QuestionHomeSettings = Pick<
  HomeSettings,
  "questionCount" | "fallbackQuestionCount" | "fallbackBuiltin" | "questionLanguage"
>;

type BypassHomeSettings = Pick<
  HomeSettings,
  | "trustedGroupCount"
  | "requiredChannelID"
  | "requiredChannelFailOpen"
  | "channelDisplay"
  | "channelInviteURL"
  | "channelWhitelistCount"
>;

type ModerationHomeSettings = Pick<
  HomeSettings,
  "antispamEnabled" | "warnLimit" | "adminLogChatID"
>;

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

function safeIntegerFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) ? value : undefined;
}

function choiceFromPayload<T extends readonly string[]>(
  value: unknown,
  choices: T
): T[number] | undefined {
  return typeof value === "string" && choices.includes(value)
    ? (value as T[number])
    : undefined;
}

function verificationSettingsFromPayload(
  response: Readonly<Record<string, unknown>>
): VerificationHomeSettings | undefined {
  const enabled = settingFromPayload(response.enabled, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const deliveryMode = settingFromPayload(response.delivery_mode, (value) =>
    choiceFromPayload(value, deliveryModes)
  );
  const verifyMode = settingFromPayload(response.verify_mode, (value) =>
    choiceFromPayload(value, verifyModes)
  );
  const nameSpoiler = settingFromPayload(response.name_spoiler, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const timeoutSeconds = settingFromPayload(response.timeout_seconds, safeIntegerFromPayload);
  const verifyMaxFails = settingFromPayload(response.verify_max_fails, safeIntegerFromPayload);
  const verifyRetrySeconds = settingFromPayload(response.verify_retry_seconds, safeIntegerFromPayload);
  const banSeconds = settingFromPayload(response.ban_seconds, safeIntegerFromPayload);
  const muteSeconds = settingFromPayload(response.mute_seconds, safeIntegerFromPayload);
  const verifyInvited = settingFromPayload(response.verify_invited, (value) =>
    typeof value === "boolean" ? value : undefined
  );

  if (
    !enabled ||
    !deliveryMode ||
    !verifyMode ||
    !nameSpoiler ||
    !timeoutSeconds ||
    !verifyMaxFails ||
    !verifyRetrySeconds ||
    !banSeconds ||
    !muteSeconds ||
    !verifyInvited
  ) {
    return undefined;
  }

  return {
    enabled,
    deliveryMode,
    verifyMode,
    nameSpoiler,
    timeoutSeconds,
    verifyMaxFails,
    verifyRetrySeconds,
    banSeconds,
    muteSeconds,
    verifyInvited
  };
}

function questionSettingsFromPayload(
  response: Readonly<Record<string, unknown>>
): QuestionHomeSettings | undefined {
  const questionCount = settingFromPayload(response.questions, (value) =>
    Array.isArray(value) ? value.length : undefined
  );
  const fallbackQuestionCount = settingFromPayload(response.fallback_questions, (value) =>
    Array.isArray(value) ? value.length : undefined
  );
  const fallbackBuiltin = settingFromPayload(response.fallback_builtin, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const questionLanguage = settingFromPayload(response.lang, (value) =>
    choiceFromPayload(value, questionLanguages)
  );

  if (!questionCount || !fallbackQuestionCount || !fallbackBuiltin || !questionLanguage) {
    return undefined;
  }

  return { questionCount, fallbackQuestionCount, fallbackBuiltin, questionLanguage };
}

function bypassSettingsFromPayload(
  response: Readonly<Record<string, unknown>>
): BypassHomeSettings | undefined {
  const trustedGroupCount = settingFromPayload(response.trusted_member_group_ids, (value) =>
    Array.isArray(value) ? value.length : undefined
  );
  const requiredChannelID = settingFromPayload(response.required_channel_id, safeIntegerFromPayload);
  const requiredChannelFailOpen = settingFromPayload(response.required_channel_fail_open, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const channelDisplay = settingFromPayload(response.channel_display, (value) =>
    typeof value === "string" ? value : undefined
  );
  const channelInviteURL = settingFromPayload(response.channel_invite_url, (value) =>
    typeof value === "string" ? value : undefined
  );
  const channelWhitelistCount = settingFromPayload(response.channel_whitelist, (value) =>
    Array.isArray(value) ? value.length : undefined
  );

  if (
    !trustedGroupCount ||
    !requiredChannelID ||
    !requiredChannelFailOpen ||
    !channelDisplay ||
    !channelInviteURL ||
    !channelWhitelistCount
  ) {
    return undefined;
  }

  return {
    trustedGroupCount,
    requiredChannelID,
    requiredChannelFailOpen,
    channelDisplay,
    channelInviteURL,
    channelWhitelistCount
  };
}

function moderationSettingsFromPayload(
  response: Readonly<Record<string, unknown>>
): ModerationHomeSettings | undefined {
  const antispamEnabled = settingFromPayload(response.antispam_enabled, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const warnLimit = settingFromPayload(response.warn_limit, safeIntegerFromPayload);
  const adminLogChatID = settingFromPayload(response.admin_log_chat_id, safeIntegerFromPayload);

  if (!antispamEnabled || !warnLimit || !adminLogChatID) {
    return undefined;
  }

  return { antispamEnabled, warnLimit, adminLogChatID };
}

export function homeSettingsFromPayload(payload: unknown): HomeSettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const revision = safeIntegerFromPayload(response.revision);
  const verification = verificationSettingsFromPayload(response);
  const questions = questionSettingsFromPayload(response);
  const bypass = bypassSettingsFromPayload(response);
  const moderation = moderationSettingsFromPayload(response);
  if (
    revision === undefined ||
    revision < 0 ||
    !verification ||
    !questions ||
    !bypass ||
    !moderation
  ) {
    return undefined;
  }

  return { revision, ...verification, ...questions, ...bypass, ...moderation };
}

export function loadHomeSettings(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<HomeSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    parse: homeSettingsFromPayload
  });
}
