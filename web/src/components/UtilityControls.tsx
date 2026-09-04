import { useEffect, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  DEFAULT_LOCALE,
  isAppLocale,
  locales,
  setAppLocale,
  type AppLocale
} from "../i18n";
import i18n from "../i18n";
import {
  applyThemePreference,
  THEME_PREFERENCE_CHANGE_EVENT,
  readThemePreference,
  themePreferences,
  type ThemePreference
} from "../app/theme";
import type { IconName } from "../icons";
import { AppSelect } from "./AppSelect";

const localeLabelKeys: Record<AppLocale, string> = {
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

export function UtilityControls() {
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

  const resolvedLocale = i18n.resolvedLanguage ?? i18n.language;
  const locale = isAppLocale(resolvedLocale) ? resolvedLocale : DEFAULT_LOCALE;

  function changeTheme(nextTheme: string): void {
    if (!themePreferences.includes(nextTheme as ThemePreference)) {
      return;
    }

    const preference = nextTheme as ThemePreference;
    applyThemePreference(preference);
  }

  function changeLocale(nextLocale: string): void {
    if (!isAppLocale(nextLocale)) {
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
  const localeOptions = locales.map((optionLocale) => ({
    label: t(localeLabelKeys[optionLocale]),
    value: optionLocale
  }));


  return (
    <div data-utility-controls>
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
