import { Provider } from "@react-spectrum/s2/Provider";
import { useSyncExternalStore, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";

import { readThemePreference, THEME_PREFERENCE_CHANGE_EVENT } from "../app/theme";

function subscribeTheme(listener: () => void): () => void {
  window.addEventListener(THEME_PREFERENCE_CHANGE_EVENT, listener);
  return () => window.removeEventListener(THEME_PREFERENCE_CHANGE_EVENT, listener);
}

const touchQuery = "not ((hover: hover) and (pointer: fine))";

function subscribeTouch(listener: () => void): () => void {
  const media = window.matchMedia(touchQuery);
  media.addEventListener("change", listener);
  return () => media.removeEventListener("change", listener);
}

function isTouch(): boolean {
  return window.matchMedia(touchQuery).matches;
}

export function useConsoleSize(size: "M" | "L" | "XL"): "M" | "L" | "XL" {
  const touch = useSyncExternalStore(subscribeTouch, isTouch);
  return touch ? "XL" : size;
}

export function ConsoleProvider({ children }: Readonly<{ children: ReactNode }>) {
  const theme = useSyncExternalStore(subscribeTheme, readThemePreference);
  const { i18n } = useTranslation();
  const navigate = useNavigate();

  return (
    <Provider
      locale={i18n.resolvedLanguage}
      colorScheme={theme === "system" ? undefined : theme}
      router={{ navigate: (path, options) => { void navigate(path, options); } }}
    >
      {children}
    </Provider>
  );
}
