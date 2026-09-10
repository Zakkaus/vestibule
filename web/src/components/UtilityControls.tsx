import { Picker, PickerItem } from "@react-spectrum/s2/Picker";
import { Content, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useEffect, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  isLocalePreference,
  localePreferences,
  readLocalePreference,
  setAppLocale,
  type LocalePreference
} from "../i18n";
import {
  applyThemePreference,
  THEME_PREFERENCE_CHANGE_EVENT,
  readThemePreference,
  themePreferences,
  type ThemePreference
} from "../app/theme";
import { Icon, type IconName } from "../icons";
import { useConsoleSize } from "./ConsoleProvider";
import { AppSelect } from "./AppSelect";

const localeLabelKeys: Record<LocalePreference, string> = {
  system: "locale.system",
  "zh-CN": "locale.zhCN",
  "zh-TW": "locale.zhTW",
  en: "locale.en"
};

// The glyph belongs to the preference, not to the control: the theme icon has
// to say which of the three is chosen, and a Record over the same closed set
// that defines the preferences makes a fourth one a compile error rather than a
// control that silently keeps the old icon.
const themeIcons: Record<ThemePreference, IconName> = {
  system: "monitor",
  light: "sun",
  dark: "moon"
};

const themeLabelKeys: Record<ThemePreference, string> = {
  system: "theme.system",
  light: "theme.light",
  dark: "theme.dark"
};
// Wide enough for the longest value in each supported language: at 160 the
// browser-default language read as "跟随浏…".
const chromePickerLayout = style({
  width: {
    default: 176,
    "@media (max-width: 48rem)": "full"
  },
  minWidth: 0
});

type UtilityControlsProps = Readonly<{
  /** "chrome" drops the label above each control: in a corner of the screen the
      glyph and the current value already say which control this is, and the two
      stacked names said more about themselves than the card below them said
      about what to do next. "labelled" is the settings-page treatment, where the
      control is the content of the screen and its name belongs on screen. */
  variant?: "labelled" | "chrome";
}>;

export function UtilityControls({ variant = "labelled" }: UtilityControlsProps) {
  const { t } = useTranslation();
  const [theme, setTheme] = useState<ThemePreference>(readThemePreference);
  const size = useConsoleSize("L");

  useEffect(() => {
    const synchronizeTheme = () => {
      setTheme(readThemePreference());
    };

    window.addEventListener(THEME_PREFERENCE_CHANGE_EVENT, synchronizeTheme);
    return () => {
      window.removeEventListener(THEME_PREFERENCE_CHANGE_EVENT, synchronizeTheme);
    };
  }, []);

  const locale = readLocalePreference();

  function changeTheme(nextTheme: string): void {
    if (!themePreferences.includes(nextTheme as ThemePreference)) {
      return;
    }

    const preference = nextTheme as ThemePreference;
    applyThemePreference(preference);
  }

  function changeLocale(nextLocale: string): void {
    if (!isLocalePreference(nextLocale)) {
      return;
    }

    void setAppLocale(nextLocale);
  }
  const themeLabelId = useId();
  const localeLabelId = useId();
  const themeOptions = themePreferences.map((preference) => ({
    label: t(themeLabelKeys[preference]),
    value: preference
  }));
  const localeOptions = localePreferences.map((preference) => ({
    label: t(localeLabelKeys[preference]),
    value: preference
  }));


  const chrome = variant === "chrome";

  if (chrome) {
    return (
      <Content data-utility-controls data-variant="chrome" styles={style({ display: { default: "flex", "@media (max-width: 48rem)": "contents" }, alignItems: "center", gap: 8, minWidth: 0 })}>
        <Content data-utility-control styles={style({ minWidth: 0 })}>
          <Picker
            aria-label={t("theme.label")}
            selectedKey={theme}
            onSelectionChange={(key) => { if (key !== null) changeTheme(String(key)); }}
            items={themeOptions}
            size={size}
            align="end"
            data-console-control
            data-control-size={size}
            styles={chromePickerLayout}
            renderValue={(items) => (
              <Content data-console-choice-value styles={style({ display: "flex", alignItems: "center", gap: 8, minWidth: 0 })}>
                <Icon name={themeIcons[theme]} /><Text styles={style({ minWidth: 0, overflow: "hidden", whiteSpace: "nowrap", textOverflow: "ellipsis" })}>{items[0]?.label}</Text>
              </Content>
            )}
          >
            {(option) => <PickerItem id={option.value}>{option.label}</PickerItem>}
          </Picker>
        </Content>
        <Content data-utility-control styles={style({ minWidth: 0 })}>
          <Picker
            aria-label={t("locale.label")}
            selectedKey={locale}
            onSelectionChange={(key) => { if (key !== null) changeLocale(String(key)); }}
            items={localeOptions}
            size={size}
            align="end"
            data-console-control
            data-control-size={size}
            styles={chromePickerLayout}
            renderValue={(items) => (
              <Content data-console-choice-value styles={style({ display: "flex", alignItems: "center", gap: 8, minWidth: 0 })}>
                <Icon name="languages" /><Text styles={style({ minWidth: 0, overflow: "hidden", whiteSpace: "nowrap", textOverflow: "ellipsis" })}>{items[0]?.label}</Text>
              </Content>
            )}
          >
            {(option) => <PickerItem id={option.value}>{option.label}</PickerItem>}
          </Picker>
        </Content>
      </Content>
    );
  }

  return (
    <div data-utility-controls data-variant={variant}>
      <div data-utility-control>
        <span id={themeLabelId}>{t("theme.label")}</span>
        <AppSelect
          aria-labelledby={themeLabelId}
          icon={themeIcons[theme]}
          value={theme}
          options={themeOptions}
          onValueChange={changeTheme}
        />
      </div>
      <div data-utility-control>
        <span id={localeLabelId}>{t("locale.label")}</span>
        <AppSelect
          aria-labelledby={localeLabelId}
          icon="languages"
          value={locale}
          options={localeOptions}
          onValueChange={changeLocale}
        />
      </div>
    </div>
  );
}
