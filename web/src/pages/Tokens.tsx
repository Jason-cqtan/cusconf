import { useEffect, useState } from "react";
import { Plus, Pencil, Trash2, Ban, RotateCcw, History, ExternalLink, Download, Scissors, X } from "lucide-react";
import { api, defaultPlatform, formatTime, installURL, subURL, tokenURL, type Access, type Capabilities, type Profile, type Token } from "../api";
import { Badge, Button, Card, CopyButton, Empty, ErrorText, Field, Input, Modal, PageHeader, Select } from "../components/ui";
import { TargetPicker } from "./PreviewPanel";
import { tr } from "../i18n";

const expiryOptions = [
  { label: tr("永不过期"), days: 0 },
  { label: tr("1 天"), days: 1 },
  { label: tr("7 天"), days: 7 },
  { label: tr("30 天"), days: 30 },
  { label: tr("90 天"), days: 90 },
  { label: tr("365 天"), days: 365 },
];

function status(t: Token): { label: string; color: "green" | "red" | "amber" } {
  if (t.revoked) return { label: tr("已吊销"), color: "red" };
  if (t.expiresAt && t.expiresAt * 1000 < Date.now()) return { label: tr("已过期"), color: "red" };
  if (t.maxUses && t.uses >= t.maxUses) return { label: tr("次数用尽"), color: "amber" };
  return { label: tr("有效"), color: "green" };
}

