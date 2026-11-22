import { useEffect, useState } from "react";
import { Plus, Trash2, Ban, RotateCcw, History, ExternalLink, Download } from "lucide-react";
import { api, formatTime, installURL, subURL, type Access, type Capabilities, type Profile, type Token } from "../api";
import { Badge, Button, Card, CopyButton, Empty, ErrorText, Field, Input, Modal, PageHeader, Select } from "../components/ui";
import { TargetPicker } from "./PreviewPanel";

const expiryOptions = [
  { label: "永不过期", days: 0 },
  { label: "1 天", days: 1 },
  { label: "7 天", days: 7 },
  { label: "30 天", days: 30 },
  { label: "90 天", days: 90 },
  { label: "365 天", days: 365 },
];

function status(t: Token): { label: string; color: "green" | "red" | "amber" } {
  if (t.revoked) return { label: "已吊销", color: "red" };
  if (t.expiresAt && t.expiresAt * 1000 < Date.now()) return { label: "已过期", color: "red" };
  if (t.maxUses && t.uses >= t.maxUses) return { label: "次数用尽", color: "amber" };
  return { label: "有效", color: "green" };
}

export default function TokensPage() {
  const [items, setItems] = useState<Token[]>([]);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [caps, setCaps] = useState<Capabilities | null>(null);
  const [creating, setCreating] = useState(false);
  const [log, setLog] = useState<{ t: Token; items: Access[] } | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const load = () => api.tokens().then(setItems).catch((e) => setErr(e.message));
  useEffect(() => {
    load();
    api.profiles().then(setProfiles);
    api.capabilities().then(setCaps);
  }, []);

  const profileName = (id: string) => profiles.find((p) => p.id === id)?.name ?? "（已删除）";
  const platformLabel = (t: Token) => caps?.clients[t.target.client]?.platforms[t.target.platform]?.label ?? `${t.target.client} ${t.target.platform}`;

  return (
    <>
      <PageHeader
        sticky
        title="订阅链接"
        desc="每个链接绑定一份配置与目标客户端版本。客户端通过链接导入后会按间隔自动更新。"
        actions={<Button variant="primary" disabled={!profiles.length} onClick={() => setCreating(true)}><Plus size={16} /> 新建链接</Button>}
      />
      <ErrorText error={err} />
      {!profiles.length && <Empty>请先创建一份配置。</Empty>}
      {profiles.length > 0 && items.length === 0 && <Empty>还没有订阅链接。</Empty>}
      <div className="grid gap-3">
        {items.map((t) => {
          const st = status(t);
          return (
            <Card key={t.token} className="space-y-3 p-4">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{t.name}</span>
                <Badge color={st.color}>{st.label}</Badge>
                <Badge color="indigo">{platformLabel(t)} {t.target.version || "最新"}</Badge>
                <Badge>{profileName(t.profileId)}</Badge>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <code className="min-w-0 flex-1 truncate rounded bg-zinc-100 dark:bg-zinc-800 px-2 py-1 text-xs">{subURL(t.token)}</code>
                <CopyButton text={subURL(t.token)} />
                <a href={installURL(t)}><Button size="sm"><ExternalLink size={14} /> 导入到 {t.target.client === "surge" ? "Surge" : "Stash"}</Button></a>
                {/* The subscription response is served as an attachment; it keeps the managed URL, so an imported file still updates. */}
                <a href={subURL(t.token)} download title="下载配置文件，在 App 中手动导入；文件里带订阅地址，导入后仍会自动更新。下载计入一次访问。">
                  <Button size="sm"><Download size={14} /> 下载</Button>
                </a>
              </div>
              <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-zinc-500">
                <span>过期 {t.expiresAt ? formatTime(t.expiresAt) : "永不"}</span>
                <span>访问 {t.uses}{t.maxUses ? ` / ${t.maxUses}` : ""} 次</span>
                <span>最近 {formatTime(t.lastUsedAt)}{t.lastIp && ` · ${t.lastIp}`}</span>
                <span className="ml-auto flex gap-1">
                  <Button size="sm" variant="ghost" onClick={async () => setLog({ t, items: await api.access(t.token) })}><History size={14} /> 日志</Button>
                  <Button size="sm" variant="ghost" onClick={async () => { await api.saveToken({ ...t, revoked: !t.revoked }); load(); }}>
                    {t.revoked ? <><RotateCcw size={14} /> 恢复</> : <><Ban size={14} /> 吊销</>}
                  </Button>
                  <Button size="sm" variant="danger" onClick={async () => { if (confirm("删除该订阅链接？已导入的客户端将无法更新。")) { await api.deleteToken(t.token); load(); } }}><Trash2 size={14} /></Button>
                </span>
              </div>
            </Card>
          );
        })}
      </div>
      {creating && <CreateToken profiles={profiles} caps={caps} onClose={() => setCreating(false)} onSaved={() => { setCreating(false); load(); }} />}
      <Modal open={!!log} onClose={() => setLog(null)} title={`访问日志 · ${log?.t.name ?? ""}`} wide>
        {log && (log.items.length === 0 ? <Empty>暂无访问</Empty> : (
          <table className="w-full text-xs">
            <thead className="text-left text-zinc-500"><tr><th className="py-1">时间</th><th>IP</th><th>状态</th><th>User-Agent</th></tr></thead>
            <tbody>{log.items.map((a, i) => (
              <tr key={i} className="border-t border-zinc-100 dark:border-zinc-800">
                <td className="py-1 pr-3 whitespace-nowrap">{formatTime(a.at)}</td>
                <td className="pr-3 font-mono">{a.ip}</td>
                <td className="pr-3"><Badge color={a.status === 200 ? "green" : "red"}>{a.status}</Badge></td>
                <td className="truncate text-zinc-500">{a.userAgent}</td>
              </tr>
            ))}</tbody>
          </table>
        ))}
      </Modal>
    </>
  );
}

