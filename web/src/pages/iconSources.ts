/** Where the icon picker gets its icons. Lists are fetched in the browser and cached. */
export interface IconSource {
  id: string;
  name: string;
  /** Link to the project, shown under the grid. */
  home?: string;
  load: () => Promise<Icon[]>;
}
export interface Icon { name: string; url: string }

const QURE = "https://raw.githubusercontent.com/Koolson/Qure/master/IconSet/Color/";
const commonNames = [
  "Proxy", "Auto", "Available", "Static", "Final", "Direct", "Reject", "Advertising", "Global", "China",
  "Hong_Kong", "Taiwan", "Japan", "Singapore", "United_States", "Korea", "United_Kingdom", "Germany", "Russia", "India",
  "ChatGPT", "Netflix", "YouTube", "Disney+", "Spotify", "TikTok", "Telegram", "Twitter", "Google_Search", "Apple",
  "Microsoft", "GitHub", "Steam", "Speedtest", "HBO", "Prime_Video", "Hulu", "Streaming", "Game",
];

/**
 * Icon gallery JSON as used by Quantumult X / Loon / Stash icon subscriptions:
 * {"name": "...", "icons": [{"name": "...", "url": "..."}]}
 */
async function gallery(url: string): Promise<Icon[]> {
  const r = await fetch(url);
  if (!r.ok) throw new Error(`HTTP ${r.status}`);
  const d = await r.json();
  if (!Array.isArray(d?.icons)) throw new Error("不是图标订阅 JSON（缺少 icons 数组）");
  return d.icons
    .filter((i: Icon) => typeof i?.url === "string" && i.url)
    .map((i: Icon) => ({ name: String(i.name ?? "").replace(/\.png$/i, ""), url: i.url }));
}

// LobeHub's AI / tech brand logos (Claude, Gemini, Grok, DeepSeek …) as PNG, served by jsDelivr.
const LOBE = "@lobehub/icons-static-png@1.97.1";
async function lobehub(): Promise<Icon[]> {
  const r = await fetch(`https://data.jsdelivr.com/v1/packages/npm/${LOBE}?structure=flat`);
  if (!r.ok) throw new Error(`HTTP ${r.status}`);
  const files: string[] = ((await r.json()).files ?? []).map((f: { name: string }) => f.name).filter((n: string) => n.startsWith("/light/") && n.endsWith(".png"));
  const set = new Set(files);
  const out: Icon[] = [];
  for (const f of files) {
    const base = f.slice("/light/".length, -".png".length);
    // One icon per brand: the colored mark when there is one; skip wordmarks.
    if (/-(text|brand|color)/.test(base)) continue;
    const color = `/light/${base}-color.png`;
    out.push({ name: base, url: `https://cdn.jsdelivr.net/npm/${LOBE}${set.has(color) ? color : f}` });
  }
  return out;
}

export const builtinIconSources: IconSource[] = [
  { id: "common", name: "常用", home: "https://github.com/Koolson/Qure", load: async () => commonNames.map((n) => ({ name: n, url: `${QURE}${n}.png` })) },
  { id: "qure", name: "Qure 全部", home: "https://github.com/Koolson/Qure", load: () => gallery("https://raw.githubusercontent.com/Koolson/Qure/master/Other/QureColor.json") },
  { id: "lobehub", name: "AI 品牌（LobeHub）", home: "https://github.com/lobehub/lobe-icons", load: lobehub },
  { id: "semporia", name: "手绘（Semporia）", home: "https://github.com/Semporia/Hand-Painted-icon", load: () => gallery("https://raw.githubusercontent.com/Semporia/Hand-Painted-icon/master/Semporia.json") },
  { id: "mini", name: "mini（Orz-3）", home: "https://github.com/Orz-3/mini", load: () => gallery("https://raw.githubusercontent.com/Orz-3/mini/master/mini.json") },
];

/** A user-added icon subscription JSON URL. */
export const customIconSource = (url: string): IconSource => ({ id: `custom:${url}`, name: decodeURIComponent(url.split("/").pop() || url), home: url, load: () => gallery(url) });

const cache = new Map<string, Promise<Icon[]>>();
export function loadIcons(src: IconSource): Promise<Icon[]> {
  let p = cache.get(src.id);
  if (!p) {
    p = src.load();
    p.catch(() => cache.delete(src.id)); // retry on the next open
    cache.set(src.id, p);
  }
  return p;
}

// Per-browser conveniences: custom sources and the last source used.
const KEY_CUSTOM = "cusconf.iconSources";
const KEY_LAST = "cusconf.iconSource";
export const prefs = {
  custom(): string[] { try { return JSON.parse(localStorage.getItem(KEY_CUSTOM) ?? "[]"); } catch { return []; } },
  setCustom(v: string[]) { try { localStorage.setItem(KEY_CUSTOM, JSON.stringify(v)); } catch { /* storage unavailable */ } },
  last(): string { try { return localStorage.getItem(KEY_LAST) ?? "common"; } catch { return "common"; } },
  setLast(id: string) { try { localStorage.setItem(KEY_LAST, id); } catch { /* storage unavailable */ } },
};
