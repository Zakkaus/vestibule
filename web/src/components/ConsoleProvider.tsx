import { Provider } from "@react-spectrum/s2/Provider";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useSyncExternalStore, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";

import { readThemePreference, THEME_PREFERENCE_CHANGE_EVENT } from "../app/theme";

function subscribeTheme(listener: () => void): () => void {
  window.addEventListener(THEME_PREFERENCE_CHANGE_EVENT, listener);
  return () => window.removeEventListener(THEME_PREFERENCE_CHANGE_EVENT, listener);
}

// The content panel is layer-2. A card on it takes layer-1, one step back towards the
// page ground, which is the same tint the home page's rows use: a card that was also
// layer-2 was white on white and had only its border to show it was there.
const legacySurfaceStyles = style({
  "--background": { type: "backgroundColor", value: "layer-1" },
  "--card": { type: "backgroundColor", value: "layer-1" },
  "--popover": { type: "backgroundColor", value: "layer-2" },
  "--surface-raised": { type: "backgroundColor", value: "layer-2" },
  "--muted": { type: "backgroundColor", value: "layer-1" }
});

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
      background="base"
      styles={legacySurfaceStyles}
      router={{ navigate: (path, options) => { void navigate(path, options); } }}
    >
      {children}
    </Provider>
  );
}