function CreateToken({ profiles, caps, onClose, onSaved }: { profiles: Profile[]; caps: Capabilities | null; onClose: () => void; onSaved: () => void }) {
  const [t, setT] = useState<Partial<Token>>({ profileId: profiles[0]?.id, target: { client: "surge", platform: "ios", version: "" }, name: "" });
  const [days, setDays] = useState(0);
  const [err, setErr] = useState<string | null>(null);
  const save = async () => {
    try {
      await api.saveToken({ ...t, expiresAt: days ? Math.floor(Date.now() / 1000) + days * 86400 : 0 });
      onSaved();
    } catch (e) { setErr((e as Error).message); }
  };
  return (
    <Modal open onClose={onClose} title="新建订阅链接" footer={<><Button onClick={onClose}>取消</Button><Button variant="primary" onClick={save}>创建</Button></>}>
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-3">
          <Field label="名称"><Input value={t.name} placeholder="例如 iPhone" onChange={(e) => setT({ ...t, name: e.target.value })} /></Field>
          <Field label="配置">
            <Select value={t.profileId} onChange={(e) => setT({ ...t, profileId: e.target.value })}>
              {profiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </Select>
          </Field>
        </div>
        <TargetPicker value={t.target!} onChange={(target) => setT({ ...t, target })} caps={caps} />
        <div className="grid grid-cols-2 gap-3">
          <Field label="有效期">
            <Select value={days} onChange={(e) => setDays(+e.target.value)}>
              {expiryOptions.map((o) => <option key={o.days} value={o.days}>{o.label}</option>)}
            </Select>
          </Field>
          <Field label="最大访问次数" hint="0 表示不限">
            <Input type="number" min={0} value={t.maxUses ?? 0} onChange={(e) => setT({ ...t, maxUses: +e.target.value })} />
          </Field>
        </div>
        <ErrorText error={err} />
      </div>
    </Modal>
  );
}
