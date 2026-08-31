export const themePreferences = ["system", "light", "dark"] as const;

export type ThemePreference = (typeof themePreferences)[number];

export const THEME_STORAGE_KEY = "verify-console-theme";

export function readThemePreference(): ThemePreference {
  const theme = document.documentElement.dataset.theme;
  return theme === "light" || theme === "dark" ? theme : "system";
}

export function applyThemePreference(preference: ThemePreference): void {
  const root = document.documentElement;

  if (preference === "system") {
    delete root.dataset.theme;
  } else {
    root.dataset.theme = preference;
  }

  root.dataset.themePreference = preference;

  try {
    if (preference === "system") {
      localStorage.removeItem(THEME_STORAGE_KEY);
    } else {
      localStorage.setItem(THEME_STORAGE_KEY, preference);
    }
  } catch {
    return;
  }
}
