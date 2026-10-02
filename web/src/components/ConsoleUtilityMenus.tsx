import More from "@react-spectrum/s2/icons/More";
import { ActionButton } from "@react-spectrum/s2/ActionButton";
import { Menu, MenuItem, MenuTrigger, SubmenuTrigger } from "@react-spectrum/s2/Menu";
import { Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useTranslation } from "react-i18next";
import { Icon } from "../icons";

type Choice = Readonly<{ value: string; label: string }>;
export function ConsoleUtilityMenus({ locale, theme, locales, themes, onLocale, onTheme }: Readonly<{
  locale: string; theme: string; locales: readonly Choice[]; themes: readonly Choice[];
  onLocale: (value: string) => void; onTheme: (value: string) => void;
}>) {
  const { t } = useTranslation();
  const language = <Menu aria-label={t("locale.label")} selectionMode="single" selectedKeys={[locale]} onAction={key => onLocale(String(key))}>
    {locales.map(item => <MenuItem id={item.value} key={item.value}>{item.label}</MenuItem>)}
  </Menu>;
  const appearance = <Menu aria-label={t("theme.label")} selectionMode="single" selectedKeys={[theme]} onAction={key => onTheme(String(key))}>
    {themes.map(item => <MenuItem id={item.value} key={item.value}>{item.label}</MenuItem>)}
  </Menu>;
  return <>
    <div data-console-utilities="desktop" className={style({ display: { default: "flex", "@media (max-width: 1023px)": "none" }, alignItems: "center", gap: 4 })}>
      <MenuTrigger><ActionButton data-preference="locale" size="M" isQuiet aria-label={t("locale.label")}><Icon name="languages" /></ActionButton>{language}</MenuTrigger>
      <MenuTrigger><ActionButton data-preference="theme" size="M" isQuiet aria-label={t("theme.label")}><Icon name="sun" /></ActionButton>{appearance}</MenuTrigger>
    </div>
    <div data-console-utilities="mobile" className={style({ display: { default: "none", "@media (max-width: 1023px)": "block" } })}>
      <MenuTrigger><ActionButton size="M" isQuiet aria-label={t("shell.more")}><More /></ActionButton>
        <Menu aria-label={t("shell.more")}>
          <SubmenuTrigger><MenuItem data-preference="locale" textValue={t("locale.label")}><Icon name="languages" /><Text slot="label">{t("locale.label")}</Text></MenuItem>{language}</SubmenuTrigger>
          <SubmenuTrigger><MenuItem data-preference="theme" textValue={t("theme.label")}><Icon name="sun" /><Text slot="label">{t("theme.label")}</Text></MenuItem>{appearance}</SubmenuTrigger>
        </Menu>
      </MenuTrigger>
    </div>
  </>;
}