export default function TokensPage() {
  const [items, setItems] = useState<Token[]>([]);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [caps, setCaps] = useState<Capabilities | null>(null);
  const [editing, setEditing] = useState<Token | "new" | null>(null);
  const [log, setLog] = useState<{ t: Token; items: Access[] } | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const load = () => api.tokens().then(setItems).catch((e) => setErr(e.message));
  useEffect(() => {
    load();
    api.profiles().then(setProfiles);
    api.capabilities().then(setCaps);
  }, []);

  const profileName = (id: string) => profiles.find((p) => p.id === id)?.name ?? tr("（已删除）");
  const clientLabel = (t: Token) => caps?.clients[t.target.client]?.label ?? t.target.client;
  const platformLabel = (t: Token) => {
    const ci = caps?.clients[t.target.client];
    const p = ci?.platforms[t.target.platform || defaultPlatform(ci)]?.label ?? `${t.target.client} ${t.target.platform}`;
    return p.startsWith(clientLabel(t)) ? p : `${clientLabel(t)} · ${p}`;
  };

  return (
    <>
      <PageHeader
        sticky
        title={tr("订阅链接")}
        desc={tr("每个链接绑定一份配置与目标客户端版本。客户端通过链接导入后会按间隔自动更新。")}
        actions={<Button variant="primary" disabled={!profiles.length} onClick={() => setEditing("new")}><Plus size={16} /> {tr("新建链接")}</Button>}
      />
      <ErrorText error={err} />
      {!profiles.length && <Empty>{tr("请先创建一份配置。")}</Empty>}
      {profiles.length > 0 && items.length === 0 && <Empty>{tr("还没有订阅链接。")}</Empty>}
      <div className="grid grid-cols-1 gap-3">
        {items.map((t) => {
          const st = status(t);
          return (
            <Card key={t.token} className="space-y-3 p-4">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{t.name}</span>
                <Badge color={st.color}>{st.label}</Badge>
                <Badge color="indigo">{platformLabel(t)} {t.target.version || tr("最新")}</Badge>
                <Badge>{profileName(t.profileId)}</Badge>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <code className="min-w-0 basis-full truncate rounded sm:basis-0 sm:flex-1 bg-zinc-100 dark:bg-zinc-800 px-2 py-1 text-xs" title={t.short ? tr("完整链接：{0}", subURL(t.token)) : undefined}>{tokenURL(t)}</code>
                <CopyButton text={tokenURL(t)} />
                {t.short
                  ? <Button size="sm" variant="ghost" title={tr("删除短链接（已用短链接导入的客户端将无法更新），改回完整链接")} onClick={async () => { if (confirm(tr("删除短链接？用短链接导入的客户端将无法再更新。"))) { await api.deleteShort(t.token); load(); } }}><X size={14} /> {tr("短链")}</Button>
                  : <Button size="sm" title={tr("生成一个短链接，方便手动输入；完整链接仍然可用")} onClick={async () => { await api.createShort(t.token); load(); }}><Scissors size={14} /> {tr("生成短链")}</Button>}
                {installURL(t, caps) && <a href={installURL(t, caps)!}><Button size="sm"><ExternalLink size={14} /> {tr("导入到")} {clientLabel(t)}</Button></a>}
                {/* The subscription response is served as an attachment; it keeps the managed URL, so an imported file still updates. */}
                <a href={tokenURL(t)} download title={tr("下载配置文件，在 App 中手动导入；文件里带订阅地址，导入后仍会自动更新。下载计入一次访问。")}>
                  <Button size="sm"><Download size={14} /> {tr("下载")}</Button>
                </a>
              </div>
              <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-zinc-500">
                <span>{tr("过期")} {t.expiresAt ? formatTime(t.expiresAt) : tr("永不")}</span>
                <span>{tr("访问")} {t.uses}{t.maxUses ? ` / ${t.maxUses}` : ""} {tr("次")}</span>
                <span>{tr("最近")} {formatTime(t.lastUsedAt)}{t.lastIp && ` · ${t.lastIp}`}</span>
                <span className="ml-auto flex gap-1">
                  <Button size="sm" variant="ghost" title={tr("修改有效期、次数、配置或目标客户端，链接地址不变")} onClick={() => setEditing(t)}><Pencil size={14} /> {tr("编辑")}</Button>
                  <Button size="sm" variant="ghost" onClick={async () => setLog({ t, items: await api.access(t.token) })}><History size={14} /> {tr("日志")}</Button>
                  <Button size="sm" variant="ghost" onClick={async () => { await api.saveToken({ ...t, revoked: !t.revoked }); load(); }}>
                    {t.revoked ? <><RotateCcw size={14} /> {tr("恢复")}</> : <><Ban size={14} /> {tr("吊销")}</>}
                  </Button>
                  <Button size="sm" variant="danger" onClick={async () => { if (confirm(tr("删除该订阅链接？已导入的客户端将无法更新。"))) { await api.deleteToken(t.token); load(); } }}><Trash2 size={14} /></Button>
                </span>
              </div>
            </Card>
          );
        })}
      </div>
      {editing && <TokenForm token={editing === "new" ? undefined : editing} profiles={profiles} caps={caps} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(); }} />}
      <Modal open={!!log} onClose={() => setLog(null)} title={tr("访问日志 · {0}", log?.t.name ?? "")} wide>
        {log && (log.items.length === 0 ? <Empty>{tr("暂无访问")}</Empty> : (
          <table className="w-full text-xs">
            <thead className="text-left text-zinc-500"><tr><th className="py-1">{tr("时间")}</th><th>IP</th><th>{tr("状态")}</th><th>User-Agent</th></tr></thead>
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

// keepExpiry is the expiry option that leaves an edited link's expiry as it is.
const keepExpiry = -1;

function TokenForm({ token, profiles, caps, onClose, onSaved }: { token?: Token; profiles: Profile[]; caps: Capabilities | null; onClose: () => void; onSaved: () => void }) {
  const [t, setT] = useState<Partial<Token>>(token ?? { profileId: profiles[0]?.id, target: { client: "surge", platform: "ios", version: "" }, name: "" });
  const [days, setDays] = useState(token ? keepExpiry : 0);
  const [err, setErr] = useState<string | null>(null);
  const save = async () => {
    try {
      const expiresAt = days === keepExpiry ? t.expiresAt ?? 0 : days ? Math.floor(Date.now() / 1000) + days * 86400 : 0;
      await api.saveToken({ ...t, expiresAt });
      onSaved();
    } catch (e) { setErr((e as Error).message); }
  };
  return (
    <Modal open onClose={onClose} title={token ? tr("编辑订阅链接") : tr("新建订阅链接")} footer={<><Button onClick={onClose}>{tr("取消")}</Button><Button variant="primary" onClick={save}>{token ? tr("保存") : tr("创建")}</Button></>}>
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-3">
          <Field label={tr("名称")}><Input value={t.name} placeholder={tr("例如 iPhone")} onChange={(e) => setT({ ...t, name: e.target.value })} /></Field>
          <Field label={tr("配置")}>
            <Select value={t.profileId} onChange={(e) => setT({ ...t, profileId: e.target.value })}>
              {profiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </Select>
          </Field>
        </div>
        <TargetPicker value={t.target!} onChange={(target) => setT({ ...t, target })} caps={caps} />
        <div className="grid grid-cols-2 gap-3">
          <Field label={tr("有效期")} hint={token ? tr("从现在起算") : undefined}>
            <Select value={days} onChange={(e) => setDays(+e.target.value)}>
              {token && <option value={keepExpiry}>{tr("保持不变")}（{token.expiresAt ? formatTime(token.expiresAt) : tr("永不")}）</option>}
              {expiryOptions.map((o) => <option key={o.days} value={o.days}>{o.label}</option>)}
            </Select>
          </Field>
          <Field label={tr("最大访问次数")} hint={tr("0 表示不限")}>
            <Input type="number" min={0} value={t.maxUses ?? 0} onChange={(e) => setT({ ...t, maxUses: +e.target.value })} />
          </Field>
        </div>
        <ErrorText error={err} />
      </div>
    </Modal>
  );
}
