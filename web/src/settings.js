const KEY = "scheduler_settings_v1";
const DEFAULTS = { theme: "dark", pollMs: 6000 };

export function getSettings() {
  try {
    return { ...DEFAULTS, ...JSON.parse(localStorage.getItem(KEY) || "{}") };
  } catch {
    return DEFAULTS;
  }
}

export function saveSettings(patch) {
  const next = { ...getSettings(), ...patch };
  localStorage.setItem(KEY, JSON.stringify(next));
  return next;
}

export function applyTheme(theme) {
  document.documentElement.setAttribute("data-theme", theme);
}
