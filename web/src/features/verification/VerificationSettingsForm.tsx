import { type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { StatusBadge } from "../../components/StatusBadge";
import {
  deliveryModes,
  verifyModes,
  type DeliveryMode,
  type SettingSource,
  type VerificationSettingField,
  type VerificationSettings,
  type VerifyMode
} from "./api";
import type { DraftSettings, FieldErrors } from "./form";

type VerificationSettingsFormProps = Readonly<{
  settings: VerificationSettings;
  draft: DraftSettings;
  errors: FieldErrors;
  saving: boolean;
  hasChanges: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onDraftChange: <K extends keyof DraftSettings>(field: K, value: DraftSettings[K]) => void;
  onRestore: (field: VerificationSettingField) => void;
}>;

type SettingRowProps = Readonly<{
  field: VerificationSettingField;
  controlID: string;
  labelKey: string;
  descriptionKey: string;
  source: SettingSource;
  errorKey?: string;
  onRestore: (field: VerificationSettingField) => void;
  children: (describedBy: string) => ReactNode;
}>;

type NumericSettingProps = Readonly<{
  field:
    | "timeout_seconds"
    | "verify_max_fails"
    | "verify_retry_seconds"
    | "ban_seconds"
    | "mute_seconds";
  controlID: string;
  labelKey: string;
  descriptionKey: string;
  source: SettingSource;
  errorKey?: string;
  value: string;
  saving: boolean;
  onChange: (value: string) => void;
  onRestore: (field: VerificationSettingField) => void;
}>;

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "verification.source.factoryDefault",
  "user file": "verification.source.userFile",
  "chat override": "verification.source.chatOverride"
};

const deliveryModeMessageKeys: Readonly<Record<DeliveryMode, string>> = {
  group: "verification.delivery.group",
  dm: "verification.delivery.dm",
  both: "verification.delivery.both"
};

const deliveryModeDescriptionKeys: Readonly<Record<DeliveryMode, string>> = {
  group: "verification.delivery.groupDescription",
  dm: "verification.delivery.dmDescription",
  both: "verification.delivery.bothDescription"
};

const verifyModeMessageKeys: Readonly<Record<VerifyMode, string>> = {
  kernel: "verification.challenge.kernel",
  quiz: "verification.challenge.quiz",
  mixed: "verification.challenge.mixed"
};

const verifyModeDescriptionKeys: Readonly<Record<VerifyMode, string>> = {
  kernel: "verification.challenge.kernelDescription",
  quiz: "verification.challenge.quizDescription",
  mixed: "verification.challenge.mixedDescription"
};

function SettingRow({
  field,
  controlID,
  labelKey,
  descriptionKey,
  source,
  errorKey,
  onRestore,
  children
}: SettingRowProps) {
  const { t } = useTranslation();
  const descriptionID = `${controlID}-description`;
  const errorID = `${controlID}-error`;
  const describedBy = errorKey ? `${descriptionID} ${errorID}` : descriptionID;

  return (
    <div data-slot="setting" data-verification-setting={field}>
      <div data-verification-setting-copy>
        <label htmlFor={controlID}>{t(labelKey)}</label>
        <p id={descriptionID}>{t(descriptionKey)}</p>
        <StatusBadge tone="neutral">
          {t("verification.source.value", { source: t(sourceMessageKeys[source]) })}
        </StatusBadge>
        {errorKey ? (
          <p id={errorID} data-slot="field-error" role="alert">
            {t(errorKey)}
          </p>
        ) : null}
      </div>
      <div data-verification-setting-control>
        {children(describedBy)}
        {source === "chat override" ? (
          <button
            type="button"
            data-slot="button"
            data-variant="link"
            data-size="sm"
            onClick={() => onRestore(field)}
          >
            {t("verification.actions.restore")}
          </button>
        ) : null}
      </div>
    </div>
  );
}

function NumericSetting({
  field,
  controlID,
  labelKey,
  descriptionKey,
  source,
  errorKey,
  value,
  saving,
  onChange,
  onRestore
}: NumericSettingProps) {
  return (
    <SettingRow
      field={field}
      controlID={controlID}
      labelKey={labelKey}
      descriptionKey={descriptionKey}
      source={source}
      errorKey={errorKey}
      onRestore={onRestore}
    >
      {(describedBy) => (
        <input
          id={controlID}
          data-slot="input"
          data-verification-number
          type="number"
          inputMode="numeric"
          step="1"
          aria-invalid={errorKey ? "true" : undefined}
          aria-describedby={describedBy}
          value={value}
          disabled={saving}
          onChange={(event) => onChange(event.currentTarget.value)}
        />
      )}
    </SettingRow>
  );
}

