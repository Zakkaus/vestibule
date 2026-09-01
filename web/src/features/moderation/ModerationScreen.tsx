import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

import type { ApiRequestError } from "../../lib/api";
import type { SettingSource, SettingValue } from "./api";
import type { ModerationEvaluation, ModerationField } from "./model";
import {
  useModerationSettings,
  type ModerationController,
  type ModerationFeedback,
  type ModerationReadyState
} from "./useModerationSettings";

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "moderation.source.factory",
  "user file": "moderation.source.file",
  "chat override": "moderation.source.chat"
};

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_required: "moderation.errors.authenticationRequired",
  session_expired: "moderation.errors.authenticationExpired",
  chat_access_denied: "moderation.errors.accessDenied",
  chat_access_unavailable: "moderation.errors.accessUnavailable",
  chat_not_found: "moderation.errors.chatNotFound",
  csrf_invalid: "moderation.errors.authenticationExpired",
  invalid_settings: "moderation.errors.invalidSettings",
  settings_unavailable: "moderation.errors.settingsUnavailable"
};

function moderationErrorMessageKey(error: ApiRequestError, fallback: string): string {
  return error.kind === "api" ? (errorMessageKeys[error.code] ?? fallback) : fallback;
}

type StateCardProps = Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  role?: "alert";
  live?: "polite";
  children?: ReactNode;
}>;

function ModerationStateCard({
  id,
  titleKey,
  descriptionKey,
  role,
  live,
  children
}: StateCardProps) {
  const { t } = useTranslation();
  return (
    <section
      data-slot="card"
      data-moderation-state-card={id}
      role={role}
      aria-live={live}
      aria-labelledby={`moderation-${id}-title`}
    >
      <h2 id={`moderation-${id}-title`}>{t(titleKey)}</h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

type SettingMetaProps = Readonly<{
  field: ModerationField;
  source: SettingSource;
  pending: boolean;
  restoring: boolean;
  saving: boolean;
  onSetRestoring: (field: ModerationField, enabled: boolean) => void;
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
          {t(restoring ? "moderation.source.restoring" : "moderation.source.pending")}
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
          {t(restoring ? "moderation.actions.cancelRestore" : "moderation.actions.restore")}
        </button>
      ) : null}
    </span>
  );
}

type NumericSettingProps = Readonly<{
  field: "warnLimit" | "adminLogChatID";
  setting: SettingValue<number>;
  value: string;
  pending: boolean;
  restoring: boolean;
  saving: boolean;
  invalid: boolean;
  onChange: (value: string) => void;
  onSetRestoring: (field: ModerationField, enabled: boolean) => void;
}>;

function NumericSetting({
  field,
  setting,
  value,
  pending,
  restoring,
  saving,
  invalid,
  onChange,
  onSetRestoring
}: NumericSettingProps) {
  const { t } = useTranslation();
  const isWarnLimit = field === "warnLimit";
  const inputID = `moderation-${field}-input`;
  const descriptionID = `moderation-${field}-description`;
  const errorID = `moderation-${field}-error`;
  const readOnly = setting.source === "user file" || restoring || saving;
  const labelKey = isWarnLimit ? "moderation.warnLimit.label" : "moderation.adminLog.label";
  const descriptionKey = isWarnLimit
    ? "moderation.warnLimit.description"
    : "moderation.adminLog.description";
  const validationKey = isWarnLimit
    ? "moderation.validation.warnLimit"
    : "moderation.validation.adminLog";

  return (
    <div data-slot="setting" data-moderation-field={field} data-read-only={readOnly || undefined}>
      <div data-setting-copy>
        <div data-setting-heading>
          <label htmlFor={inputID}>{t(labelKey)}</label>
          <SettingMeta
            field={field}
            source={setting.source}
            pending={pending}
            restoring={restoring}
            saving={saving}
            onSetRestoring={onSetRestoring}
          />
        </div>
        <p id={descriptionID} data-setting-description>
          {t(descriptionKey)}
        </p>
        {invalid ? (
          <p id={errorID} data-slot="field-error" role="alert">
            {t(validationKey)}
          </p>
        ) : null}
      </div>
      <div data-setting-control>
        <input
          id={inputID}
          type={isWarnLimit ? "number" : "text"}
          inputMode={isWarnLimit ? "numeric" : "text"}
          min={isWarnLimit ? 1 : undefined}
          step={isWarnLimit ? 1 : undefined}
          data-slot="input"
          value={value}
          readOnly={readOnly}
          aria-invalid={invalid ? "true" : undefined}
          aria-describedby={`${descriptionID}${invalid ? ` ${errorID}` : ""}`}
          onChange={(event) => onChange(event.currentTarget.value)}
        />
      </div>
    </div>
  );
}

