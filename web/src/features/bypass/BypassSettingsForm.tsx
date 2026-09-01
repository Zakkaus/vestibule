import type { FormEvent, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import type { SettingSource, SettingValue } from "./api";
import type {
  BypassEvaluation,
  BypassField,
  BypassTextField,
  BypassValidationError
} from "./model";
import type { BypassFeedback, BypassReadyState } from "./useBypassSettings";

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "bypass.source.factory",
  "user file": "bypass.source.file",
  "chat override": "bypass.source.chat"
};

function validationMessageKey(error: BypassValidationError): string {
  switch (error) {
    case "integer":
      return "bypass.validation.integer";
    case "list":
      return "bypass.validation.list";
    case "reachableChannel":
      return "bypass.validation.reachableChannel";
  }
}

type SettingMetaProps = Readonly<{
  field: BypassField;
  source: SettingSource;
  pending: boolean;
  restoring: boolean;
  saving: boolean;
  onSetRestoring: (field: BypassField, enabled: boolean) => void;
}>;

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
    <span data-setting-meta>
      <span
        data-slot={source === "factory default" ? undefined : "badge"}
        data-status={source === "chat override" ? "info" : "neutral"}
        data-setting-source={source}
      >
        {t(sourceMessageKeys[source])}
      </span>
      {pending ? (
        <span data-slot="badge" data-status="pending" data-setting-pending>
          {t(restoring ? "bypass.source.restoring" : "bypass.source.pending")}
        </span>
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
          {t(restoring ? "bypass.actions.cancelRestore" : "bypass.actions.restore")}
        </button>
      ) : null}
    </span>
  );
}

type SettingRowProps = Readonly<{
  field: BypassField;
  controlID: string;
  labelKey: string;
  descriptionKey: string;
  source: SettingSource;
  pending: boolean;
  restoring: boolean;
  saving: boolean;
  error?: BypassValidationError;
  switchLabel?: true;
  noteKey?: string;
  noteTone?: "warning" | "neutral";
  onSetRestoring: (field: BypassField, enabled: boolean) => void;
  children: (describedBy: string, labelledBy: string) => ReactNode;
}>;

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
  noteKey,
  noteTone,
  onSetRestoring,
  children
}: SettingRowProps) {
  const { t } = useTranslation();
  const labelID = `${controlID}-label`;
  const descriptionID = `${controlID}-description`;
  const errorID = `${controlID}-error`;
  const describedBy = error ? `${descriptionID} ${errorID}` : descriptionID;

  return (
    <div data-slot="setting" data-bypass-setting={field}>
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
        {noteKey && noteTone ? (
          <p data-bypass-safety-note data-tone={noteTone}>
            {t(noteKey)}
          </p>
        ) : null}
        {error ? (
          <p id={errorID} data-slot="field-error" role="alert">
            {t(validationMessageKey(error))}
          </p>
        ) : null}
      </div>
      <div data-setting-control>{children(describedBy, labelID)}</div>
    </div>
  );
}

type TextSettingProps = Readonly<{
  field: BypassTextField;
  controlID: string;
  labelKey: string;
  descriptionKey: string;
  setting: SettingValue<number | string>;
  value: string;
  pending: boolean;
  restoring: boolean;
  saving: boolean;
  error?: BypassValidationError;
  inputMode?: "numeric";
  onChange: (field: BypassTextField, value: string) => void;
  onSetRestoring: (field: BypassField, enabled: boolean) => void;
}>;

function TextSetting({
  field,
  controlID,
  labelKey,
  descriptionKey,
  setting,
  value,
  pending,
  restoring,
  saving,
  error,
  inputMode,
  onChange,
  onSetRestoring
}: TextSettingProps) {
  const readOnly = setting.source === "user file" || restoring || saving;
  return (
    <SettingRow
      field={field}
      controlID={controlID}
      labelKey={labelKey}
      descriptionKey={descriptionKey}
      source={setting.source}
      pending={pending}
      restoring={restoring}
      saving={saving}
      error={error}
      onSetRestoring={onSetRestoring}
    >
      {(describedBy) => (
        <input
          id={controlID}
          type="text"
          inputMode={inputMode}
          data-slot="input"
          value={value}
          readOnly={readOnly}
          aria-invalid={error ? "true" : undefined}
          aria-describedby={describedBy}
          onChange={(event) => onChange(field, event.currentTarget.value)}
        />
      )}
    </SettingRow>
  );
}

type IDListSettingProps = Readonly<{
  field: "trustedMemberGroupIDs" | "channelWhitelist";
  controlID: string;
  labelKey: string;
  descriptionKey: string;
  setting: SettingValue<readonly number[]>;
  value: string;
  pending: boolean;
  restoring: boolean;
  saving: boolean;
  error?: BypassValidationError;
  onChange: (field: BypassTextField, value: string) => void;
  onSetRestoring: (field: BypassField, enabled: boolean) => void;
}>;

