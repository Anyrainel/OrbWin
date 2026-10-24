import { useEffect, useState } from "react";

// Appearance is a per-user UI preference only; it never touches engine state.
// "system" follows prefers-color-scheme through CSS without any attribute.
export type ThemePreference = "system" | "light" | "dark";
const key = "orbwin.theme";

function stored(): ThemePreference {
  try {
    const value = window.localStorage.getItem(key);
    return value === "light" || value === "dark" ? value : "system";
  } catch {
    return "system";
  }
}

export function useTheme() {
  const [theme, setTheme] = useState<ThemePreference>(stored);
  useEffect(() => {
    const root = document.documentElement;
    if (theme === "system") delete root.dataset.theme;
    else root.dataset.theme = theme;
    try {
      if (theme === "system") window.localStorage.removeItem(key);
      else window.localStorage.setItem(key, theme);
    } catch {
      /* Storage can be unavailable; the choice still applies for this session. */
    }
  }, [theme]);
  return [theme, setTheme] as const;
}