type AntispamSettingProps = Readonly<{
  state: ModerationReadyState;
  pending: boolean;
  onChange: (value: boolean) => void;
  onSetRestoring: (field: ModerationField, enabled: boolean) => void;
}>;

function AntispamSetting({
  state,
  pending,
  onChange,
  onSetRestoring
}: AntispamSettingProps) {
  const { t } = useTranslation();
  const field = "antispamEnabled";
  const restoring = state.restoring[field] === true;
  const readOnly = state.settings.antispamEnabled.source === "user file" || restoring || state.saving;
  return (
    <div data-slot="setting" data-moderation-field={field} data-read-only={readOnly || undefined}>
      <div data-setting-copy>
        <div data-setting-heading>
          <span id="moderation-antispam-label">{t("moderation.antispam.label")}</span>
          <SettingMeta
            field={field}
            source={state.settings.antispamEnabled.source}
            pending={pending}
            restoring={restoring}
            saving={state.saving}
            onSetRestoring={onSetRestoring}
          />
        </div>
        <p id="moderation-antispam-description" data-setting-description>
          {t("moderation.antispam.description")}
        </p>
      </div>
      <div data-setting-control>
        <button
          type="button"
          role="switch"
          data-slot="switch"
          aria-checked={state.form.antispamEnabled}
          aria-disabled={readOnly ? "true" : undefined}
          aria-labelledby="moderation-antispam-label"
          aria-describedby="moderation-antispam-description"
          onClick={() => {
            if (!readOnly) {
              onChange(!state.form.antispamEnabled);
            }
          }}
        />
      </div>
    </div>
  );
}

function ModerationFeedbackNotice({
  feedback,
  onReload
}: Readonly<{ feedback: ModerationFeedback; onReload: () => void }>) {
  const { t } = useTranslation();
  const messageKey =
    feedback.kind === "saved"
      ? "moderation.feedback.saved"
      : feedback.kind === "conflict"
        ? "moderation.feedback.conflict"
        : moderationErrorMessageKey(feedback.error, "moderation.errors.saveUnavailable");
  return (
    <div
      data-moderation-feedback={feedback.kind}
      data-status={feedback.kind === "saved" ? "ok" : "error"}
      role={feedback.kind === "saved" ? "status" : "alert"}
    >
      <span>{t(messageKey)}</span>
      {feedback.kind === "conflict" ? (
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-size="sm"
          onClick={onReload}
        >
          {t("moderation.actions.reload")}
        </button>
      ) : null}
    </div>
  );
}

function ModerationSaveBar({
  state,
  evaluation,
  onDiscard,
  onSave
}: Readonly<{
  state: ModerationReadyState;
  evaluation: ModerationEvaluation;
  onDiscard: () => void;
  onSave: () => void;
}>) {
  const { t } = useTranslation();
  if (evaluation.count === 0) {
    return null;
  }
  const saveBlocked = state.saving || !evaluation.valid;
  return (
    <aside
      data-moderation-savebar
      data-save-state={state.saving ? "submitting" : "idle"}
      aria-label={t("moderation.save.label")}
    >
      <span aria-live="polite">
        {t("moderation.save.unsaved", { count: evaluation.count })}
      </span>
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
          {t("moderation.actions.discard")}
        </button>
        <button
          type="button"
          data-slot="button"
          data-variant="primary"
          data-size="sm"
          aria-disabled={saveBlocked ? "true" : undefined}
          onClick={() => {
            if (!saveBlocked) {
              onSave();
            }
          }}
        >
          {t(state.saving ? "moderation.actions.saving" : "moderation.actions.save")}
        </button>
      </span>
    </aside>
  );
}

