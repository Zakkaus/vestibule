import type { ModerationSettings, ModerationSettingsChanges, SettingValue } from "./api";

export const moderationFixtureSettings: ModerationSettings = {
  revision: 7,
  warnLimit: { value: 5, source: "chat override" },
  antispamEnabled: { value: true, source: "factory default" },
  adminLogChatID: { value: 0, source: "user file" }
};

function changedSetting<T>(
  current: SettingValue<T>,
  change: T | null | undefined,
  fallback: T
): SettingValue<T> {
  if (change === undefined) {
    return current;
  }
  if (change === null || Object.is(change, fallback)) {
    return { value: fallback, source: "factory default" };
  }
  return { value: change, source: "chat override" };
}

export function applyModerationFixtureChanges(
  current: ModerationSettings,
  changes: ModerationSettingsChanges
): ModerationSettings {
  const warnLimit = changedSetting(current.warnLimit, changes.warn_limit, 3);
  const antispamEnabled = changedSetting(
    current.antispamEnabled,
    changes.antispam_enabled,
    true
  );
  const adminLogChatID = changedSetting(
    current.adminLogChatID,
    changes.admin_log_chat_id,
    0
  );
  const changed =
    warnLimit !== current.warnLimit ||
    antispamEnabled !== current.antispamEnabled ||
    adminLogChatID !== current.adminLogChatID;

  return {
    revision: changed ? current.revision + 1 : current.revision,
    warnLimit,
    antispamEnabled,
    adminLogChatID
  };
}
