import { type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { StatusBadge } from "../../components/StatusBadge";
import type {
  MessageSettingField,
  MessageSettings,
  SettingSource
} from "./api";
import type {
  MessageSettingsDraft,
  MessageSettingsErrors,
  MessageSettingsEvaluation
} from "./settings";

type MessageSettingsFormProps = Readonly<{
  settings: MessageSettings;
  draft: MessageSettingsDraft;
  evaluation: MessageSettingsEvaluation;
  restoring: ReadonlySet<MessageSettingField>;
  saving: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onDraftChange: <K extends keyof MessageSettingsDraft>(
    field: K,
    value: MessageSettingsDraft[K]
  ) => void;
  onSetRestoring: (field: MessageSettingField, restoring: boolean) => void;
}>;

type SettingMetaProps = Readonly<{
  field: MessageSettingField;
  source: SettingSource;
  pending: boolean;
  restoring: boolean;
  saving: boolean;
  onSetRestoring: (field: MessageSettingField, restoring: boolean) => void;
}>;

type SettingRowProps = Readonly<{
  field: MessageSettingField;
  controlID: string;
  labelKey: string;
  descriptionKey: string;
  source: SettingSource;
  pending: boolean;
  restoring: boolean;
  saving: boolean;
  error?: MessageSettingsErrors[MessageSettingField];
  switchLabel?: true;
  onSetRestoring: (field: MessageSettingField, restoring: boolean) => void;
  children: (describedBy: string, labelledBy: string, readOnly: boolean) => ReactNode;
}>;

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "messages.source.factoryDefault",
  "user file": "messages.source.userFile",
  "chat override": "messages.source.chatOverride"
};

function SettingMeta({
  field,
  source,
  pending,
  restoring,
  saving,
  onSetRestoring
}: SettingMetaProps) {
  const { t } = useTranslation();

  return (
    <span data-messages-setting-meta>
      <StatusBadge tone={source === "chat override" ? "info" : "neutral"}>
        {t("messages.source.value", { source: t(sourceMessageKeys[source]) })}
      </StatusBadge>
      {pending ? (
        <StatusBadge tone="pending">
          {t(restoring ? "messages.source.restoring" : "messages.source.pending")}
        </StatusBadge>
      ) : null}
      {source === "chat override" ? (
        <button
          type="button"
          data-slot="button"
          data-variant="link"
          data-size="sm"
          aria-disabled={saving ? "true" : undefined}
          onClick={() => {
            if (!saving) {
              onSetRestoring(field, !restoring);
            }
          }}
        >
          {t(restoring ? "messages.actions.cancelRestore" : "messages.actions.restore")}
        </button>
      ) : null}
    </span>
  );
}

function SettingRow({
  field,
  controlID,
  labelKey,
  descriptionKey,
  source,
  pending,
  restoring,
  saving,
  error,
  switchLabel,
  onSetRestoring,
  children
}: SettingRowProps) {
  const { t } = useTranslation();
  const labelID = `${controlID}-label`;
  const descriptionID = `${controlID}-description`;
  const errorID = `${controlID}-error`;
  const readOnly = restoring || saving;
  const describedBy = error ? `${descriptionID} ${errorID}` : descriptionID;

  return (
    <div data-slot="setting" data-messages-setting={field} data-read-only={readOnly || undefined}>
      <div data-setting-copy>
        <div data-setting-heading>
          {switchLabel ? (
            <span id={labelID}>{t(labelKey)}</span>
          ) : (
            <label id={labelID} htmlFor={controlID}>
              {t(labelKey)}
            </label>
          )}
          <SettingMeta
            field={field}
            source={source}
            pending={pending}
            restoring={restoring}
            saving={saving}
            onSetRestoring={onSetRestoring}
          />
        </div>
        <p id={descriptionID} data-setting-description>
          {t(descriptionKey)}
        </p>
        {error ? (
          <p id={errorID} data-slot="field-error" role="alert">
            {t(error === "integer" ? "messages.validation.integer" : "messages.validation.ttl")}
          </p>
        ) : null}
      </div>
      <div data-setting-control>{children(describedBy, labelID, readOnly)}</div>
    </div>
  );
}