function ModerationFormView({ controller }: Readonly<{ controller: ModerationController }>) {
  const state = controller.state;
  const evaluation = controller.evaluation;
  if (state.kind !== "ready" || !evaluation) {
    return null;
  }

  return (
    <div
      data-moderation-form
      data-save-state={state.saving ? "submitting" : (state.feedback?.kind ?? "idle")}
      aria-busy={state.saving || undefined}
    >
      <section data-slot="card" data-moderation-settings-card>
        <NumericSetting
          field="warnLimit"
          setting={state.settings.warnLimit}
          value={state.form.warnLimit}
          pending={evaluation.changes.warn_limit !== undefined || evaluation.errors.warnLimit === true}
          restoring={state.restoring.warnLimit === true}
          saving={state.saving}
          invalid={evaluation.errors.warnLimit === true}
          onChange={controller.editWarnLimit}
          onSetRestoring={controller.setRestoring}
        />
        <AntispamSetting
          state={state}
          pending={evaluation.changes.antispam_enabled !== undefined}
          onChange={controller.editAntispamEnabled}
          onSetRestoring={controller.setRestoring}
        />
        <NumericSetting
          field="adminLogChatID"
          setting={state.settings.adminLogChatID}
          value={state.form.adminLogChatID}
          pending={
            evaluation.changes.admin_log_chat_id !== undefined ||
            evaluation.errors.adminLogChatID === true
          }
          restoring={state.restoring.adminLogChatID === true}
          saving={state.saving}
          invalid={evaluation.errors.adminLogChatID === true}
          onChange={controller.editAdminLogChatID}
          onSetRestoring={controller.setRestoring}
        />
      </section>
      <ModerationSaveBar
        state={state}
        evaluation={evaluation}
        onDiscard={controller.discard}
        onSave={controller.save}
      />
      {state.feedback ? (
        <ModerationFeedbackNotice feedback={state.feedback} onReload={controller.reload} />
      ) : null}
    </div>
  );
}

function ModerationStateContent({
  controller
}: Readonly<{ controller: ModerationController }>) {
  const { t } = useTranslation();
  const { state } = controller;
  if (state.kind === "ready") {
    return <ModerationFormView controller={controller} />;
  }
  if (state.kind === "loading") {
    return (
      <ModerationStateCard
        id="loading"
        titleKey="moderation.loading.title"
        descriptionKey="moderation.loading.description"
        live="polite"
      />
    );
  }
  if (state.kind === "group-required") {
    return (
      <ModerationStateCard
        id="group-required"
        titleKey="moderation.groupRequired.title"
        descriptionKey="moderation.groupRequired.description"
      >
        <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
          {t("moderation.groupRequired.select")}
        </Link>
      </ModerationStateCard>
    );
  }
  if (state.kind === "no-groups") {
    return (
      <ModerationStateCard
        id="no-groups"
        titleKey="moderation.noGroups.title"
        descriptionKey="moderation.noGroups.description"
      />
    );
  }
  return (
    <ModerationStateCard
      id="unavailable"
      titleKey="moderation.unavailable.title"
      descriptionKey={moderationErrorMessageKey(
        state.error,
        "moderation.errors.loadUnavailable"
      )}
      role="alert"
    >
      <button
        type="button"
        data-slot="button"
        data-variant="outline"
        data-size="sm"
        onClick={controller.reload}
      >
        {t("moderation.unavailable.retry")}
      </button>
    </ModerationStateCard>
  );
}

export function ModerationScreen() {
  const { t } = useTranslation();
  const controller = useModerationSettings();
  const { state } = controller;
  const dataState = state.kind === "ready" ? state.origin : state.kind;
  return (
    <section
      data-moderation-page
      data-moderation-state={dataState === "live" ? "loaded" : dataState}
      aria-busy={state.kind === "loading" || (state.kind === "ready" && state.saving) || undefined}
      aria-labelledby="moderation-title"
    >
      <header data-page-heading>
        <h1 id="moderation-title">{t("moderation.title")}</h1>
        <p>{t("moderation.description")}</p>
      </header>
      <ModerationStateContent controller={controller} />
    </section>
  );
}
