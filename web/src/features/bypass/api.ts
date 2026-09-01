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

export type BypassSettings = Readonly<{
  revision: number;
  trustedMemberGroupIDs: SettingValue<readonly number[]>;
  requiredChannelID: SettingValue<number>;
  requiredChannelFailOpen: SettingValue<boolean>;
  channelDisplay: SettingValue<string>;
  channelInviteURL: SettingValue<string>;
  channelWhitelist: SettingValue<readonly number[]>;
}>;

export type BypassSettingsChanges = Readonly<
  Partial<{
    trusted_member_group_ids: readonly number[] | null;
    required_channel_id: number | null;
    required_channel_fail_open: boolean | null;
    channel_display: string | null;
    channel_invite_url: string | null;
    channel_whitelist: readonly number[] | null;
  }>
>;

type PayloadParser<T> = (value: unknown) => T | undefined;

function isSettingSource(value: unknown): value is SettingSource {
  return typeof value === "string" && settingSources.includes(value as SettingSource);
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
  const source = isSettingSource(field.source) ? field.source : undefined;
  return value === undefined || source === undefined ? undefined : { value, source };
}

function safeIntegerFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) ? value : undefined;
}

function integerListFromPayload(value: unknown): readonly number[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const parsed: number[] = [];
  for (const entry of value) {
    const parsedEntry = safeIntegerFromPayload(entry);
    if (parsedEntry === undefined) {
      return undefined;
    }
    parsed.push(parsedEntry);
  }
  return parsed;
}

export function bypassSettingsFromPayload(payload: unknown): BypassSettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const revision = safeIntegerFromPayload(response.revision);
  const trustedMemberGroupIDs = settingFromPayload(
    response.trusted_member_group_ids,
    integerListFromPayload
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
  const channelWhitelist = settingFromPayload(response.channel_whitelist, integerListFromPayload);

  if (
    revision === undefined ||
    revision < 0 ||
    !trustedMemberGroupIDs ||
    !requiredChannelID ||
    !requiredChannelFailOpen ||
    !channelDisplay ||
    !channelInviteURL ||
    !channelWhitelist
  ) {
    return undefined;
  }

  return {
    revision,
    trustedMemberGroupIDs,
    requiredChannelID,
    requiredChannelFailOpen,
    channelDisplay,
    channelInviteURL,
    channelWhitelist
  };
}

export function loadBypassSettings(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<BypassSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    parse: bypassSettingsFromPayload
  });
}

export function saveBypassSettings(
  transport: ApiTransport,
  chatID: string,
  expectedRevision: number,
  changes: BypassSettingsChanges
): Promise<ApiResult<BypassSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    method: "PATCH",
    body: {
      expected_revision: expectedRevision,
      changes
    },
    parse: bypassSettingsFromPayload
  });
}