export function MessageSettingsForm({
  settings,
  draft,
  evaluation,
  restoring,
  saving,
  onSubmit,
  onDraftChange,
  onSetRestoring
}: MessageSettingsFormProps) {
  const { t } = useTranslation();
  const changedCount = evaluation.changedFields.size;
  const hasChanges = changedCount > 0;
  const saveBlocked = saving || !evaluation.values || !hasChanges;

  return (
    <form
      data-messages-settings-form
      data-save-state={saving ? "submitting" : "idle"}
      aria-busy={saving || undefined}
      onSubmit={onSubmit}
    >
      <section data-slot="card" data-messages-settings-section aria-labelledby="messages-display-title">
        <div data-messages-section-heading>
          <h2 id="messages-display-title">{t("messages.display.title")}</h2>
          <p>{t("messages.display.description")}</p>
        </div>
        <SettingRow
          field="name_spoiler"
          controlID="messages-name-spoiler"
          labelKey="messages.display.nameSpoiler.label"
          descriptionKey="messages.display.nameSpoiler.description"
          source={settings.name_spoiler.source}
          pending={evaluation.changedFields.has("name_spoiler")}
          restoring={restoring.has("name_spoiler")}
          saving={saving}
          switchLabel
          onSetRestoring={onSetRestoring}
        >
          {(describedBy, labelledBy, readOnly) => (
            <button
              id="messages-name-spoiler"
              type="button"
              role="switch"
              data-slot="switch"
              aria-checked={draft.name_spoiler}
              aria-disabled={readOnly ? "true" : undefined}
              aria-labelledby={labelledBy}
              aria-describedby={describedBy}
              onClick={() => {
                if (!readOnly) {
                  onDraftChange("name_spoiler", !draft.name_spoiler);
                }
              }}
            />
          )}
        </SettingRow>
        <SettingRow
          field="rich_messages"
          controlID="messages-rich-messages"
          labelKey="messages.display.richMessages.label"
          descriptionKey="messages.display.richMessages.description"
          source={settings.rich_messages.source}
          pending={evaluation.changedFields.has("rich_messages")}
          restoring={restoring.has("rich_messages")}
          saving={saving}
          switchLabel
          onSetRestoring={onSetRestoring}
        >
          {(describedBy, labelledBy, readOnly) => (
            <button
              id="messages-rich-messages"
              type="button"
              role="switch"
              data-slot="switch"
              aria-checked={draft.rich_messages}
              aria-disabled={readOnly ? "true" : undefined}
              aria-labelledby={labelledBy}
              aria-describedby={describedBy}
              onClick={() => {
                if (!readOnly) {
                  onDraftChange("rich_messages", !draft.rich_messages);
                }
              }}
            />
          )}
        </SettingRow>
      </section>

      <section data-slot="card" data-messages-settings-section aria-labelledby="messages-auto-delete-title">
        <div data-messages-section-heading>
          <h2 id="messages-auto-delete-title">{t("messages.autoDelete.title")}</h2>
          <p>{t("messages.autoDelete.description")}</p>
        </div>
        <SettingRow
          field="lookup_auto_delete_enabled"
          controlID="messages-auto-delete-enabled"
          labelKey="messages.autoDelete.enabled.label"
          descriptionKey="messages.autoDelete.enabled.description"
          source={settings.lookup_auto_delete_enabled.source}
          pending={evaluation.changedFields.has("lookup_auto_delete_enabled")}
          restoring={restoring.has("lookup_auto_delete_enabled")}
          saving={saving}
          switchLabel
          onSetRestoring={onSetRestoring}
        >
          {(describedBy, labelledBy, readOnly) => (
            <button
              id="messages-auto-delete-enabled"
              type="button"
              role="switch"
              data-slot="switch"
              aria-checked={draft.lookup_auto_delete_enabled}
              aria-disabled={readOnly ? "true" : undefined}
              aria-labelledby={labelledBy}
              aria-describedby={describedBy}
              onClick={() => {
                if (!readOnly) {
                  onDraftChange(
                    "lookup_auto_delete_enabled",
                    !draft.lookup_auto_delete_enabled
                  );
                }
              }}
            />
          )}
        </SettingRow>
        <SettingRow
          field="lookup_ttl_seconds"
          controlID="messages-auto-delete-delay"
          labelKey="messages.autoDelete.delay.label"
          descriptionKey="messages.autoDelete.delay.description"
          source={settings.lookup_ttl_seconds.source}
          pending={evaluation.changedFields.has("lookup_ttl_seconds")}
          restoring={restoring.has("lookup_ttl_seconds")}
          saving={saving}
          error={evaluation.errors.lookup_ttl_seconds}
          onSetRestoring={onSetRestoring}
        >
          {(describedBy, _labelledBy, readOnly) => (
            <input
              id="messages-auto-delete-delay"
              type="number"
              inputMode="numeric"
              min="1"
              max="86400"
              step="1"
              data-slot="input"
              data-messages-delay-input
              value={draft.lookup_ttl_seconds}
              readOnly={readOnly}
              aria-invalid={evaluation.errors.lookup_ttl_seconds ? "true" : undefined}
              aria-describedby={describedBy}
              onChange={(event) => onDraftChange("lookup_ttl_seconds", event.currentTarget.value)}
            />
          )}
        </SettingRow>
      </section>

      <footer data-slot="card" data-messages-settings-savebar>
        <div>
          <h2>{t("messages.settingsSave.title")}</h2>
          <p>
            {t(
              hasChanges ? "messages.settingsSave.dirty" : "messages.settingsSave.clean",
              { count: changedCount }
            )}
          </p>
        </div>
        <button
          type="submit"
          data-slot="button"
          data-variant="primary"
          aria-disabled={saveBlocked ? "true" : undefined}
          onClick={(event) => {
            if (saveBlocked) {
              event.preventDefault();
            }
          }}
        >
          {t(saving ? "messages.actions.savingSettings" : "messages.actions.saveSettings")}
        </button>
      </footer>
    </form>
  );
}
