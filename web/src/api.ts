// Typed client for the cusconf admin API.

export type NodeType =
  | "ss" | "ssr" | "vmess" | "vless" | "trojan" | "hysteria" | "hysteria2"
  | "tuic" | "wireguard" | "snell" | "anytls" | "http" | "socks5";

export interface Node {
  name: string;
  type: NodeType;
  server: string;
  port: number;
  source?: string;
  key: string;
  region: string;
  info?: boolean;
  selfHosted?: boolean;
  probe?: Probe;
  cipher?: string;
  plugin?: string;
  flow?: string;
  tls?: { enabled?: boolean; sni?: string; realityPubKey?: string };
  transport?: { type?: string };
  [k: string]: unknown;
}

export interface SubInfo { upload: number; download: number; total: number; expire: number }

export interface Source {
  id: string;
  name: string;
  kind: "url" | "manual";
  url?: string;
  userAgent?: string;
  content?: string;
  refreshInterval: number;
  enabled: boolean;
  selfHosted?: boolean;
  fetchedAt?: number;
  lastError?: string;
  nodeCount: number;
  format?: string;
  info?: SubInfo;
  warnings?: string[];
}

export interface Filter {
  include?: string;
  exclude?: string;
  regions?: string[];
  excludeRegions?: string[];
  nodeKind?: "" | "airport" | "selfhosted";
  types?: string[];
  sources?: string[];
  names?: string[];
  reachable?: boolean;
  maxLatency?: number;
  minScores?: Record<string, number> | null;
  exitRegions?: string[];
  unlocks?: string[];
  excludeFlags?: string[];
}

export interface Probe {
  latency: number;
  testedAt: number;
  exitCountry?: string;
  scores?: Record<string, number>;
  unlocked?: string[];
  partial?: string[];
  flags?: string[];
}

export type ServiceStatus = "yes" | "partial" | "no" | "unknown" | "error";
export interface ServiceResult { status: ServiceStatus; region?: string; note?: string }
export interface IPReport { source: string; ip?: string; country?: string; city?: string; isp?: string; asn?: string; score?: number; flags?: string[]; error?: string }
export interface TestResult {
  /** Received from another cusconf instead of tested here. */
  synced?: boolean;
  key: string;
  name: string;
  testedAt: number;
  latency: number;
  error?: string;
  warning?: string;
  exit?: { ip?: string; country?: string; city?: string; isp?: string; asn?: string; scores?: Record<string, number>; flags?: string[] };
  reports?: IPReport[];
  services?: Record<string, ServiceResult>;
}
export interface IPSource {
  name: string;
  enabled: boolean;
  url: string;
  headers?: Record<string, string>;
  ip?: string;
  country?: string;
  city?: string;
  isp?: string;
  asn?: string;
  score?: string;
  scoreScale?: number;
  higherIsSafer?: boolean; // fixed by the server for built-in sources
  flags?: Record<string, string>;
}
export interface TestSettings {
  concurrency: number;
  timeoutSec: number;
  latencyUrl: string;
  ipSources: IPSource[];
  services: string[];
  scheduleHours: number;
  sources?: string[] | null;
  /** Push results to another cusconf (e.g. a server that cannot reach some nodes). */
  sync?: { url?: string; key?: string; auto?: boolean };
}
export interface SyncStatus { at: number; count: number; error?: string }
export interface TestJob { running: boolean; total: number; done: number; reachable: number; startedAt?: number; finishedAt?: number; scheduled?: boolean; checks: { ip: boolean; services: boolean }; /** The job could not run, e.g. mihomo is missing. */ error?: string; sync?: SyncStatus | null }

export type GroupType = "select" | "url-test" | "fallback" | "load-balance" | "smart";

export interface Group {
  name: string;
  type: GroupType;
  icon?: string;
  members?: string[];
  nodes?: Filter | null;
  /** Expands member groups into their nodes that match this filter. */
  memberFilter?: Filter | null;
  order?: string[];
  url?: string;
  interval?: number;
  tolerance?: number;
  timeout?: number;
  persistent?: boolean;
  hidden?: boolean;
  category?: string;
}

export interface Rule { type: string; value: string; policy: string; noResolve?: boolean; /** Written as "# …" above the rule. */ comment?: string }

