import { useMemo, useRef, useState } from "react";
import { Plus, Search } from "lucide-react";
import { Badge, Field, Textarea, cx } from "../components/ui";

interface Option {
  key: string; // Surge: "key"; Stash: top-level "key" or nested "dns.key"
  value: string; // example value; for Stash, YAML for the value (may be multi-line)
  desc: string;
  platform?: "iOS" | "Mac" | "macOS";
  overrides?: boolean; // replaces a value cusconf generates
}

// Surge [General] keys. Availability depends on the Surge version; check the manual when unsure.
const surgeOptions: Option[] = [
  { key: "loglevel", value: "notify", desc: "日志级别：verbose / info / notify / warning", overrides: true },
  { key: "dns-server", value: "system, 223.5.5.5, 119.29.29.29", desc: "上游 DNS 服务器", overrides: true },
  { key: "encrypted-dns-server", value: "https://223.5.5.5/dns-query", desc: "加密 DNS（DoH / DoQ / DoT）。设置后普通 DNS 只用于解析加密 DNS 自身的域名" },
  { key: "encrypted-dns-follow-outbound-mode", value: "false", desc: "加密 DNS 请求是否跟随出站模式（走代理）" },
  { key: "encrypted-dns-skip-cert-verification", value: "false", desc: "加密 DNS 跳过证书校验" },
  { key: "hijack-dns", value: "8.8.8.8:53, 8.8.4.4:53", desc: "劫持发往这些地址的 DNS 查询，由 Surge 处理；* 表示全部" },
  { key: "read-etc-hosts", value: "true", desc: "读取系统 /etc/hosts" },
  { key: "use-local-host-item-for-proxy", value: "false", desc: "代理请求也使用本地 [Host] 的解析结果" },
  { key: "always-real-ip", value: "*.lan, *.local, time.*.com", desc: "这些域名返回真实 IP 而不是 Fake IP" },
  { key: "allow-dns-svcb", value: "false", desc: "允许 SVCB / HTTPS 类型的 DNS 记录" },
  { key: "ipv6", value: "false", desc: "启用 IPv6（也可在上方开关设置）", overrides: true },
  { key: "ipv6-vif", value: "auto", desc: "虚拟网卡的 IPv6：disabled / auto / always" },
  { key: "skip-proxy", value: "127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, localhost, *.local", desc: "跳过代理的地址", overrides: true },
  { key: "exclude-simple-hostnames", value: "true", desc: "不带点的主机名不走代理", overrides: true },
  { key: "tun-excluded-routes", value: "192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12", desc: "这些网段不经过 Surge 虚拟网卡" },
  { key: "tun-included-routes", value: "192.168.1.12/32", desc: "额外让这些网段经过 Surge 虚拟网卡" },
  { key: "proxy-test-url", value: "http://www.gstatic.com/generate_204", desc: "代理测速地址", overrides: true },
  { key: "internet-test-url", value: "http://www.apple.com/library/test/success.html", desc: "直连测速地址", overrides: true },
  { key: "proxy-test-udp", value: "www.apple.com@64.6.64.6", desc: "UDP 测速：通过代理向该 DNS 服务器查询域名" },
  { key: "test-timeout", value: "5", desc: "测速超时（秒）" },
  { key: "show-error-page-for-reject", value: "true", desc: "被 REJECT 的 HTTP 请求显示错误页" },
  { key: "udp-policy-not-supported-behaviour", value: "REJECT", desc: "策略不支持 UDP 时的处理：DIRECT / REJECT" },
  { key: "udp-priority", value: "false", desc: "网络繁忙时优先处理 UDP（游戏、通话）" },
  { key: "geoip-maxmind-url", value: "https://github.com/Loyalsoldier/geoip/releases/latest/download/Country.mmdb", desc: "自定义 GeoIP 数据库地址" },
  { key: "disable-geoip-db-auto-update", value: "false", desc: "关闭 GeoIP 数据库自动更新" },
  { key: "external-controller-access", value: "password@0.0.0.0:6170", desc: "允许外部控制器（如 Surge Dashboard）访问" },
  { key: "allow-wifi-access", value: "false", desc: "允许局域网设备通过本机代理上网", platform: "iOS" },
  { key: "wifi-access-http-port", value: "6152", desc: "局域网共享的 HTTP 代理端口", platform: "iOS" },
  { key: "wifi-access-socks5-port", value: "6153", desc: "局域网共享的 SOCKS5 代理端口", platform: "iOS" },
  { key: "allow-hotspot-access", value: "false", desc: "允许个人热点连接的设备使用代理", platform: "iOS" },
  { key: "wifi-assist", value: "false", desc: "Wi-Fi 不佳时用蜂窝网络辅助建立连接", platform: "iOS" },
  { key: "all-hybrid", value: "false", desc: "所有连接同时尝试 Wi-Fi 与蜂窝网络", platform: "iOS" },
  { key: "auto-suspend", value: "false", desc: "连接到指定 SSID 时自动暂停 Surge", platform: "iOS" },
  { key: "hide-vpn-icon", value: "false", desc: "隐藏状态栏 VPN 图标", platform: "iOS" },
  { key: "http-listen", value: "0.0.0.0:6152", desc: "HTTP 代理监听地址", platform: "Mac" },
  { key: "socks5-listen", value: "0.0.0.0:6153", desc: "SOCKS5 代理监听地址", platform: "Mac" },
  { key: "http-api", value: "examplekey@127.0.0.1:6171", desc: "开启 HTTP API（key@地址:端口）", platform: "Mac" },
  { key: "http-api-web-dashboard", value: "true", desc: "开启 HTTP API 的网页面板", platform: "Mac" },
  { key: "vif-mode", value: "auto", desc: "增强模式的虚拟网卡模式", platform: "Mac" },
];

