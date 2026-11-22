// Minimal i18n: the Chinese UI text is the key, en.ts maps it to English.
// Placeholders {0}, {1}, … are filled from the extra arguments.
import { en } from "./en";
import { getLang } from "./prefs";

export const lang = getLang();
document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";

export function tr(zh: string, ...args: unknown[]): string {
  let s = lang === "en" ? (en[zh] ?? zh) : zh;
  if (args.length) s = s.replace(/\{(\d+)\}/g, (m, i) => (+i < args.length ? String(args[+i]) : m));
  return s;
}

// Messages that come from the server with values inside them.
const patterns: [RegExp, string][] = [
  [/^商店货币 (.+)$/, "Store currency {0}"],
  [/^节点服务器被解析为 (\S+)（本机代理软件的 Fake IP）.*$/, "The node server resolved to {0} (a Fake IP of a proxy app on this machine). Connections follow that app's rules and may go through another proxy, causing timeouts or wrong results. Let the mihomo process (node tests connect through it) or this domain go direct in Surge / Clash."],
];

/** Translates text produced by the server (test notes, errors, IP labels). */
export function trMsg(s?: string | null): string {
  if (!s || lang !== "en") return s ?? "";
  if (en[s] !== undefined) return en[s];
  for (const [re, out] of patterns) {
    const m = s.match(re);
    if (m) return tr(out, ...m.slice(1));
  }
  return s;
}
