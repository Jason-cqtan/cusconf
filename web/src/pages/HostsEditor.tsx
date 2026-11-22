import { useState } from "react";
import { ClipboardPaste, Plus, Trash2 } from "lucide-react";
import type { Host } from "../api";
import { Badge, Button, Card, Field, Input, Modal, Textarea } from "../components/ui";

const ipRe = /^(\d{1,3}(\.\d{1,3}){3}|[0-9a-f:]+:[0-9a-f:.]*)$/i;

export function hostKind(h: Host): "ip" | "server" | "alias" | "" {
  if (!h.value) return "";
  if (h.value.startsWith("server:")) return "server";
  // One IP, or several separated by commas.
  return h.value.split(",").every((p) => ipRe.test(p.trim())) ? "ip" : "alias";
}

const kindInfo = {
  ip: { label: "IP", color: "green" as const, stash: "Stash：写入 hosts" },
  server: { label: "指定 DNS", color: "indigo" as const, stash: "Stash：写入 dns.nameserver-policy（server:system 不支持，会跳过）" },
  alias: { label: "域名别名", color: "amber" as const, stash: "Stash 不支持域名别名，生成 Stash 配置时会跳过" },
};

/** Parses Surge [Host] lines: `domain = value`, ignoring comments and section headers. */
export function parseHosts(text: string): { hosts: Host[]; errors: string[] } {
  const hosts: Host[] = [];
  const errors: string[] = [];
  text.split(/\r?\n/).forEach((raw, i) => {
    const t = raw.trim();
    if (!t || /^(#|\/\/|;)/.test(t) || /^\[.*\]$/.test(t)) return;
    // Surge "domain = value[, value…]" (a trailing "# …" is a comment), or Clash "domain: ip".
    const m = t.match(/^([^=\s]+)\s*=\s*(.+?)\s*(?:\s#.*)?$/) ?? t.match(/^([^:\s]+)\s*:\s*['"]?([^\s'"#]+)/);
    if (!m) return void errors.push(`第 ${i + 1} 行无法识别：${t}`);
    hosts.push({ domain: m[1], value: m[2].replace(/\s*,\s*/g, ", ") });
  });
  return { hosts, errors };
}

export function HostsEditor({ hosts, onChange }: { hosts: Host[]; onChange: (h: Host[]) => void }) {
  const [pasting, setPasting] = useState(false);
  const set = (i: number, h: Partial<Host>) => onChange(hosts.map((x, j) => (j === i ? { ...x, ...h } : x)));

  return (
    <div className="space-y-2">
      <div className="flex items-end justify-between gap-2">
        <div>
          <div className="text-xs font-medium text-zinc-600 dark:text-zinc-400">Host（本地 DNS 映射）</div>
          <p className="text-xs text-zinc-500">
            对应 Surge 的 [Host] 段。值可以是 IP（多个用逗号分隔）、另一个域名，或 <code>server:DNS地址</code>（可为 DoH 地址）指定解析服务器；域名支持 <code>*.example.com</code>。
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button size="sm" onClick={() => setPasting(true)}><ClipboardPaste size={14} /> 批量粘贴</Button>
          <Button size="sm" onClick={() => onChange([...hosts, { domain: "", value: "" }])}><Plus size={14} /> 添加</Button>
        </div>
      </div>
      {hosts.length > 0 && (
        <Card className="divide-y divide-zinc-100 dark:divide-zinc-800">
          {hosts.map((h, i) => {
            const k = hostKind(h);
            return (
              <div key={i} className="px-3 py-2">
              {h.comment && <div className="mb-1 truncate font-mono text-xs text-emerald-700 dark:text-emerald-400" title={h.comment}># <span className="font-sans">{h.comment}</span></div>}
              <div className="flex flex-wrap items-center gap-2 sm:flex-nowrap">
                <Input className="h-8 min-w-0 flex-1 font-mono text-xs" placeholder="example.com 或 *.example.com" value={h.domain} onChange={(e) => set(i, { domain: e.target.value.trim() })} />
                <span className="text-zinc-400">=</span>
                <Input className="h-8 min-w-0 flex-1 font-mono text-xs" placeholder="1.2.3.4, 5.6.7.8 / other.com / server:https://dns.alidns.com/dns-query" value={h.value} onChange={(e) => set(i, { value: e.target.value.trim() })} />
                <span className="w-20 shrink-0" title={k ? kindInfo[k].stash : undefined}>{k && <Badge color={kindInfo[k].color}>{kindInfo[k].label}</Badge>}</span>
                <Button variant="danger" size="sm" onClick={() => onChange(hosts.filter((_, j) => j !== i))} aria-label="删除"><Trash2 size={14} /></Button>
              </div>
              </div>
            );
          })}
        </Card>
      )}
      {hosts.some((h) => hostKind(h) === "alias" || h.value === "server:system") && (
        <p className="text-xs text-amber-700 dark:text-amber-400">提示：域名别名和 server:system 只在 Surge 中生效，生成 Stash 配置时会跳过并给出提示。</p>
      )}
      <PasteHosts open={pasting} onClose={() => setPasting(false)} onAdd={(add) => {
        const seen = new Set(hosts.map((h) => `${h.domain}=${h.value}`));
        onChange([...hosts.filter((h) => h.domain || h.value), ...add.filter((h) => !seen.has(`${h.domain}=${h.value}`))]);
      }} />
    </div>
  );
}

function PasteHosts({ open, onClose, onAdd }: { open: boolean; onClose: () => void; onAdd: (h: Host[]) => void }) {
  const [text, setText] = useState("");
  const { hosts, errors } = parseHosts(text);
  const close = () => { setText(""); onClose(); };
  return (
    <Modal open={open} onClose={close} title="批量粘贴 Host"
      footer={<><Button onClick={close}>取消</Button><Button variant="primary" disabled={!hosts.length} onClick={() => { onAdd(hosts); close(); }}>添加 {hosts.length || ""} 条</Button></>}>
      <Field label="每行一条" hint="Surge 格式 domain = value；也接受 Clash hosts 的 domain: ip。# 开头为注释，重复的条目会跳过。">
        <Textarea rows={8} autoFocus value={text} onChange={(e) => setText(e.target.value)}
          placeholder={"router.lan = 192.168.1.1\n*.corp.example.com = server:10.0.0.53\nalias.example.com = real.example.com"} />
      </Field>
      {errors.length > 0 && <ul className="mt-2 list-disc pl-5 text-xs text-red-600">{errors.map((e, i) => <li key={i}>{e}</li>)}</ul>}
    </Modal>
  );
}