// Stash top-level YAML (Clash-compatible keys). Keys under dns merge into the generated dns block.
const stashOptions: Option[] = [
  { key: "log-level", value: "info", desc: "日志级别：silent / error / warning / info / debug", overrides: true },
  { key: "allow-lan", value: "true", desc: "允许局域网设备连接本机代理端口", overrides: true },
  { key: "mixed-port", value: "7890", desc: "HTTP + SOCKS5 混合端口", overrides: true },
  { key: "http-port", value: "7891", desc: "单独的 HTTP 代理端口" },
  { key: "socks-port", value: "7892", desc: "单独的 SOCKS5 代理端口" },
  { key: "bind-address", value: "'*'", desc: "允许局域网连接时绑定的地址" },
  { key: "external-controller", value: "127.0.0.1:9090", desc: "RESTful API 监听地址" },
  { key: "secret", value: "''", desc: "RESTful API 密钥" },
  { key: "interface-name", value: "en0", desc: "出站使用的网卡", platform: "macOS" },
  { key: "dns.fake-ip-filter", value: "\n  - '*.lan'\n  - '+.local'\n  - time.*.com", desc: "这些域名不使用 Fake IP" },
  { key: "dns.fallback", value: "\n  - https://1.1.1.1/dns-query\n  - https://dns.google/dns-query", desc: "备用 DNS（配合 fallback-filter 防污染）" },
  { key: "dns.fallback-filter", value: "\n  geoip: true\n  geoip-code: CN", desc: "非中国大陆 IP 的结果改用备用 DNS" },
  { key: "dns.nameserver-policy", value: "\n  '+.corp.example.com': 10.0.0.53", desc: "指定域名使用的 DNS（也可在上方 Host 用 server: 设置）" },
  { key: "dns.default-nameserver", value: "\n  - 223.5.5.5", desc: "解析 DNS 服务器域名用的 DNS", overrides: true },
  { key: "dns.nameserver", value: "\n  - https://223.5.5.5/dns-query", desc: "主 DNS 服务器", overrides: true },
];

type Sel = { text: string; start: number; end: number };