export function VerificationSettingsForm({
  settings,
  draft,
  errors,
  saving,
  hasChanges,
  onSubmit,
  onDraftChange,
  onRestore
}: VerificationSettingsFormProps) {
  const { t } = useTranslation();

  return (
    <form data-verification-form onSubmit={onSubmit}>
      <section data-slot="card" data-verification-section aria-labelledby="verification-delivery-title">
        <div data-verification-section-heading>
          <h2 id="verification-delivery-title">{t("verification.delivery.title")}</h2>
          <p>{t("verification.delivery.description")}</p>
        </div>
        <SettingRow
          field="delivery_mode"
          controlID="verification-delivery-mode"
          labelKey="verification.delivery.label"
          descriptionKey={deliveryModeDescriptionKeys[draft.delivery_mode]}
          source={settings.delivery_mode.source}
          errorKey={errors.delivery_mode}
          onRestore={onRestore}
        >
          {(describedBy) => (
            <select
              id="verification-delivery-mode"
              aria-describedby={describedBy}
              value={draft.delivery_mode}
              disabled={saving}
              onChange={(event) => onDraftChange("delivery_mode", event.currentTarget.value as DeliveryMode)}
            >
              {deliveryModes.map((mode) => (
                <option key={mode} value={mode}>
                  {t(deliveryModeMessageKeys[mode])}
                </option>
              ))}
            </select>
          )}
        </SettingRow>
      </section>

      <section data-slot="card" data-verification-section aria-labelledby="verification-challenge-title">
        <div data-verification-section-heading>
          <h2 id="verification-challenge-title">{t("verification.challenge.title")}</h2>
          <p>{t("verification.challenge.description")}</p>
        </div>
        <SettingRow
          field="verify_mode"
          controlID="verification-mode"
          labelKey="verification.challenge.label"
          descriptionKey={verifyModeDescriptionKeys[draft.verify_mode]}
          source={settings.verify_mode.source}
          errorKey={errors.verify_mode}
          onRestore={onRestore}
        >
          {(describedBy) => (
            <select
              id="verification-mode"
              aria-describedby={describedBy}
              value={draft.verify_mode}
              disabled={saving}
              onChange={(event) => onDraftChange("verify_mode", event.currentTarget.value as VerifyMode)}
            >
              {verifyModes.map((mode) => (
                <option key={mode} value={mode}>
                  {t(verifyModeMessageKeys[mode])}
                </option>
              ))}
            </select>
          )}
        </SettingRow>
      </section>

      <section data-slot="card" data-verification-section aria-labelledby="verification-timing-title">
        <div data-verification-section-heading>
          <h2 id="verification-timing-title">{t("verification.timing.title")}</h2>
          <p>{t("verification.timing.description")}</p>
        </div>
        <NumericSetting
          field="timeout_seconds"
          controlID="verification-timeout-seconds"
          labelKey="verification.timing.timeout.label"
          descriptionKey="verification.timing.timeout.description"
          source={settings.timeout_seconds.source}
          errorKey={errors.timeout_seconds}
          value={draft.timeout_seconds}
          saving={saving}
          onChange={(value) => onDraftChange("timeout_seconds", value)}
          onRestore={onRestore}
        />
        <NumericSetting
          field="verify_max_fails"
          controlID="verification-max-fails"
          labelKey="verification.timing.maxFails.label"
          descriptionKey="verification.timing.maxFails.description"
          source={settings.verify_max_fails.source}
          errorKey={errors.verify_max_fails}
          value={draft.verify_max_fails}
          saving={saving}
          onChange={(value) => onDraftChange("verify_max_fails", value)}
          onRestore={onRestore}
        />
        <NumericSetting
          field="verify_retry_seconds"
          controlID="verification-retry-seconds"
          labelKey="verification.timing.retry.label"
          descriptionKey="verification.timing.retry.description"
          source={settings.verify_retry_seconds.source}
          errorKey={errors.verify_retry_seconds}
          value={draft.verify_retry_seconds}
          saving={saving}
          onChange={(value) => onDraftChange("verify_retry_seconds", value)}
          onRestore={onRestore}
        />
        <NumericSetting
          field="ban_seconds"
          controlID="verification-ban-seconds"
          labelKey="verification.timing.ban.label"
          descriptionKey="verification.timing.ban.description"
          source={settings.ban_seconds.source}
          errorKey={errors.ban_seconds}
          value={draft.ban_seconds}
          saving={saving}
          onChange={(value) => onDraftChange("ban_seconds", value)}
          onRestore={onRestore}
        />
        <NumericSetting
          field="mute_seconds"
          controlID="verification-mute-seconds"
          labelKey="verification.timing.mute.label"
          descriptionKey="verification.timing.mute.description"
          source={settings.mute_seconds.source}
          errorKey={errors.mute_seconds}
          value={draft.mute_seconds}
          saving={saving}
          onChange={(value) => onDraftChange("mute_seconds", value)}
          onRestore={onRestore}
        />
      </section>

      <section data-slot="card" data-verification-section aria-labelledby="verification-invited-title">
        <div data-verification-section-heading>
          <h2 id="verification-invited-title">{t("verification.invited.title")}</h2>
          <p>{t("verification.invited.description")}</p>
        </div>
        <SettingRow
          field="verify_invited"
          controlID="verification-invited-members"
          labelKey="verification.invited.label"
          descriptionKey="verification.invited.settingDescription"
          source={settings.verify_invited.source}
          errorKey={errors.verify_invited}
          onRestore={onRestore}
        >
          {(describedBy) => (
            <input
              id="verification-invited-members"
              data-slot="switch"
              type="checkbox"
              role="switch"
              aria-checked={draft.verify_invited}
              aria-describedby={describedBy}
              checked={draft.verify_invited}
              disabled={saving}
              onChange={(event) => onDraftChange("verify_invited", event.currentTarget.checked)}
            />
          )}
        </SettingRow>
      </section>

      <footer data-slot="card" data-verification-savebar>
        <p>{t(hasChanges ? "verification.save.dirty" : "verification.save.clean")}</p>
        <button
          type="submit"
          data-slot="button"
          data-variant="primary"
          aria-disabled={saving ? "true" : undefined}
          disabled={!hasChanges}
        >
          {t(saving ? "verification.actions.saving" : "verification.actions.save")}
        </button>
      </footer>
    </form>
  );
}
