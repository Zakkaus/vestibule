import { Button, ButtonGroup, Content, Heading, InlineAlert, Text } from "@react-spectrum/s2";
import { useTranslation } from "react-i18next";
import type { VerificationSettingField, VerificationSettings } from "./api";
import { settingsDraft, type DraftSettings } from "./form";

export const verificationFieldLabels: Record<VerificationSettingField, string> = {
  delivery_mode: "verification.delivery.label", verify_mode: "verification.challenge.label",
  timeout_seconds: "verification.timing.timeout.label", verify_max_fails: "verification.timing.maxFails.label",
  verify_retry_seconds: "verification.timing.retry.label", ban_seconds: "verification.timing.ban.label",
  mute_seconds: "verification.timing.mute.label", verify_invited: "verification.invited.label"
};

export function changedDraftFields(baseline: VerificationSettings, draft: DraftSettings, restored: ReadonlySet<VerificationSettingField>) {
  const original = settingsDraft(baseline);
  return (Object.keys(original) as VerificationSettingField[]).filter((field) => restored.has(field) || original[field] !== draft[field]);
}

export function reapplyDraft(baseline: VerificationSettings, draft: DraftSettings, latest: VerificationSettings, restored: ReadonlySet<VerificationSettingField>): DraftSettings {
  const next = { ...settingsDraft(latest) };
  for (const field of changedDraftFields(baseline, draft, restored)) {
    Object.assign(next, { [field]: draft[field] });
  }
  return next;
}

export function VerificationConflict({ baseline, draft, latest, restored, onDiscard, onReapply }: Readonly<{
  baseline: VerificationSettings; draft: DraftSettings; latest: VerificationSettings;
  restored: ReadonlySet<VerificationSettingField>; onDiscard: () => void; onReapply: () => void;
}>) {
  const { t } = useTranslation();
  const overlapping = changedDraftFields(baseline, draft, restored).filter((field) =>
    baseline[field].value !== latest[field].value || baseline[field].source !== latest[field].source);
  const display = (value: string | number | boolean) => typeof value === "boolean" ? t(value ? "drafts.enabled" : "drafts.disabled") : String(value);
  return (
    <InlineAlert variant="notice" data-verification-conflict>
      <Heading>{t("drafts.conflictTitle")}</Heading>
      <Content>
        <Text>{t("drafts.conflictDescription", { revision: latest.revision })}</Text>
        {overlapping.length ? <ul>{overlapping.map((field) => <li key={field}>
          {t("drafts.conflictField", { field: t(verificationFieldLabels[field]), latest: display(latest[field].value), mine: restored.has(field) ? t("drafts.inherited") : display(draft[field]) })}
        </li>)}</ul> : <p>{t("drafts.noOverlap")}</p>}
      </Content>
      <ButtonGroup>
        <Button variant="secondary" onPress={onDiscard}>{t("drafts.discard")}</Button>
        <Button variant="primary" onPress={onReapply}>{t("drafts.reapply")}</Button>
      </ButtonGroup>
    </InlineAlert>
  );
}