function insertSurge(text: string, o: Option): Sel {
  const lines = text.split("\n");
  const idx = lines.findIndex((l) => !/^\s*(#|\/\/)/.test(l) && l.split("=")[0].trim() === o.key);
  if (idx >= 0) return selectLine(lines, idx, lines[idx].indexOf("=") + 1);
  const body = text.replace(/\n*$/, "");
  const prefix = body ? body + "\n" : "";
  const line = `${o.key} = ${o.value}`;
  const next = prefix + line + "\n";
  return { text: next, start: prefix.length + o.key.length + 3, end: prefix.length + line.length };
}

function insertStash(text: string, o: Option): Sel {
  const lines = text.split("\n");
  const [top, child] = o.key.includes(".") ? o.key.split(".", 2) : [o.key, ""];
  const multiline = o.value.startsWith("\n");
  if (!child) {
    const idx = lines.findIndex((l) => l.startsWith(`${top}:`));
    if (idx >= 0) return selectLine(lines, idx, top.length + 1);
    const body = text.replace(/\n*$/, "");
    const prefix = body ? body + "\n" : "";
    const block = `${top}:${multiline ? o.value : " " + o.value}`;
    return { text: prefix + block + "\n", start: prefix.length + top.length + 1, end: prefix.length + block.length };
  }
  const indented = o.value.split("\n").map((l, i) => (i === 0 ? l : "  " + l)).join("\n");
  const childBlock = `  ${child}:${multiline ? indented : " " + o.value}`;
  const parent = lines.findIndex((l) => /^\S/.test(l) && l.trim() === `${top}:`);
  if (parent >= 0) {
    // already present under the parent?
    for (let i = parent + 1; i < lines.length && (/^\s/.test(lines[i]) || !lines[i]); i++) {
      if (lines[i].trim().startsWith(`${child}:`)) return selectLine(lines, i, lines[i].indexOf(":") + 1);
    }
    const before = lines.slice(0, parent + 1).join("\n") + "\n";
    const after = lines.slice(parent + 1).join("\n");
    return { text: before + childBlock + (after ? "\n" + after : "\n"), start: before.length, end: before.length + childBlock.length };
  }
  const body = text.replace(/\n*$/, "");
  const prefix = (body ? body + "\n" : "") + `${top}:\n`;
  return { text: prefix + childBlock + "\n", start: prefix.length, end: prefix.length + childBlock.length };
}

function selectLine(lines: string[], idx: number, from: number): Sel {
  const text = lines.join("\n");
  const start = lines.slice(0, idx).join("\n").length + (idx ? 1 : 0);
  let f = Math.max(from, 0);
  while (lines[idx][f] === " ") f++; // select the value, not the space before it
  return { text, start: start + f, end: start + lines[idx].length };
}

function present(text: string, o: Option, client: "surge" | "stash"): boolean {
  if (client === "surge") return text.split("\n").some((l) => !/^\s*(#|\/\/)/.test(l) && l.split("=")[0].trim() === o.key);
  const [top, child] = o.key.includes(".") ? o.key.split(".", 2) : [o.key, ""];
  if (!child) return new RegExp(`^${top}:`, "m").test(text);
  return new RegExp(`^${top}:\\s*$[\\s\\S]*?^\\s+${child}:`, "m").test(text);
}

/** Textarea for raw extra config with a searchable list of known options that insert on click. */
export function ExtraConfigEditor({ client, label, hint, value, onChange, placeholder }: {
  client: "surge" | "stash";
  label: string;
  hint: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const [q, setQ] = useState("");
  const options = client === "surge" ? surgeOptions : stashOptions;
  const shown = useMemo(() => {
    const k = q.trim().toLowerCase();
    return k ? options.filter((o) => `${o.key} ${o.desc} ${o.platform ?? ""}`.toLowerCase().includes(k)) : options;
  }, [q, options]);

  const insert = (o: Option) => {
    const r = client === "surge" ? insertSurge(value, o) : insertStash(value, o);
    onChange(r.text);
    // After React commits the new value, focus the textarea and select the inserted value.
    setTimeout(() => {
      const ta = ref.current;
      if (!ta) return;
      ta.focus();
      ta.setSelectionRange(r.start, r.end);
    }, 0);
  };

  return (
    <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <Field label={label} hint={hint}>
        <Textarea ref={ref} rows={12} value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className="min-h-72" />
      </Field>
      <div className="flex min-h-0 flex-col rounded-md border border-zinc-200 dark:border-zinc-800">
        <div className="flex items-center gap-2 border-b border-zinc-200 dark:border-zinc-800 px-2.5 py-2">
          <Search size={14} className="text-zinc-400" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="搜索可配置项" className="w-full bg-transparent text-sm outline-none" />
        </div>
        <div className="max-h-80 overflow-y-auto">
          {shown.map((o) => {
            const has = present(value, o, client);
            return (
              <button
                type="button"
                key={o.key}
                onClick={() => insert(o)}
                title={has ? "已添加，点击定位" : "点击插入"}
                className="group flex w-full items-start gap-2 border-b border-zinc-100 dark:border-zinc-800/60 px-2.5 py-2 text-left last:border-0 hover:bg-zinc-50 dark:hover:bg-zinc-800/50"
              >
                <Plus size={14} className={cx("mt-0.5 shrink-0", has ? "text-emerald-500" : "text-zinc-400 group-hover:text-indigo-500")} />
                <span className="min-w-0 flex-1">
                  <span className="flex flex-wrap items-center gap-1">
                    <code className="text-xs font-medium">{o.key}</code>
                    {o.platform && <Badge>{o.platform}</Badge>}
                    {o.overrides && <Badge color="amber">覆盖生成值</Badge>}
                    {has && <Badge color="green">已添加</Badge>}
                  </span>
                  <span className="mt-0.5 block text-xs text-zinc-500">{o.desc}</span>
                </span>
              </button>
            );
          })}
          {shown.length === 0 && <div className="px-3 py-4 text-center text-xs text-zinc-500">没有匹配的配置项</div>}
        </div>
      </div>
    </div>
  );
}
