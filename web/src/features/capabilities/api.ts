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

export type CapabilityField =
  | "enabled"
  | "antispam_enabled"
  | "gentoo_lookups_enabled"
  | "linux_lookups_enabled";

export const capabilityFields = [
  "enabled",
  "antispam_enabled",
  "gentoo_lookups_enabled",
  "linux_lookups_enabled"
] as const satisfies readonly CapabilityField[];

export type CapabilitySettings = Readonly<{
  revision: number;
  enabled: SourcedSetting<boolean>;
  antispam_enabled: SourcedSetting<boolean>;
  gentoo_lookups_enabled: SourcedSetting<boolean>;
  linux_lookups_enabled: SourcedSetting<boolean>;
}>;

export type CapabilitySettingsChanges = Readonly<
  Partial<Record<CapabilityField, boolean | null>>
>;

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
  const gentooLookupsEnabled = sourcedBooleanFromPayload(response.gentoo_lookups_enabled);
  const linuxLookupsEnabled = sourcedBooleanFromPayload(response.linux_lookups_enabled);
  if (
    typeof revision !== "number" ||
    !Number.isSafeInteger(revision) ||
    revision < 0 ||
    !enabled ||
    !antispamEnabled ||
    !gentooLookupsEnabled ||
    !linuxLookupsEnabled
  ) {
    return undefined;
  }

  return {
    revision,
    enabled,
    antispam_enabled: antispamEnabled,
    gentoo_lookups_enabled: gentooLookupsEnabled,
    linux_lookups_enabled: linuxLookupsEnabled
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
