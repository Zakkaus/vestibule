import { Button, ButtonGroup, Footer, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useTranslation } from "react-i18next";
import { useConsoleSize } from "./ConsoleProvider";

const phoneActionWidth = style({ width: { "@media (max-width: 48rem)": "full" } });

export function SettingsSaveFooter({ dirtyCount, pending, disabled, onDiscard, saveLabel, savingLabel, cleanLabel, ...attributes }: Readonly<{
  dirtyCount: number; pending: boolean; disabled?: boolean; onDiscard: () => void;
  saveLabel: string; savingLabel: string; cleanLabel: string;
}> & Record<`data-${string}`, string | boolean | undefined>) {
  const { t } = useTranslation();
  const size = useConsoleSize("L");
  return (
    <Footer {...attributes} styles={style({ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 12, padding: 16, minWidth: 0 })}>
      <Text>{dirtyCount ? t("drafts.dirtyCount", { total: dirtyCount }) : cleanLabel}</Text>
      <ButtonGroup styles={phoneActionWidth}>
        <Button variant="secondary" size={size} styles={phoneActionWidth} isDisabled={!dirtyCount || pending} onPress={onDiscard}>{t("drafts.discard")}</Button>
        <Button type="submit" variant="accent" size={size} styles={phoneActionWidth} isDisabled={!dirtyCount || disabled} isPending={pending}>{pending ? savingLabel : saveLabel}</Button>
      </ButtonGroup>
    </Footer>
  );
}
