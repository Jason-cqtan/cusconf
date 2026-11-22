import type { RuleSet } from "../api";
import { acl4ssrFiles } from "./acl4ssrFiles";
import { tr } from "../i18n";

/** A repository of ready-made rule lists. */
export interface PresetSource {
  id: string;
  name: string;
  repo: string;
  /** Frequently used lists shown first, with a short description. */
  common: { name: string; label: string }[];
  /** Every list the source offers. */
  all: string[];
  build: (name: string) => RuleSet;
  /** Lists made only of IP rules: their RULE-SET line gets no-resolve. */
  ipOnly: Set<string>;
}

// --- blackmatrix7/ios_rule_script: separate Surge (.list) and Clash (.yaml) builds ---

const bm7Names = [
  "OpenAI", "Claude", "Gemini", "Copilot", "Netflix", "Disney", "YouTube", "Spotify", "TikTok", "HBO", "PrimeVideo", "Hulu",
  "Telegram", "X", "Facebook", "Instagram", "Google", "GitHub", "Microsoft", "Apple", "AppleMedia", "Steam", "Epic",
  "PayPal", "Speedtest", "Advertising", "Privacy", "Global", "ChinaMax", "China", "Lan",
];
// Display name -> folder name, where they differ.
const bm7Folder: Record<string, string> = { X: "Twitter" };

export function bm7(name: string): RuleSet {
  const base = "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/";
  const dir = bm7Folder[name] ?? name;
  return { name, behavior: "classical", surgeUrl: `${base}Surge/${dir}/${dir}.list`, stashUrl: `${base}Clash/${dir}/${dir}.yaml`, stashFormat: "yaml", interval: 86400 };
}

// --- ACL4SSR: one classical .list per rule set, readable by both Surge (RULE-SET) and Stash (format: text) ---

export function acl4ssr(name: string): RuleSet {
  const url = `https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/${acl4ssrFiles[name]}`;
  return { name, behavior: "classical", surgeUrl: url, stashUrl: url, stashFormat: "text", interval: 86400 };
}

// The lists behind the widely used ACL4SSR_Online config, in its usual rule order.
const aclCommon: [string, string][] = [
  ["LocalAreaNetwork", tr("局域网 / 内网直连")],
  ["UnBan", tr("解除误判（直连）")],
  ["BanAD", tr("广告拦截")],
  ["BanProgramAD", tr("应用内广告拦截")],
  ["GoogleFCM", tr("谷歌推送 FCM")],
  ["GoogleCN", tr("谷歌中国服务（直连）")],
  ["SteamCN", tr("Steam 国内 CDN（直连）")],
  ["Microsoft", tr("微软服务")],
  ["Apple", tr("苹果服务")],
  ["Telegram", "Telegram"],
  ["AI", tr("AI 服务合集")],
  ["OpenAi", "OpenAI / ChatGPT"],
  ["Claude", "Claude"],
  ["Gemini", "Gemini"],
  ["Netflix", "Netflix"],
  ["DisneyPlus", "Disney+"],
  ["YouTube", "YouTube"],
  ["Spotify", "Spotify"],
  ["TikTok", "TikTok"],
  ["Twitter", "X / Twitter"],
  ["Github", "GitHub"],
  ["Steam", "Steam"],
  ["Epic", "Epic Games"],
  ["Nintendo", tr("任天堂")],
  ["Bilibili", tr("哔哩哔哩")],
  ["BilibiliHMT", tr("哔哩哔哩港澳台")],
  ["ProxyMedia", tr("国外媒体")],
  ["ChinaMedia", tr("国内媒体（直连）")],
  ["ProxyGFWlist", tr("代理列表（GFW）")],
  ["ProxyLite", tr("精简代理列表")],
  ["ChinaDomain", tr("国内域名（直连）")],
  ["ChinaCompanyIp", tr("国内公司 IP（直连）")],
  ["ChinaIp", tr("中国 IP（直连）")],
  ["Download", tr("下载类（直连）")],
];

// ACL4SSR first: it is the default source in the picker.
export const presetSources: PresetSource[] = [
  {
    id: "acl4ssr",
    name: "ACL4SSR",
    repo: "ACL4SSR/ACL4SSR",
    common: aclCommon.filter(([n]) => acl4ssrFiles[n]).map(([name, label]) => ({ name, label })),
    all: Object.keys(acl4ssrFiles),
    build: acl4ssr,
    ipOnly: new Set(["ChinaIp", "ChinaIpV6", "ChinaCompanyIp", "NetflixIP", "AmazonIp", "GoogleCNProxyIP"].filter((n) => acl4ssrFiles[n])),
  },
  {
    id: "bm7",
    name: "blackmatrix7",
    repo: "blackmatrix7/ios_rule_script",
    common: bm7Names.map((n) => ({ name: n, label: n })),
    all: bm7Names,
    build: bm7,
    ipOnly: new Set(),
  },
];
