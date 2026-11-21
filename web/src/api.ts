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
  types?: string[];
  sources?: string[];
  names?: string[];
}

export type GroupType = "select" | "url-test" | "fallback" | "load-balance" | "smart";

export interface Group {
  name: string;
  type: GroupType;
  icon?: string;
  members?: string[];
  nodes?: Filter | null;
  order?: string[];
  url?: string;
  interval?: number;
  tolerance?: number;
  timeout?: number;
  persistent?: boolean;
  hidden?: boolean;
  category?: string;
}

export interface Rule { type: string; value: string; policy: string; noResolve?: boolean }

export interface RuleSet {
  name: string;
  behavior: "classical" | "domain" | "ipcidr";
  surgeUrl?: string;
  stashUrl?: string;
  stashFormat?: "yaml" | "text";
  interval?: number;
}

export interface Host { domain: string; value: string }

export interface General {
  testUrl?: string;
  internetTestUrl?: string;
  dns?: string[];
  ipv6?: boolean;
  skipProxy?: string[];
  updateInterval?: number;
  extra?: Record<string, string>;
  hosts?: Host[] | null;
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

export type Client = "surge" | "stash";
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
  uses: number;
  lastUsedAt?: number;
  lastIp?: string;
  lastUa?: string;
}

export interface Warning { kind: string; name: string; message: string }
export interface RenderResult { content: string; contentType: string; warnings: Warning[] | null }

export interface Capabilities {
  clients: Record<Client, {
    platforms: Record<string, { label: string; latest: string }>;
    features: Record<string, Record<string, string | boolean>>;
  }>;
}

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
  resolve: (profile: Profile) => req<{ nodes: Node[]; members: Record<string, string[]>; warnings: Warning[] | null }>("POST", "/resolve", profile),

  tokens: () => req<Token[]>("GET", "/tokens"),
  saveToken: (t: Partial<Token>) => t.token ? req<Token>("PUT", `/tokens/${t.token}`, t) : req<Token>("POST", "/tokens", t),
  deleteToken: (t: string) => req<void>("DELETE", `/tokens/${t}`),
  access: (t: string) => req<Access[]>("GET", `/tokens/${t}/access`),
};

export const subURL = (token: string) => `${location.origin}/sub/${token}`;

export function installURL(t: Token): string {
  const u = encodeURIComponent(subURL(t.token));
  return t.target.client === "surge" ? `surge:///install-config?url=${u}` : `stash://install-config?url=${u}`;
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
