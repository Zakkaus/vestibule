import { useEffect, useState } from "react";
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

const localeLabelKeys: Record<AppLocale, string> = {
  "zh-CN": "locale.zhCN",
  "zh-TW": "locale.zhTW",
  en: "locale.en"
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

  return (
    <div data-utility-controls>
      <label data-utility-control>
        <span>{t("theme.label")}</span>
        <select
          aria-label={t("theme.label")}
          value={theme}
          onChange={(event) => changeTheme(event.currentTarget.value)}
        >
          {themePreferences.map((preference) => (
            <option key={preference} value={preference}>
              {t(themeLabelKeys[preference])}
            </option>
          ))}
        </select>
      </label>
      <label data-utility-control>
        <span>{t("locale.label")}</span>
        <select
          aria-label={t("locale.label")}
          value={locale}
          onChange={(event) => changeLocale(event.currentTarget.value)}
        >
          {locales.map((optionLocale) => (
            <option key={optionLocale} value={optionLocale}>
              {t(localeLabelKeys[optionLocale])}
            </option>
          ))}
        </select>
      </label>
    </div>
  );
}