function IDListSetting({
  field,
  controlID,
  labelKey,
  descriptionKey,
  setting,
  value,
  pending,
  restoring,
  saving,
  error,
  onChange,
  onSetRestoring
}: IDListSettingProps) {
  const readOnly = setting.source === "user file" || restoring || saving;
  return (
    <SettingRow
      field={field}
      controlID={controlID}
      labelKey={labelKey}
      descriptionKey={descriptionKey}
      source={setting.source}
      pending={pending}
      restoring={restoring}
      saving={saving}
      error={error}
      onSetRestoring={onSetRestoring}
    >
      {(describedBy) => (
        <textarea
          id={controlID}
          data-slot="textarea"
          rows={4}
          value={value}
          readOnly={readOnly}
          aria-invalid={error ? "true" : undefined}
          aria-describedby={describedBy}
          onChange={(event) => onChange(field, event.currentTarget.value)}
        />
      )}
    </SettingRow>
  );
}

type FailOpenSettingProps = Readonly<{
  state: BypassReadyState;
  pending: boolean;
  onChange: (value: boolean) => void;
  onSetRestoring: (field: BypassField, enabled: boolean) => void;
}>;

function FailOpenSetting({ state, pending, onChange, onSetRestoring }: FailOpenSettingProps) {
  const field = "requiredChannelFailOpen";
  const setting = state.settings.requiredChannelFailOpen;
  const restoring = state.restoring[field] === true;
  const readOnly = setting.source === "user file" || restoring || state.saving;
  const controlID = "bypass-required-channel-fail-open";
  const isFailOpen = state.form.requiredChannelFailOpen;

  return (
    <SettingRow
      field={field}
      controlID={controlID}
      labelKey="bypass.requiredChannel.failOpen.label"
      descriptionKey="bypass.requiredChannel.failOpen.description"
      source={setting.source}
      pending={pending}
      restoring={restoring}
      saving={state.saving}
      switchLabel
      noteKey={
        isFailOpen
          ? "bypass.requiredChannel.failOpen.enabled"
          : "bypass.requiredChannel.failOpen.disabled"
      }
      noteTone={isFailOpen ? "warning" : "neutral"}
      onSetRestoring={onSetRestoring}
    >
      {(describedBy, labelledBy) => (
        <button
          id={controlID}
          type="button"
          role="switch"
          data-slot="switch"
          aria-checked={isFailOpen}
          aria-disabled={readOnly ? "true" : undefined}
          aria-labelledby={labelledBy}
          aria-describedby={describedBy}
          onClick={() => {
            if (!readOnly) {
              onChange(!isFailOpen);
            }
          }}
        />
      )}
    </SettingRow>
  );
}

function BypassFeedbackNotice({
  feedback,
  errorMessageKey
}: Readonly<{
  feedback: BypassFeedback;
  errorMessageKey: (
    error: Extract<BypassFeedback, { kind: "error" }>["error"]
  ) => string;
}>) {
  const { t } = useTranslation();
  const messageKey =
    feedback.kind === "saved"
      ? "bypass.feedback.saved"
      : feedback.kind === "conflict"
        ? "bypass.feedback.conflict"
        : errorMessageKey(feedback.error);
  return (
    <div
      data-bypass-feedback={feedback.kind}
      data-status={feedback.kind === "saved" ? "ok" : "error"}
      role={feedback.kind === "saved" ? "status" : "alert"}
    >
      {t(messageKey)}
    </div>
  );
}

type BypassSettingsFormProps = Readonly<{
  state: BypassReadyState;
  evaluation: BypassEvaluation;
  onEditText: (field: BypassTextField, value: string) => void;
  onEditFailOpen: (value: boolean) => void;
  onSetRestoring: (field: BypassField, enabled: boolean) => void;
  onDiscard: () => void;
  onSave: () => void;
  errorMessageKey: (error: Extract<BypassFeedback, { kind: "error" }>["error"]) => string;
}>;

