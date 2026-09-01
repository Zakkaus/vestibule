import type {
  DeliveryMode,
  VerificationSettingField,
  VerificationSettings,
  VerificationSettingsChanges,
  VerificationSettingsValues,
  VerifyMode
} from "./api";

export type DraftSettings = Readonly<{
  delivery_mode: DeliveryMode;
  verify_mode: VerifyMode;
  timeout_seconds: string;
  verify_max_fails: string;
  verify_retry_seconds: string;
  ban_seconds: string;
  mute_seconds: string;
  verify_invited: boolean;
}>;

export type FieldErrors = Readonly<Partial<Record<VerificationSettingField, string>>>;

export type DraftValidation = Readonly<{
  errors: FieldErrors;
  values?: VerificationSettingsValues;
}>;

const MAX_TELEGRAM_DURATION_SECONDS = 366 * 24 * 60 * 60;

export function settingsDraft(settings: VerificationSettings): DraftSettings {
  return {
    delivery_mode: settings.delivery_mode.value,
    verify_mode: settings.verify_mode.value,
    timeout_seconds: String(settings.timeout_seconds.value),
    verify_max_fails: String(settings.verify_max_fails.value),
    verify_retry_seconds: String(settings.verify_retry_seconds.value),
    ban_seconds: String(settings.ban_seconds.value),
    mute_seconds: String(settings.mute_seconds.value),
    verify_invited: settings.verify_invited.value
  };
}

function integerFromDraft(value: string): number | undefined {
  if (!/^-?\d+$/.test(value)) {
    return undefined;
  }

  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : undefined;
}

export function validateDraft(draft: DraftSettings): DraftValidation {
  const timeoutSeconds = integerFromDraft(draft.timeout_seconds);
  const verifyMaxFails = integerFromDraft(draft.verify_max_fails);
  const verifyRetrySeconds = integerFromDraft(draft.verify_retry_seconds);
  const banSeconds = integerFromDraft(draft.ban_seconds);
  const muteSeconds = integerFromDraft(draft.mute_seconds);
  const errors: Partial<Record<VerificationSettingField, string>> = {};

  if (timeoutSeconds === undefined) {
    errors.timeout_seconds = "verification.validation.integer";
  } else if (timeoutSeconds < 30 || timeoutSeconds > 1800) {
    errors.timeout_seconds = "verification.validation.timeout";
  }
  if (verifyMaxFails === undefined) {
    errors.verify_max_fails = "verification.validation.integer";
  }
  if (verifyRetrySeconds === undefined) {
    errors.verify_retry_seconds = "verification.validation.integer";
  } else if (verifyRetrySeconds > MAX_TELEGRAM_DURATION_SECONDS) {
    errors.verify_retry_seconds = "verification.validation.retry";
  }
  if (banSeconds === undefined) {
    errors.ban_seconds = "verification.validation.integer";
  } else if (
    banSeconds !== 0 &&
    (banSeconds < 30 || banSeconds > MAX_TELEGRAM_DURATION_SECONDS)
  ) {
    errors.ban_seconds = "verification.validation.ban";
  }
  if (muteSeconds === undefined) {
    errors.mute_seconds = "verification.validation.integer";
  } else if (muteSeconds < 30 || muteSeconds > MAX_TELEGRAM_DURATION_SECONDS) {
    errors.mute_seconds = "verification.validation.mute";
  }

  if (
    Object.keys(errors).length > 0 ||
    timeoutSeconds === undefined ||
    verifyMaxFails === undefined ||
    verifyRetrySeconds === undefined ||
    banSeconds === undefined ||
    muteSeconds === undefined
  ) {
    return { errors };
  }

  return {
    errors,
    values: {
      delivery_mode: draft.delivery_mode,
      verify_mode: draft.verify_mode,
      timeout_seconds: timeoutSeconds,
      verify_max_fails: verifyMaxFails,
      verify_retry_seconds: verifyRetrySeconds,
      ban_seconds: banSeconds,
      mute_seconds: muteSeconds,
      verify_invited: draft.verify_invited
    }
  };
}

export function hasDraftChanges(
  settings: VerificationSettings,
  draft: DraftSettings,
  restored: ReadonlySet<VerificationSettingField>
): boolean {
  return (
    restored.size > 0 ||
    draft.delivery_mode !== settings.delivery_mode.value ||
    draft.verify_mode !== settings.verify_mode.value ||
    draft.timeout_seconds !== String(settings.timeout_seconds.value) ||
    draft.verify_max_fails !== String(settings.verify_max_fails.value) ||
    draft.verify_retry_seconds !== String(settings.verify_retry_seconds.value) ||
    draft.ban_seconds !== String(settings.ban_seconds.value) ||
    draft.mute_seconds !== String(settings.mute_seconds.value) ||
    draft.verify_invited !== settings.verify_invited.value
  );
}

export function sparseChanges(
  settings: VerificationSettings,
  values: VerificationSettingsValues,
  restored: ReadonlySet<VerificationSettingField>
): VerificationSettingsChanges {
  const changes: VerificationSettingsChanges = {};

  if (restored.has("delivery_mode")) {
    changes.delivery_mode = null;
  } else if (values.delivery_mode !== settings.delivery_mode.value) {
    changes.delivery_mode = values.delivery_mode;
  }
  if (restored.has("verify_mode")) {
    changes.verify_mode = null;
  } else if (values.verify_mode !== settings.verify_mode.value) {
    changes.verify_mode = values.verify_mode;
  }
  if (restored.has("timeout_seconds")) {
    changes.timeout_seconds = null;
  } else if (values.timeout_seconds !== settings.timeout_seconds.value) {
    changes.timeout_seconds = values.timeout_seconds;
  }
  if (restored.has("verify_max_fails")) {
    changes.verify_max_fails = null;
  } else if (values.verify_max_fails !== settings.verify_max_fails.value) {
    changes.verify_max_fails = values.verify_max_fails;
  }
  if (restored.has("verify_retry_seconds")) {
    changes.verify_retry_seconds = null;
  } else if (values.verify_retry_seconds !== settings.verify_retry_seconds.value) {
    changes.verify_retry_seconds = values.verify_retry_seconds;
  }
  if (restored.has("ban_seconds")) {
    changes.ban_seconds = null;
  } else if (values.ban_seconds !== settings.ban_seconds.value) {
    changes.ban_seconds = values.ban_seconds;
  }
  if (restored.has("mute_seconds")) {
    changes.mute_seconds = null;
  } else if (values.mute_seconds !== settings.mute_seconds.value) {
    changes.mute_seconds = values.mute_seconds;
  }
  if (restored.has("verify_invited")) {
    changes.verify_invited = null;
  } else if (values.verify_invited !== settings.verify_invited.value) {
    changes.verify_invited = values.verify_invited;
  }

  return changes;
}
