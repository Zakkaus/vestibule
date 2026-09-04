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
import type { IconName } from "../icons";
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

  return (
    <div data-utility-controls data-variant={variant}>
      <div data-utility-control>
        {chrome ? null : <span id={themeLabelId}>{t("theme.label")}</span>}
        <AppSelect
          aria-label={chrome ? t("theme.label") : undefined}
          aria-labelledby={chrome ? undefined : themeLabelId}
          icon={themeIcons[theme]}
          value={theme}
          options={themeOptions}
          onValueChange={changeTheme}
        />
      </div>
      <div data-utility-control>
        {chrome ? null : <span id={localeLabelId}>{t("locale.label")}</span>}
        <AppSelect
          aria-label={chrome ? t("locale.label") : undefined}
          aria-labelledby={chrome ? undefined : localeLabelId}
          icon="languages"
          value={locale}
          options={localeOptions}
          onValueChange={changeLocale}
        />
      </div>
    </div>
  );
}
