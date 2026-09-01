import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export const settingSources = ["factory default", "user file", "chat override"] as const;

export type SettingSource = (typeof settingSources)[number];

export type SettingValue<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

export type ModerationSettings = Readonly<{
  revision: number;
  warnLimit: SettingValue<number>;
  antispamEnabled: SettingValue<boolean>;
  adminLogChatID: SettingValue<number>;
}>;

export type ModerationSettingsChanges = Readonly<
  Partial<{
    warn_limit: number | null;
    antispam_enabled: boolean | null;
    admin_log_chat_id: number | null;
  }>
>;

type PayloadParser<T> = (value: unknown) => T | undefined;

function sourceFromPayload(value: unknown): SettingSource | undefined {
  return typeof value === "string" && settingSources.includes(value as SettingSource)
    ? (value as SettingSource)
    : undefined;
}

function settingFromPayload<T>(
  payload: unknown,
  parseValue: PayloadParser<T>
): SettingValue<T> | undefined {
  const field = objectFromPayload(payload);
  if (!field) {
    return undefined;
  }

  const value = parseValue(field.value);
  const source = sourceFromPayload(field.source);
  return value === undefined || source === undefined ? undefined : { value, source };
}

function safeIntegerFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) ? value : undefined;
}

function moderationSettingsFromPayload(payload: unknown): ModerationSettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const revision = safeIntegerFromPayload(response.revision);
  const warnLimit = settingFromPayload(response.warn_limit, safeIntegerFromPayload);
  const antispamEnabled = settingFromPayload(
    response.antispam_enabled,
    (value) => (typeof value === "boolean" ? value : undefined)
  );
  const adminLogChatID = settingFromPayload(response.admin_log_chat_id, safeIntegerFromPayload);
  if (
    revision === undefined ||
    revision < 0 ||
    !warnLimit ||
    !antispamEnabled ||
    !adminLogChatID
  ) {
    return undefined;
  }

  return { revision, warnLimit, antispamEnabled, adminLogChatID };
}

export function loadModerationSettings(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<ModerationSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    parse: moderationSettingsFromPayload
  });
}

export function saveModerationSettings(
  transport: ApiTransport,
  chatID: string,
  expectedRevision: number,
  changes: ModerationSettingsChanges
): Promise<ApiResult<ModerationSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    method: "PATCH",
    body: {
      expected_revision: expectedRevision,
      changes
    },
    parse: moderationSettingsFromPayload
  });
}
