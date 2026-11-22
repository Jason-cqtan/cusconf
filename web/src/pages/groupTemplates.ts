import type { Rule } from "../api";
import { acl4ssr, bm7 } from "./rulePresets";
import { acl4ssrFiles } from "./acl4ssrFiles";
import { uid, type EditableGroup, type EditableProfile, type EditableRule } from "./ProfileEditor";
import { tr } from "../i18n";

const QURE = "https://raw.githubusercontent.com/Koolson/Qure/master/IconSet/Color/";
const LOBE = "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-png@1.97.1/light/";

/** A service group that can be added in one click, with the rules that route to it. */
export interface GroupTemplate {
  name: string;
  icon: string;
  /** Rule sets routed to the group: ACL4SSR lists when it has them, blackmatrix7 otherwise. */
  ruleSets?: string[];
  /**
   * Extra rules in Surge syntax without the policy ("TYPE,VALUE[,no-resolve]"), for what the
   * rule sets miss. "# …" lines become the comment of the next rule. Existing rules with the
   * same type and value are left alone.
   */
  rules?: string;
  /** Offer DIRECT first (services that usually work without a proxy). */
  direct?: boolean;
}

const claudeRules = `
# Claude 补充域名
DOMAIN-SUFFIX,claude.com
DOMAIN-SUFFIX,clau.de
DOMAIN-SUFFIX,claudemcpclient.com
DOMAIN-SUFFIX,claudemcpcontent.com
DOMAIN-SUFFIX,claudeusercontent.com
# CDN 和静态资源
DOMAIN,servd-anthropic-website.b-cdn.net
DOMAIN,anthropic.com.cdn.cloudflare.net
# 认证 / 内容
DOMAIN,anthropic.auth0.com
DOMAIN,anthropic-com.ghost.io
# 监控和遥测
DOMAIN-SUFFIX,sentry.io
DOMAIN-SUFFIX,statsigapi.net
DOMAIN,browser-intake-us5-datadoghq.com
DOMAIN-KEYWORD,datadog
DOMAIN-KEYWORD,sift
# 客服 / 第三方 widget
DOMAIN-SUFFIX,intercom.io
DOMAIN-SUFFIX,intercomcdn.com
DOMAIN,cdn.usefathom.com
# IP 段 / ASN 兜底
IP-CIDR,160.79.104.0/21,no-resolve
IP-CIDR6,2607:6bc0::/32,no-resolve
IP-ASN,399358,no-resolve
`;

/** Parses a template's extra rules, routed to `policy`. */
function extraRules(text: string | undefined, policy: string): EditableRule[] {
  const out: EditableRule[] = [];
  let note: string[] = [];
  for (const raw of (text ?? "").split("\n")) {
    const t = raw.trim();
    if (!t) continue;
    if (t.startsWith("#")) { note.push(t.slice(1).trim()); continue; }
    const [type, value, ...opts] = t.split(",").map((x) => x.trim());
    out.push({ type, value, policy, noResolve: opts.includes("no-resolve") || undefined, comment: note.join("\n") || undefined, _uid: uid() });
    note = [];
  }
  return out;
}

export const groupTemplates: { category: string; items: GroupTemplate[] }[] = [
  {
    category: tr("AI 服务"),
    items: [
      { name: "OpenAI", icon: QURE + "ChatGPT.png", ruleSets: ["OpenAi"] },
      // ACL4SSR's Claude.list only has anthropic.com and claude.ai.
      { name: "Claude", icon: LOBE + "claude-color.png", ruleSets: ["Claude"], rules: claudeRules },
      { name: "Gemini", icon: LOBE + "gemini-color.png", ruleSets: ["Gemini"] },
      { name: "Grok", icon: LOBE + "grok.png", rules: "DOMAIN-SUFFIX,grok.com\nDOMAIN-SUFFIX,x.ai" },
      { name: "Copilot", icon: QURE + "Microsoft.png", ruleSets: ["Copilot"] },
    ],
  },
  {
    category: tr("流媒体"),
    items: [
      { name: "Netflix", icon: QURE + "Netflix.png", ruleSets: ["Netflix"] },
      { name: "Disney+", icon: QURE + "Disney+.png", ruleSets: ["DisneyPlus"] },
      { name: "YouTube", icon: QURE + "YouTube.png", ruleSets: ["YouTube"] },
      { name: "HBO", icon: QURE + "HBO.png", ruleSets: ["HBO"] },
      { name: "Prime Video", icon: QURE + "Prime_Video.png", ruleSets: ["PrimeVideo"] },
      { name: "Hulu", icon: QURE + "Hulu.png", ruleSets: ["Hulu"] },
      { name: "TikTok", icon: QURE + "TikTok.png", ruleSets: ["TikTok"] },
      { name: "巴哈姆特", icon: QURE + "Bahamut.png", ruleSets: ["Bahamut"] },
    ],
  },
  {
    category: tr("音乐"),
    items: [
      { name: "Spotify", icon: QURE + "Spotify.png", ruleSets: ["Spotify"] },
      { name: "Apple Music", icon: QURE + "Apple_Music.png", ruleSets: ["AppleMusic"] },
      { name: "YouTube Music", icon: QURE + "YouTube_Music.png", ruleSets: ["YouTubeMusic"] },
    ],
  },
  {
    category: tr("社交通讯"),
    items: [
      { name: "Telegram", icon: QURE + "Telegram.png", ruleSets: ["Telegram"] },
      { name: "X", icon: QURE + "Twitter.png", ruleSets: ["Twitter"] },
      { name: "Discord", icon: QURE + "Discord.png", ruleSets: ["Discord"] },
    ],
  },
  {
    category: tr("常用服务"),
    items: [
      { name: "Google", icon: QURE + "Google_Search.png", ruleSets: ["Google"] },
      { name: "GitHub", icon: QURE + "GitHub.png", ruleSets: ["Github"] },
      { name: "Apple", icon: QURE + "Apple.png", ruleSets: ["Apple"], direct: true },
      { name: "Microsoft", icon: QURE + "Microsoft.png", ruleSets: ["Microsoft"], direct: true },
      { name: "Steam", icon: QURE + "Steam.png", ruleSets: ["Steam"] },
      { name: "PayPal", icon: QURE + "PayPal.png", ruleSets: ["PayPal"] },
    ],
  },
];

