import { Button } from "@react-spectrum/s2/Button";
import { Footer, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useTranslation } from "react-i18next";

export function SettingsSaveFooter({ dirtyCount, pending, disabled, onDiscard, saveLabel, savingLabel, cleanLabel, ...attributes }: Readonly<{
  dirtyCount: number; pending: boolean; disabled?: boolean; onDiscard: () => void;
  saveLabel: string; savingLabel: string; cleanLabel: string;
}> & Record<`data-${string}`, string | boolean | undefined>) {
  const { t } = useTranslation();
  return (
    <Footer {...attributes} styles={style({ display: "flex", flexWrap: "wrap", alignItems: "start", gap: 12, minWidth: 0 })}>
      <Text aria-live="polite" styles={style({ width: "full" })}>{dirtyCount ? t("drafts.dirtyCount", { total: dirtyCount }) : cleanLabel}</Text>
      <Button type="submit" variant="accent" size="M" styles={style({ flexShrink: 0, maxWidth: "full" })} isDisabled={!dirtyCount || disabled || pending} isPending={pending}>
        <Text styles={style({ whiteSpace: "nowrap" })}>{pending ? savingLabel : saveLabel}</Text>
      </Button>
      {dirtyCount ? <Button variant="secondary" size="M" styles={style({ flexShrink: 0, maxWidth: "full" })} isDisabled={pending} onPress={onDiscard}>
        <Text styles={style({ whiteSpace: "nowrap" })}>{t("moderation.actions.discard")}</Text>
      </Button> : null}
    </Footer>
  );
}
