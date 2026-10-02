import { TextField } from "@react-spectrum/s2/TextField";
import { ActionButton } from "@react-spectrum/s2/ActionButton";
import { Picker, PickerItem } from "@react-spectrum/s2/Picker";
import { Switch } from "@react-spectrum/s2/Switch";
import { Tooltip, TooltipTrigger } from "@react-spectrum/s2/Tooltip";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { StatusBadge } from "../../components/StatusBadge";
import { SettingsSection } from "../../components/SettingsSection";
import { SettingsFieldRow } from "../../components/SettingsFieldRow";
import { SettingsSaveFooter } from "../../components/SettingsSaveFooter";
import { Icon } from "../../icons";
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
  dirtyCount: number;
  onDiscard: () => void;
  saveBlocked: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onDraftChange: <K extends keyof DraftSettings>(field: K, value: DraftSettings[K]) => void;
  onFieldBlur: (field: VerificationSettingField) => void;
  onRestore: (field: VerificationSettingField) => void;
}>;

type SettingRowProps = Readonly<{
  field: VerificationSettingField;
  source: SettingSource;
  saving?: boolean;
  onRestore: (field: VerificationSettingField) => void;
  children: ReactNode;
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
  onBlur: () => void;
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

function SettingRow({ field, source, saving, onRestore, children }: SettingRowProps) {
  const { t } = useTranslation();
  return (
    <SettingsFieldRow data-verification-setting={field}>
      {children}
      <StatusBadge tone="neutral">{t("verification.source.value", { source: t(sourceMessageKeys[source]) })}</StatusBadge>
      {source === "chat override" ? (
        <TooltipTrigger>
          <ActionButton size="M" isDisabled={saving} aria-label={t("verification.actions.restore")} onPress={() => onRestore(field)}><Icon name="rotateCcw" /></ActionButton>
          <Tooltip>{t("verification.actions.restore")}</Tooltip>
        </TooltipTrigger>
      ) : null}
    </SettingsFieldRow>
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
  onBlur,
  onRestore
}: NumericSettingProps) {
  const { t } = useTranslation();
  return (
    <SettingRow
      field={field}
      source={source}
      saving={saving}
      onRestore={onRestore}
    >
        <TextField
          id={controlID}
          label={t(labelKey)}
          labelPosition="top"
          description={t(descriptionKey)}
          errorMessage={errorKey ? t(errorKey) : undefined}
          validationBehavior="aria"
          type="text"
          size="M"
          styles={style({ width: "full", minWidth: 0 })}
          data-verification-number
          inputMode="numeric"
          isInvalid={Boolean(errorKey)}
          value={value}
          isDisabled={saving}
          onBlur={onBlur}
          onChange={onChange}
        />
    </SettingRow>
  );
}

export function VerificationSettingsForm({
  settings,
  draft,
  errors,
  saving,
  dirtyCount,
  onDiscard,
  saveBlocked,
  onSubmit,
  onDraftChange,
  onFieldBlur,
  onRestore
}: VerificationSettingsFormProps) {
  const { t } = useTranslation();
  const deliveryOptions = deliveryModes.map(mode => ({ label: t(deliveryModeMessageKeys[mode]), value: mode }));
  const verifyOptions = verifyModes.map(mode => ({ label: t(verifyModeMessageKeys[mode]), value: mode }));

  return (
    <form data-verification-form onSubmit={onSubmit}>
      <SettingsSection data-verification-section id="verification-delivery-title" title={t("verification.delivery.title")} description={t("verification.delivery.description")}><SettingRow
        field="delivery_mode"
        source={settings.delivery_mode.source}
        saving={saving}
        onRestore={onRestore}
      >
          <Picker
            label={t("verification.delivery.label")}
            labelPosition="top"
            id="verification-delivery-mode"
            description={t(deliveryModeDescriptionKeys[draft.delivery_mode])}
            errorMessage={errors.delivery_mode ? t(errors.delivery_mode) : undefined}
            isInvalid={Boolean(errors.delivery_mode)}
            validationBehavior="aria"
            selectedKey={draft.delivery_mode}
            isDisabled={saving}
            size="M"
            styles={style({ width: "full", minWidth: 0 })}
            items={deliveryOptions}
            onSelectionChange={key => { if (key !== null) onDraftChange("delivery_mode", key as DeliveryMode); }}
          >{option => <PickerItem id={option.value}>{option.label}</PickerItem>}</Picker>
      </SettingRow></SettingsSection>

      <SettingsSection data-verification-section id="verification-challenge-title" title={t("verification.challenge.title")} description={t("verification.challenge.description")}><SettingRow
        field="verify_mode"
        source={settings.verify_mode.source}
        saving={saving}
        onRestore={onRestore}
      >
          <Picker
            label={t("verification.challenge.label")}
            labelPosition="top"
            id="verification-mode"
            description={t(verifyModeDescriptionKeys[draft.verify_mode])}
            errorMessage={errors.verify_mode ? t(errors.verify_mode) : undefined}
            isInvalid={Boolean(errors.verify_mode)}
            validationBehavior="aria"
            selectedKey={draft.verify_mode}
            isDisabled={saving}
            size="M"
            styles={style({ width: "full", minWidth: 0 })}
            items={verifyOptions}
            onSelectionChange={key => { if (key !== null) onDraftChange("verify_mode", key as VerifyMode); }}
          >{option => <PickerItem id={option.value}>{option.label}</PickerItem>}</Picker>
      </SettingRow></SettingsSection>

      <SettingsSection data-verification-section id="verification-timing-title" title={t("verification.timing.title")} description={t("verification.timing.description")}><NumericSetting
        field="timeout_seconds"
        controlID="verification-timeout-seconds"
        labelKey="verification.timing.timeout.label"
        descriptionKey="verification.timing.timeout.description"
        source={settings.timeout_seconds.source}
        errorKey={errors.timeout_seconds}
        value={draft.timeout_seconds}
        saving={saving}
        onChange={(value) => onDraftChange("timeout_seconds", value)}
        onBlur={() => onFieldBlur("timeout_seconds")}
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
        onBlur={() => onFieldBlur("verify_max_fails")}
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
        onBlur={() => onFieldBlur("verify_retry_seconds")}
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
        onBlur={() => onFieldBlur("ban_seconds")}
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
        onBlur={() => onFieldBlur("mute_seconds")}
        onRestore={onRestore}
      /></SettingsSection>

      <SettingsSection data-verification-section id="verification-invited-title" title={t("verification.invited.title")} description={t("verification.invited.description")}><SettingRow
        field="verify_invited"
        source={settings.verify_invited.source}
        saving={saving}
        onRestore={onRestore}
      >
          <Switch id="verification-invited-members" size="M" isSelected={draft.verify_invited} isDisabled={saving}
            styles={style({ width: "full", minWidth: 0 })}
            description={t("verification.invited.settingDescription")} validationBehavior="aria"
            isInvalid={Boolean(errors.verify_invited)} errorMessage={errors.verify_invited ? t(errors.verify_invited) : undefined}
            onChange={value => onDraftChange("verify_invited", value)}>{t("verification.invited.label")}</Switch>
      </SettingRow></SettingsSection>

      <SettingsSaveFooter data-verification-savebar dirtyCount={dirtyCount} pending={saving} disabled={saveBlocked} onDiscard={onDiscard}
        saveLabel={t("verification.actions.save")} savingLabel={t("verification.actions.saving")}
        cleanLabel={t("verification.save.clean")} />
    </form>
  );
}