/** Groups that collect nodes by kind or region (自建, 香港 … 其他国家). */
export function isNodeGroup(g: EditableGroup): boolean {
  const f = g.nodes;
  return !!f && (f.nodeKind === "selfhosted" || !!f.regions?.length || !!f.excludeRegions?.length);
}

/** New groups go after the preset groups, before the first node group. */
export function insertIndex(groups: EditableGroup[]): number {
  const i = groups.findIndex(isNodeGroup);
  return i < 0 ? groups.length : i;
}

// Broad rules (catch-all lists, GEOIP, FINAL): service rules go in front of them.
// LAN / unban lists stay in front of service rules, so they are not listed.
const broadSets = new Set(["Global", "ChinaMax", "China", "Proxy", "ProxyGFWlist", "ProxyLite", "ChinaDomain", "ChinaIp", "ChinaCompanyIp"]);
const isBroad = (r: Rule) => r.type === "GEOIP" || r.type === "FINAL" || (r.type === "RULE-SET" && broadSets.has(r.value));

/**
 * Adds template groups to the profile: each group offers the country groups (plus DIRECT
 * first for services that usually work without a proxy); its rule sets are added if missing, and existing rules for them are
 * pointed at the new group (new rules go in front of the broad rules).
 * Returns the profile and the new groups' ids.
 */
export function addTemplates(o: EditableProfile, picked: GroupTemplate[]): [EditableProfile, string[]] {
  let groups = [...o.groups];
  let rules: EditableRule[] = [...o.rules];
  const ruleSets = [...o.ruleSets];
  const ids: string[] = [];
  // Country groups (香港 … 其他国家): node groups other than 自建.
  const countries = groups.filter((g) => isNodeGroup(g) && g.nodes?.nodeKind !== "selfhosted").map((g) => g.name);
  let at = insertIndex(groups);
  for (const t of picked) {
    if (groups.some((g) => g.name === t.name)) continue;
    const g: EditableGroup = { name: t.name, type: "select", icon: t.icon, members: t.direct ? ["DIRECT", ...countries] : countries, _uid: uid() };
    groups = [...groups.slice(0, at), g, ...groups.slice(at)];
    at++;
    ids.push(g._uid!);

    const fresh: EditableRule[] = [];
    for (const name of t.ruleSets ?? []) {
      if (!ruleSets.some((rs) => rs.name === name)) ruleSets.push(acl4ssrFiles[name] ? acl4ssr(name) : bm7(name));
      const existing = rules.filter((r) => r.type === "RULE-SET" && r.value === name);
      if (existing.length) rules = rules.map((r) => (existing.includes(r) ? { ...r, policy: t.name } : r));
      else fresh.push({ type: "RULE-SET", value: name, policy: t.name, _uid: uid() });
    }
    let pendingNote: string | undefined;
    for (const r of extraRules(t.rules, t.name)) {
      if (rules.some((x) => x.type === r.type && x.value === r.value)) {
        pendingNote ??= r.comment; // keep a section heading when its first rule already exists
        continue;
      }
      fresh.push(r.comment || !pendingNote ? r : { ...r, comment: pendingNote });
      pendingNote = undefined;
    }
    const pos = rules.findIndex(isBroad);
    rules = pos < 0 ? [...rules, ...fresh] : [...rules.slice(0, pos), ...fresh, ...rules.slice(pos)];
  }
  return [{ ...o, groups, rules, ruleSets }, ids];
}