export interface RuleSet {
  name: string;
  behavior: "classical" | "domain" | "ipcidr";
  surgeUrl?: string;
  stashUrl?: string;
  stashFormat?: "yaml" | "text";
  /** Optional sing-box rule set (.srs / .json); without it sing-box loads a converted copy from the mirror. */
  singboxUrl?: string;
  interval?: number;
}

export interface Host { domain: string; value: string; /** Written as "# …" above the entry. */ comment?: string }

export interface General {
  testUrl?: string;
  internetTestUrl?: string;
  dns?: string[];
  ipv6?: boolean;
  skipProxy?: string[];
  updateInterval?: number;
  extra?: Record<string, string>;
  hosts?: Host[] | null;
  disableServerDirect?: boolean;
  /** Resolves node server / subscription domains with this DNS (generated [Host] entries). */
  serverDns?: string;
  /** Serve every rule set through this server's mirror (sing-box / Quantumult X use it either way). */
  mirrorRules?: boolean;
}

export interface Profile {
  id: string;
  name: string;
  sources: string[] | null;
  filter: Filter;
  rename?: { pattern: string; replace: string }[] | null;
  groups: Group[];
  rules: Rule[];
  ruleSets: RuleSet[];
  general: General;
}

export type Client = "surge" | "stash" | "clash-meta" | "flclash" | "sing-box" | "loon" | "qx";
export interface Target { client: Client; platform: string; version: string }

export interface Token {
  token: string;
  name: string;
  profileId: string;
  target: Target;
  expiresAt?: number;
  maxUses?: number;
  revoked: boolean;
  createdAt: number;
  /** Short link code: /s/{short}. */
  short?: string;
  uses: number;
  lastUsedAt?: number;
  lastIp?: string;
  lastUa?: string;
}

export interface Warning { kind: string; name: string; message: string }
export interface RenderResult { content: string; contentType: string; warnings: Warning[] | null }

export interface ClientInfo {
  label: string;
  order: number;
  ext: string;
  /** URL template that opens the app and imports the config; {url} and {name} get replaced. */
  import?: string;
  platforms: Record<string, { label: string; latest: string; order: number }>;
  features: Record<string, Record<string, string | boolean>>;
}

export interface Capabilities { clients: Record<Client, ClientInfo> }

/** Clients in display order. */
export const clientList = (caps: Capabilities | null) =>
  Object.entries(caps?.clients ?? {}).sort(([, a], [, b]) => a.order - b.order) as [Client, ClientInfo][];

/** Platforms of a client in display order. */
export const platformList = (ci?: ClientInfo) => Object.entries(ci?.platforms ?? {}).sort(([, a], [, b]) => a.order - b.order);

export const defaultPlatform = (ci?: ClientInfo) => (ci?.platforms.ios ? "ios" : platformList(ci)[0]?.[0] ?? "ios");

export interface MirrorEntry { id: string; url: string; behavior: string; size: number; fetchedAt: number; error?: string; usedAt: number }

export interface Access { at: number; ip: string; userAgent: string; status: number }

const TOKEN_KEY = "cusconf.session";

export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

let onUnauthorized: () => void = () => {};
export const setUnauthorizedHandler = (f: () => void) => { onUnauthorized = f; };

