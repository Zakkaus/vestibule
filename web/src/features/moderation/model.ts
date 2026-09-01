import type {
  ModerationSettings,
  ModerationSettingsChanges,
  SettingSource,
  SettingValue
} from "./api";

export type ModerationField = "warnLimit" | "antispamEnabled" | "adminLogChatID";

export type ModerationForm = Readonly<{
  warnLimit: string;
  antispamEnabled: boolean;
  adminLogChatID: string;
}>;

export type RestoringFields = Readonly<Partial<Record<ModerationField, true>>>;

export type ModerationValidationErrors = Readonly<{
  warnLimit?: true;
  adminLogChatID?: true;
}>;

export type ModerationEvaluation = Readonly<{
  changes: ModerationSettingsChanges;
  count: number;
  errors: ModerationValidationErrors;
  valid: boolean;
}>;

export function formFromSettings(settings: ModerationSettings): ModerationForm {
  return {
    warnLimit: String(settings.warnLimit.value),
    antispamEnabled: settings.antispamEnabled.value,
    adminLogChatID: settings.adminLogChatID.value === 0 ? "" : String(settings.adminLogChatID.value)
  };
}

export function settingForField(
  settings: ModerationSettings,
  field: ModerationField
): SettingValue<number | boolean> {
  switch (field) {
    case "warnLimit":
      return settings.warnLimit;
    case "antispamEnabled":
      return settings.antispamEnabled;
    case "adminLogChatID":
      return settings.adminLogChatID;
  }
}

export function sourceForField(
  settings: ModerationSettings,
  field: ModerationField
): SettingSource {
  return settingForField(settings, field).source;
}

function parsedInteger(raw: string): number | undefined {
  const trimmed = raw.trim();
  if (!/^-?\d+$/.test(trimmed)) {
    return undefined;
  }
  const value = Number(trimmed);
  return Number.isSafeInteger(value) ? value : undefined;
}

export function evaluateModerationForm(
  settings: ModerationSettings,
  form: ModerationForm,
  restoring: RestoringFields
): ModerationEvaluation {
  const changes: {
    warn_limit?: number | null;
    antispam_enabled?: boolean | null;
    admin_log_chat_id?: number | null;
  } = {};
  const errors: { warnLimit?: true; adminLogChatID?: true } = {};

  if (restoring.warnLimit) {
    changes.warn_limit = null;
  } else if (form.warnLimit !== String(settings.warnLimit.value)) {
    const warnLimit = parsedInteger(form.warnLimit);
    if (warnLimit === undefined || warnLimit <= 0) {
      errors.warnLimit = true;
    } else if (warnLimit !== settings.warnLimit.value) {
      changes.warn_limit = warnLimit;
    }
  }

  if (restoring.antispamEnabled) {
    changes.antispam_enabled = null;
  } else if (form.antispamEnabled !== settings.antispamEnabled.value) {
    changes.antispam_enabled = form.antispamEnabled;
  }

  if (restoring.adminLogChatID) {
    changes.admin_log_chat_id = null;
  } else {
    const rawAdminLogChatID = form.adminLogChatID.trim();
    const adminLogChatID = rawAdminLogChatID === "" ? 0 : parsedInteger(rawAdminLogChatID);
    if (adminLogChatID === undefined) {
      errors.adminLogChatID = true;
    } else if (adminLogChatID !== settings.adminLogChatID.value) {
      changes.admin_log_chat_id = adminLogChatID;
    }
  }

  const count = Object.keys(changes).length + Object.keys(errors).length;
  return {
    changes,
    count,
    errors,
    valid: Object.keys(errors).length === 0
  };
}