export function BypassSettingsForm({
  state,
  evaluation,
  onEditText,
  onEditFailOpen,
  onSetRestoring,
  onDiscard,
  onSave,
  errorMessageKey
}: BypassSettingsFormProps) {
  const { t } = useTranslation();
  const saveBlocked = state.saving || !evaluation.valid;

  function submit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    onSave();
  }

  return (
    <form data-bypass-form onSubmit={submit} aria-busy={state.saving || undefined}>
      <section data-slot="card" data-bypass-settings-card aria-labelledby="bypass-trusted-title">
        <div data-bypass-section-heading>
          <h2 id="bypass-trusted-title">{t("bypass.trusted.title")}</h2>
        </div>
        <IDListSetting
          field="trustedMemberGroupIDs"
          controlID="bypass-trusted-member-group-ids"
          labelKey="bypass.trusted.label"
          descriptionKey="bypass.trusted.description"
          setting={state.settings.trustedMemberGroupIDs}
          value={state.form.trustedMemberGroupIDs}
          pending={
            evaluation.changes.trusted_member_group_ids !== undefined ||
            evaluation.errors.trustedMemberGroupIDs !== undefined
          }
          restoring={state.restoring.trustedMemberGroupIDs === true}
          saving={state.saving}
          error={evaluation.errors.trustedMemberGroupIDs}
          onChange={onEditText}
          onSetRestoring={onSetRestoring}
        />
      </section>

      <section data-slot="card" data-bypass-settings-card aria-labelledby="bypass-required-channel-title">
        <div data-bypass-section-heading>
          <h2 id="bypass-required-channel-title">{t("bypass.requiredChannel.title")}</h2>
          <p>{t("bypass.requiredChannel.sectionDescription")}</p>
        </div>
        <TextSetting
          field="requiredChannelID"
          controlID="bypass-required-channel-id"
          labelKey="bypass.requiredChannel.id.label"
          descriptionKey="bypass.requiredChannel.id.description"
          setting={state.settings.requiredChannelID}
          value={state.form.requiredChannelID}
          pending={
            evaluation.changes.required_channel_id !== undefined ||
            evaluation.errors.requiredChannelID !== undefined
          }
          restoring={state.restoring.requiredChannelID === true}
          saving={state.saving}
          error={evaluation.errors.requiredChannelID}
          inputMode="numeric"
          onChange={onEditText}
          onSetRestoring={onSetRestoring}
        />
        <TextSetting
          field="channelDisplay"
          controlID="bypass-channel-display"
          labelKey="bypass.requiredChannel.display.label"
          descriptionKey="bypass.requiredChannel.display.description"
          setting={state.settings.channelDisplay}
          value={state.form.channelDisplay}
          pending={evaluation.changes.channel_display !== undefined}
          restoring={state.restoring.channelDisplay === true}
          saving={state.saving}
          onChange={onEditText}
          onSetRestoring={onSetRestoring}
        />
        <TextSetting
          field="channelInviteURL"
          controlID="bypass-channel-invite-url"
          labelKey="bypass.requiredChannel.invite.label"
          descriptionKey="bypass.requiredChannel.invite.description"
          setting={state.settings.channelInviteURL}
          value={state.form.channelInviteURL}
          pending={evaluation.changes.channel_invite_url !== undefined}
          restoring={state.restoring.channelInviteURL === true}
          saving={state.saving}
          onChange={onEditText}
          onSetRestoring={onSetRestoring}
        />
        <FailOpenSetting
          state={state}
          pending={evaluation.changes.required_channel_fail_open !== undefined}
          onChange={onEditFailOpen}
          onSetRestoring={onSetRestoring}
        />
      </section>

      <section data-slot="card" data-bypass-settings-card aria-labelledby="bypass-channel-whitelist-title">
        <div data-bypass-section-heading>
          <h2 id="bypass-channel-whitelist-title">{t("bypass.channelWhitelist.title")}</h2>
        </div>
        <IDListSetting
          field="channelWhitelist"
          controlID="bypass-channel-whitelist"
          labelKey="bypass.channelWhitelist.label"
          descriptionKey="bypass.channelWhitelist.description"
          setting={state.settings.channelWhitelist}
          value={state.form.channelWhitelist}
          pending={
            evaluation.changes.channel_whitelist !== undefined ||
            evaluation.errors.channelWhitelist !== undefined
          }
          restoring={state.restoring.channelWhitelist === true}
          saving={state.saving}
          error={evaluation.errors.channelWhitelist}
          onChange={onEditText}
          onSetRestoring={onSetRestoring}
        />
      </section>

      <section data-slot="card" data-bypass-member-whitelist aria-labelledby="bypass-member-whitelist-title">
        <h2 id="bypass-member-whitelist-title">{t("bypass.memberWhitelist.title")}</h2>
        <p>{t("bypass.memberWhitelist.description")}</p>
      </section>

      {evaluation.count > 0 ? (
        <aside
          data-bypass-savebar
          data-save-state={state.saving ? "submitting" : "idle"}
          aria-label={t("bypass.save.label")}
        >
          <span aria-live="polite">{t("bypass.save.unsaved", { count: evaluation.count })}</span>
          <span data-save-actions>
            <button
              type="button"
              data-slot="button"
              data-variant="outline"
              data-size="sm"
              aria-disabled={state.saving ? "true" : undefined}
              onClick={() => {
                if (!state.saving) {
                  onDiscard();
                }
              }}
            >
              {t("bypass.actions.discard")}
            </button>
            <button
              type="submit"
              data-slot="button"
              data-variant="primary"
              data-size="sm"
              aria-disabled={saveBlocked ? "true" : undefined}
              onClick={(event) => {
                if (saveBlocked) {
                  event.preventDefault();
                }
              }}
            >
              {t(state.saving ? "bypass.actions.saving" : "bypass.actions.save")}
            </button>
          </span>
        </aside>
      ) : null}
      {state.feedback ? (
        <BypassFeedbackNotice feedback={state.feedback} errorMessageKey={errorMessageKey} />
      ) : null}
    </form>
  );
}
