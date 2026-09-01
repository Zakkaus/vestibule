import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export const settingSources = ["factory default", "user file", "chat override"] as const;
export type SettingSource = (typeof settingSources)[number];

export type SourcedSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

export type CapabilitySettings = Readonly<{
  revision: number;
  enabled: SourcedSetting<boolean>;
  antispam_enabled: SourcedSetting<boolean>;
}>;

export type CapabilitySettingsChanges = Readonly<{
  enabled?: boolean | null;
}>;

function sourcedBooleanFromPayload(value: unknown): SourcedSetting<boolean> | undefined {
  const setting = objectFromPayload(value);
  if (!setting || typeof setting.value !== "boolean" || typeof setting.source !== "string") {
    return undefined;
  }
  if (!(settingSources as readonly string[]).includes(setting.source)) {
    return undefined;
  }

  return { value: setting.value, source: setting.source as SettingSource };
}

export function capabilitySettingsFromPayload(payload: unknown): CapabilitySettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const revision = response.revision;
  const enabled = sourcedBooleanFromPayload(response.enabled);
  const antispamEnabled = sourcedBooleanFromPayload(response.antispam_enabled);
  if (
    typeof revision !== "number" ||
    !Number.isSafeInteger(revision) ||
    revision < 0 ||
    !enabled ||
    !antispamEnabled
  ) {
    return undefined;
  }

  return {
    revision,
    enabled,
    antispam_enabled: antispamEnabled
  };
}

export function loadCapabilitySettings(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<CapabilitySettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    parse: capabilitySettingsFromPayload
  });
}

export function saveCapabilitySettings(
  transport: ApiTransport,
  chatID: string,
  expectedRevision: number,
  changes: CapabilitySettingsChanges
): Promise<ApiResult<CapabilitySettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    method: "PATCH",
    body: {
      expected_revision: expectedRevision,
      changes
    },
    parse: capabilitySettingsFromPayload
  });
}
