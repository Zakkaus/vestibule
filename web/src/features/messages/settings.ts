import type {
  MessageSettingField,
  MessageSettings,
  MessageSettingsChanges,
  MessageSettingsValues
} from "./api";

export type MessageSettingsDraft = Readonly<{
  name_spoiler: boolean;
  lookup_auto_delete_enabled: boolean;
  lookup_ttl_seconds: string;
  rich_messages: boolean;
}>;

export type MessageSettingsErrors = Partial<Record<MessageSettingField, "integer" | "ttl">>;

export type MessageSettingsEvaluation = Readonly<{
  errors: MessageSettingsErrors;
  values?: MessageSettingsValues;
  changes: MessageSettingsChanges;
  changedFields: ReadonlySet<MessageSettingField>;
}>;

export function messageSettingsDraft(settings: MessageSettings): MessageSettingsDraft {
  return {
    name_spoiler: settings.name_spoiler.value,
    lookup_auto_delete_enabled: settings.lookup_auto_delete_enabled.value,
    lookup_ttl_seconds: String(settings.lookup_ttl_seconds.value),
    rich_messages: settings.rich_messages.value
  };
}

function changedFields(
  settings: MessageSettings,
  draft: MessageSettingsDraft,
  restoring: ReadonlySet<MessageSettingField>
): ReadonlySet<MessageSettingField> {
  const fields = new Set<MessageSettingField>();
  for (const field of [
    "name_spoiler",
    "lookup_auto_delete_enabled",
    "lookup_ttl_seconds",
    "rich_messages"
  ] as const) {
    if (restoring.has(field)) {
      fields.add(field);
      continue;
    }

    const draftValue = draft[field];
    const currentValue = settings[field].value;
    if (field === "lookup_ttl_seconds" ? draftValue !== String(currentValue) : draftValue !== currentValue) {
      fields.add(field);
    }
  }
  return fields;
}

export function evaluateMessageSettings(
  settings: MessageSettings,
  draft: MessageSettingsDraft,
  restoring: ReadonlySet<MessageSettingField>
): MessageSettingsEvaluation {
  const pending = changedFields(settings, draft, restoring);
  const ttlSeconds = Number(draft.lookup_ttl_seconds);
  const errors: MessageSettingsErrors = {};
  if (!Number.isSafeInteger(ttlSeconds)) {
    errors.lookup_ttl_seconds = "integer";
  } else if (ttlSeconds < 1 || ttlSeconds > 86_400) {
    errors.lookup_ttl_seconds = "ttl";
  }

  if (errors.lookup_ttl_seconds) {
    return { errors, changes: {}, changedFields: pending };
  }

  const values: MessageSettingsValues = {
    name_spoiler: draft.name_spoiler,
    lookup_auto_delete_enabled: draft.lookup_auto_delete_enabled,
    lookup_ttl_seconds: ttlSeconds,
    rich_messages: draft.rich_messages
  };
  const changes: MessageSettingsChanges = {};
  if (pending.has("name_spoiler")) {
    changes.name_spoiler = restoring.has("name_spoiler") ? null : values.name_spoiler;
  }
  if (pending.has("lookup_auto_delete_enabled")) {
    changes.lookup_auto_delete_enabled = restoring.has("lookup_auto_delete_enabled")
      ? null
      : values.lookup_auto_delete_enabled;
  }
  if (pending.has("lookup_ttl_seconds")) {
    changes.lookup_ttl_seconds = restoring.has("lookup_ttl_seconds")
      ? null
      : values.lookup_ttl_seconds;
  }
  if (pending.has("rich_messages")) {
    changes.rich_messages = restoring.has("rich_messages") ? null : values.rich_messages;
  }

  return { errors, values, changes, changedFields: pending };
}
