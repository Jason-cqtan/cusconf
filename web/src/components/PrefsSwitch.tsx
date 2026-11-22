import { useSyncExternalStore } from "react";
import { Monitor, Moon, Sun } from "lucide-react";
import { lang, tr } from "../i18n";
import { getTheme, setLang, setTheme, subscribeTheme, type Theme } from "../prefs";
import { cx } from "./ui";

const themes: { id: Theme; icon: typeof Sun; label: string }[] = [
  { id: "light", icon: Sun, label: tr("白天") },
  { id: "dark", icon: Moon, label: tr("夜晚") },
  { id: "system", icon: Monitor, label: tr("跟随系统") },
];

const seg = "flex rounded-md border border-zinc-200 dark:border-zinc-700 p-0.5";
const item = (on: boolean) =>
  cx("flex h-6 min-w-7 items-center justify-center rounded px-1.5 text-xs transition-colors",
    on ? "bg-zinc-200 text-zinc-900 dark:bg-zinc-700 dark:text-zinc-100" : "text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200");

/** Theme (light / dark / system) and language (中 / EN) switches. */
export function PrefsSwitch({ className }: { className?: string }) {
  const theme = useSyncExternalStore(subscribeTheme, getTheme);
  return (
    <div className={cx("flex items-center gap-2", className)}>
      <div className={seg} role="radiogroup" aria-label={tr("外观")}>
        {themes.map((t) => (
          <button key={t.id} type="button" role="radio" aria-checked={theme === t.id} title={t.label} aria-label={t.label}
            className={item(theme === t.id)} onClick={() => setTheme(t.id)}>
            <t.icon size={13} />
          </button>
        ))}
      </div>
      <div className={seg} role="radiogroup" aria-label={tr("语言")}>
        {(["zh", "en"] as const).map((l) => (
          <button key={l} type="button" role="radio" aria-checked={lang === l} title={l === "zh" ? "中文" : "English"}
            className={item(lang === l)} onClick={() => lang !== l && setLang(l)}>
            {l === "zh" ? "中" : "EN"}
          </button>
        ))}
      </div>
    </div>
  );
}
