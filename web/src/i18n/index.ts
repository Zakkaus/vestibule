import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import en from "./locales/en.json";
import zhCN from "./locales/zh-CN.json";
import zhTW from "./locales/zh-TW.json";

export const locales = ["zh-CN", "zh-TW", "en"] as const;

export type AppLocale = (typeof locales)[number];

export const DEFAULT_LOCALE: AppLocale = "zh-CN";
export const LOCALE_STORAGE_KEY = "verify-console-locale";

export function isAppLocale(value: string): value is AppLocale {
  return locales.includes(value as AppLocale);
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

export function setAppLocale(locale: AppLocale): Promise<unknown> {
  document.documentElement.lang = locale;

  try {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale);
  } catch {
    return i18n.changeLanguage(locale);
  }

  return i18n.changeLanguage(locale);
}

export default i18n;