export const session = {
  get: () => { try { return localStorage.getItem(TOKEN_KEY); } catch { return null; } },
  set: (t: string) => { try { localStorage.setItem(TOKEN_KEY, t); } catch { /* ignore */ } },
  clear: () => { try { localStorage.removeItem(TOKEN_KEY); } catch { /* ignore */ } },
};

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  const t = session.get();
  if (t) headers.Authorization = `Bearer ${t}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(`/api${path}`, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  if (res.status === 401 && path !== "/login") {
    session.clear();
    onUnauthorized();
  }
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, data.error ?? res.statusText);
  return data as T;
}

export const api = {
  login: (password: string) => req<{ token: string }>("POST", "/login", { password }),
  capabilities: () => req<Capabilities>("GET", "/capabilities"),
  regions: () => req<{ Code: string; Name: string }[]>("GET", "/regions"),

  sources: () => req<Source[]>("GET", "/sources"),
  saveSource: (s: Partial<Source>) => s.id ? req<Source>("PUT", `/sources/${s.id}`, s) : req<Source>("POST", "/sources", s),
  deleteSource: (id: string) => req<void>("DELETE", `/sources/${id}`),
  refreshSource: (id: string) => req<Source>("POST", `/sources/${id}/refresh`),
  sourceNodes: (id: string) => req<Node[]>("GET", `/sources/${id}/nodes`),
  nodes: () => req<Node[]>("GET", "/nodes"),
  parse: (content: string) => req<{ format: string; nodes: Node[]; errors: string[] }>("POST", "/parse", { content }),

  profiles: () => req<Profile[]>("GET", "/profiles"),
  profile: (id: string) => req<Profile>("GET", `/profiles/${id}`),
  template: () => req<Profile>("GET", "/profiles/template"),
  saveProfile: (p: Profile) => p.id ? req<Profile>("PUT", `/profiles/${p.id}`, p) : req<Profile>("POST", "/profiles", p),
  deleteProfile: (id: string) => req<void>("DELETE", `/profiles/${id}`),
  preview: (profile: Profile, target: Target) => req<RenderResult>("POST", "/preview", { profile, target }),
  resolve: (profile: Profile) => req<{ nodes: Node[]; scopeNodes: Node[]; members: Record<string, string[]>; warnings: Warning[] | null }>("POST", "/resolve", profile),

  testSettings: () => req<TestSettings>("GET", "/tests/settings"),
  saveTestSettings: (s: TestSettings) => req<TestSettings>("PUT", "/tests/settings", s),
  testServices: () => req<{ services: { id: string; name: string }[]; defaultIpSources: IPSource[] }>("GET", "/tests/services"),
  testResults: () => req<Record<string, TestResult>>("GET", "/tests/results"),
  testJob: () => req<TestJob>("GET", "/tests/job"),
  startTest: (keys: string[], ip: boolean, services: boolean) => req<TestJob>("POST", "/tests/job", { keys, ip, services }),
  stopTest: () => req<TestJob>("DELETE", "/tests/job"),
  testNode: (key: string, ip = true, services = true) => req<TestResult>("POST", "/tests/node", { key, ip, services }),
  syncKey: (regenerate = false) => req<{ key: string }>(regenerate ? "POST" : "GET", "/sync/key"),
  syncPush: () => req<SyncStatus>("POST", "/sync/push"),

  tokens: () => req<Token[]>("GET", "/tokens"),
  saveToken: (t: Partial<Token>) => t.token ? req<Token>("PUT", `/tokens/${t.token}`, t) : req<Token>("POST", "/tokens", t),
  deleteToken: (t: string) => req<void>("DELETE", `/tokens/${t}`),
  access: (t: string) => req<Access[]>("GET", `/tokens/${t}/access`),
  createShort: (t: string) => req<Token>("POST", `/tokens/${t}/short`),
  deleteShort: (t: string) => req<Token>("DELETE", `/tokens/${t}/short`),

  mirror: () => req<MirrorEntry[]>("GET", "/mirror"),
  refreshMirror: (id: string) => req<MirrorEntry>("POST", `/mirror/${id}/refresh`),
  deleteMirror: (id: string) => req<void>("DELETE", `/mirror/${id}`),
};

export const subURL = (token: string) => `${location.origin}/sub/${token}`;
export const shortURL = (code: string) => `${location.origin}/s/${code}`;

/** The link the client should use: the short link when there is one. */
export const tokenURL = (t: Token) => (t.short ? shortURL(t.short) : subURL(t.token));

/** One-tap import link for the token's client, or null when the client has none. */
export function installURL(t: Token, caps: Capabilities | null): string | null {
  const tpl = caps?.clients[t.target.client]?.import;
  if (!tpl) return null;
  return tpl.replace("{url}", encodeURIComponent(tokenURL(t))).replace("{name}", encodeURIComponent(t.name));
}

export function formatBytes(n: number): string {
  if (!n) return "0";
  const u = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1);
  return `${(n / 1024 ** i).toFixed(i ? 1 : 0)} ${u[i]}`;
}

export function formatTime(unix?: number): string {
  if (!unix) return "—";
  return new Date(unix * 1000).toLocaleString();
}
