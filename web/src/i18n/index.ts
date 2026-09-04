import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import en from "./locales/en.json";
import zhCN from "./locales/zh-CN.json";
import zhTW from "./locales/zh-TW.json";

export const locales = ["zh-CN", "zh-TW", "en"] as const;

export type AppLocale = (typeof locales)[number];

export const DEFAULT_LOCALE: AppLocale = "zh-CN";
export const LOCALE_STORAGE_KEY = "verify-console-locale";

// The theme control has always offered "follow the system". The language control
// offered three languages and nothing else, so choosing one was a one-way door:
// there was no way back to following the browser, and a reader who tried the
// other two out of curiosity was stuck with whichever they left it on.
export const localePreferences = ["system", ...locales] as const;

export type LocalePreference = (typeof localePreferences)[number];

export function isAppLocale(value: string): value is AppLocale {
  return locales.includes(value as AppLocale);
}

export function isLocalePreference(value: string): value is LocalePreference {
  return localePreferences.includes(value as LocalePreference);
}

/** What the browser asks for, in the order it asks. */
export function localeFromBrowser(): AppLocale {
  const requested = navigator.languages?.length ? navigator.languages : [navigator.language];
  for (const language of requested) {
    const tag = language.toLowerCase();
    if (tag === "zh-tw" || tag === "zh-hk" || tag === "zh-mo" || tag.startsWith("zh-hant")) {
      return "zh-TW";
    }
    if (tag.startsWith("en")) {
      return "en";
    }
    if (tag.startsWith("zh")) {
      return "zh-CN";
    }
  }
  return DEFAULT_LOCALE;
}

export function readLocalePreference(): LocalePreference {
  try {
    const stored = localStorage.getItem(LOCALE_STORAGE_KEY);
    if (stored !== null && isLocalePreference(stored)) {
      return stored;
    }
  } catch {
    return "system";
  }
  return "system";
}

function initialLocale(): AppLocale {
  const language = document.documentElement.lang;
  return isAppLocale(language) ? language : DEFAULT_LOCALE;
}

i18n.use(initReactI18next).init({
  resources: {
    "zh-CN": { translation: zhCN },
    "zh-TW": { translation: zhTW },
    en: { translation: en }
  },
  lng: initialLocale(),
  fallbackLng: DEFAULT_LOCALE,
  interpolation: {
    escapeValue: false
  },
  returnNull: false
});

export function setAppLocale(preference: LocalePreference): Promise<unknown> {
  const locale = preference === "system" ? localeFromBrowser() : preference;
  document.documentElement.lang = locale;

  try {
    if (preference === "system") {
      localStorage.removeItem(LOCALE_STORAGE_KEY);
    } else {
      localStorage.setItem(LOCALE_STORAGE_KEY, preference);
    }
  } catch {
    return i18n.changeLanguage(locale);
  }

  return i18n.changeLanguage(locale);
}

export default i18n;
