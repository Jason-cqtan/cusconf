// UI preferences kept in the browser: color theme and language.

export type Theme = "system" | "light" | "dark";
export type Lang = "zh" | "en";

const THEME_KEY = "cusconf.theme";
const LANG_KEY = "cusconf.lang";

const read = (k: string) => { try { return localStorage.getItem(k); } catch { return null; } };
const write = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* storage unavailable */ } };

// --- theme ---

const media = window.matchMedia("(prefers-color-scheme: dark)");
const listeners = new Set<() => void>();

export function getTheme(): Theme {
  const v = read(THEME_KEY);
  return v === "light" || v === "dark" ? v : "system";
}

function applyTheme() {
  const t = getTheme();
  const dark = t === "dark" || (t === "system" && media.matches);
  document.documentElement.classList.toggle("dark", dark);
  document.documentElement.style.colorScheme = dark ? "dark" : "light";
}

export function setTheme(t: Theme) {
  write(THEME_KEY, t);
  applyTheme();
  listeners.forEach((f) => f());
}

export function subscribeTheme(f: () => void) {
  listeners.add(f);
  return () => { listeners.delete(f); };
}

/** Applies the saved theme now and follows the system setting while on "system". */
export function initTheme() {
  applyTheme();
  media.addEventListener("change", applyTheme);
}

// --- language ---

export function getLang(): Lang {
  const v = read(LANG_KEY);
  if (v === "zh" || v === "en") return v;
  return navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en";
}

/** Switching reloads the page, so text computed at module load is rebuilt too. */
export function setLang(l: Lang) {
  write(LANG_KEY, l);
  location.reload();
}
