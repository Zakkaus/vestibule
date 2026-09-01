import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export const deliveryModes = ["group", "dm", "both"] as const;
export type DeliveryMode = (typeof deliveryModes)[number];

export const verifyModes = ["kernel", "quiz", "mixed"] as const;
export type VerifyMode = (typeof verifyModes)[number];

export const settingSources = ["factory default", "user file", "chat override"] as const;
export type SettingSource = (typeof settingSources)[number];

export const verificationSettingFields = [
  "delivery_mode",
  "verify_mode",
  "timeout_seconds",
  "verify_max_fails",
  "verify_retry_seconds",
  "ban_seconds",
  "mute_seconds",
  "verify_invited"
] as const;
export type VerificationSettingField = (typeof verificationSettingFields)[number];

export type SourcedSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

export type VerificationSettings = Readonly<{
  revision: number;
  delivery_mode: SourcedSetting<DeliveryMode>;
  verify_mode: SourcedSetting<VerifyMode>;
  timeout_seconds: SourcedSetting<number>;
  verify_max_fails: SourcedSetting<number>;
  verify_retry_seconds: SourcedSetting<number>;
  ban_seconds: SourcedSetting<number>;
  mute_seconds: SourcedSetting<number>;
  verify_invited: SourcedSetting<boolean>;
}>;

export type VerificationSettingsValues = Readonly<{
  delivery_mode: DeliveryMode;
  verify_mode: VerifyMode;
  timeout_seconds: number;
  verify_max_fails: number;
  verify_retry_seconds: number;
  ban_seconds: number;
  mute_seconds: number;
  verify_invited: boolean;
}>;

export type VerificationSettingsChanges = Partial<{
  delivery_mode: DeliveryMode | null;
  verify_mode: VerifyMode | null;
  timeout_seconds: number | null;
  verify_max_fails: number | null;
  verify_retry_seconds: number | null;
  ban_seconds: number | null;
  mute_seconds: number | null;
  verify_invited: boolean | null;
}>;

function choiceFromPayload<T extends readonly string[]>(
  value: unknown,
  choices: T
): T[number] | undefined {
  return typeof value === "string" && (choices as readonly string[]).includes(value)
    ? (value as T[number])
    : undefined;
}

function integerFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) ? value : undefined;
}


function sourcedSettingFromPayload<T>(
  value: unknown,
  parseValue: (candidate: unknown) => T | undefined
): SourcedSetting<T> | undefined {
  const setting = objectFromPayload(value);
  if (!setting) {
    return undefined;
  }

  const parsedValue = parseValue(setting.value);
  const source = choiceFromPayload(setting.source, settingSources);
  if (parsedValue === undefined || source === undefined) {
    return undefined;
  }

  return { value: parsedValue, source };
}

export function verificationSettingsFromPayload(payload: unknown): VerificationSettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const revision = integerFromPayload(response.revision);
  const deliveryMode = sourcedSettingFromPayload(response.delivery_mode, (value) =>
    choiceFromPayload(value, deliveryModes)
  );
  const verifyMode = sourcedSettingFromPayload(response.verify_mode, (value) =>
    choiceFromPayload(value, verifyModes)
  );
  const timeoutSeconds = sourcedSettingFromPayload(response.timeout_seconds, integerFromPayload);
  const verifyMaxFails = sourcedSettingFromPayload(response.verify_max_fails, integerFromPayload);
  const verifyRetrySeconds = sourcedSettingFromPayload(response.verify_retry_seconds, integerFromPayload);
  const banSeconds = sourcedSettingFromPayload(response.ban_seconds, integerFromPayload);
  const muteSeconds = sourcedSettingFromPayload(response.mute_seconds, integerFromPayload);
  const verifyInvited = sourcedSettingFromPayload(response.verify_invited, (value) =>
    typeof value === "boolean" ? value : undefined
  );

  if (
    revision === undefined ||
    revision < 0 ||
    !deliveryMode ||
    !verifyMode ||
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
    revision,
    delivery_mode: deliveryMode,
    verify_mode: verifyMode,
    timeout_seconds: timeoutSeconds,
    verify_max_fails: verifyMaxFails,
    verify_retry_seconds: verifyRetrySeconds,
    ban_seconds: banSeconds,
    mute_seconds: muteSeconds,
    verify_invited: verifyInvited
  };
}

export function loadVerificationSettings(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<VerificationSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    parse: verificationSettingsFromPayload
  });
}

export function saveVerificationSettings(
  transport: ApiTransport,
  chatID: string,
  expectedRevision: number,
  changes: VerificationSettingsChanges
): Promise<ApiResult<VerificationSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    method: "PATCH",
    body: {
      expected_revision: expectedRevision,
      changes
    },
    parse: verificationSettingsFromPayload
  });
}
