import { useCallback, useEffect, useState } from "react";
import { Plus, RefreshCw, Pencil, Trash2, AlertTriangle, List } from "lucide-react";
import { api, formatBytes, formatTime, type Node, type Source } from "../api";
import { Badge, Button, Card, Empty, ErrorText, Field, Input, Modal, PageHeader, Select, Textarea, Toggle } from "../components/ui";
import { NodeTable } from "./Nodes";
import { tr } from "../i18n";

const blank: Partial<Source> = { kind: "url", enabled: true, refreshInterval: 21600, name: "", url: "", content: "" };

export default function SourcesPage() {
  const [items, setItems] = useState<Source[]>([]);
  const [edit, setEdit] = useState<Partial<Source> | null>(null);
  const [viewing, setViewing] = useState<{ src: Source; nodes: Node[] } | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(() => api.sources().then(setItems).catch((e) => setErr(e.message)), []);
  useEffect(() => { load(); }, [load]);

  const refresh = async (s: Source) => {
    setBusy(s.id);
    try { await api.refreshSource(s.id); } catch (e) { setErr(`${s.name}: ${(e as Error).message}`); }
    setBusy(null);
    load();
  };

  return (
    <>
      <PageHeader
        sticky
        title={tr("订阅源")}
        desc={tr("机场订阅链接或手动添加的自建节点。支持 URI 列表 / Base64 / Clash YAML / Surge / sing-box 格式。")}
        actions={<Button variant="primary" onClick={() => setEdit({ ...blank })}><Plus size={16} /> {tr("添加")}</Button>}
      />
      <ErrorText error={err} />
      {items.length === 0 ? (
        <Empty>{tr("还没有订阅源。添加一个机场订阅或自建节点开始。")}</Empty>
      ) : (
        <div className="grid gap-3">
          {items.map((s) => (
            <Card key={s.id} className="p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{s.name}</span>
                    <Badge color={s.kind === "url" ? "indigo" : "zinc"}>{s.kind === "url" ? tr("订阅") : tr("手动")}</Badge>
                    {!s.enabled && <Badge>{tr("已停用")}</Badge>}
                    {s.format && <Badge>{s.format}</Badge>}
                    {s.selfHosted && <Badge color="green">{tr("自建")}</Badge>}
                  </div>
                  {s.kind === "url" && <div className="truncate font-mono text-xs text-zinc-500 max-w-xl">{s.url}</div>}
                  <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-500">
                    <span>{s.nodeCount} {tr("个节点")}</span>
                    <span>{tr("更新于")} {formatTime(s.fetchedAt)}</span>
                    {s.info && s.info.total > 0 && (
                      <span>{tr("流量")} {formatBytes(s.info.upload + s.info.download)} / {formatBytes(s.info.total)}</span>
                    )}
                    {s.info && s.info.expire > 0 && <span>{tr("到期")} {new Date(s.info.expire * 1000).toLocaleDateString()}</span>}
                  </div>
                  {s.lastError && <div className="text-xs text-red-600">{tr("错误：")}{s.lastError}</div>}
                  {s.warnings && s.warnings.length > 0 && (
                    <details className="text-xs text-amber-700 dark:text-amber-400">
                      <summary className="cursor-pointer"><AlertTriangle size={12} className="inline" /> {s.warnings.length} {tr("条解析警告")}</summary>
                      <ul className="mt-1 list-disc pl-5">{s.warnings.map((w, i) => <li key={i}>{w}</li>)}</ul>
                    </details>
                  )}
                </div>
                <div className="flex gap-1">
                  <Button size="sm" variant="ghost" onClick={async () => setViewing({ src: s, nodes: await api.sourceNodes(s.id) })}><List size={14} /> {tr("节点")}</Button>
                  <Button size="sm" variant="ghost" disabled={busy === s.id} onClick={() => refresh(s)}><RefreshCw size={14} className={busy === s.id ? "animate-spin" : ""} /> {tr("刷新")}</Button>
                  <Button size="sm" variant="ghost" onClick={() => setEdit(s)}><Pencil size={14} /></Button>
                  <Button size="sm" variant="danger" onClick={async () => { if (confirm(tr("删除订阅源「{0}」？", s.name))) { await api.deleteSource(s.id); load(); } }}><Trash2 size={14} /></Button>
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}
      {edit && <SourceForm initial={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); }} />}
      <Modal open={!!viewing} onClose={() => setViewing(null)} title={tr("{0} · 节点", viewing?.src.name ?? "")} wide>
        {viewing && <NodeTable nodes={viewing.nodes} className="max-h-[60vh]" />}
      </Modal>
    </>
  );
}

function SourceForm({ initial, onClose, onSaved }: { initial: Partial<Source>; onClose: () => void; onSaved: () => void }) {
  const [s, setS] = useState<Partial<Source>>(initial);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState<{ format: string; nodes: Node[]; errors: string[] } | null>(null);
  const set = (p: Partial<Source>) => setS((o) => ({ ...o, ...p }));

  const save = async () => {
    setBusy(true);
    setErr(null);
    try {
      const saved = await api.saveSource(s);
      if (saved.lastError) { setErr(tr("已保存，但拉取失败：{0}", saved.lastError)); setS(saved); setBusy(false); return; }
      onSaved();
    } catch (e) {
      setErr((e as Error).message);
    }
    setBusy(false);
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={s.id ? tr("编辑订阅源") : tr("添加订阅源")}
      wide={s.kind === "manual"}
      footer={<><Button onClick={onClose}>{tr("取消")}</Button><Button variant="primary" disabled={busy || !s.name} onClick={save}>{busy ? tr("保存中…") : tr("保存")}</Button></>}
    >
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-3">
          <Field label={tr("名称")}><Input value={s.name} onChange={(e) => set({ name: e.target.value })} placeholder={tr("机场 A")} /></Field>
          <Field label={tr("类型")}>
            <Select value={s.kind} onChange={(e) => {
              const kind = e.target.value as Source["kind"];
              // Hand-entered nodes are usually your own servers.
              set(s.id ? { kind } : { kind, selfHosted: kind === "manual" });
            }} disabled={!!s.id}>
              <option value="url">{tr("订阅链接")}</option>
              <option value="manual">{tr("手动节点")}</option>
            </Select>
          </Field>
        </div>
        {s.kind === "url" ? (
          <>
            <Field label={tr("订阅链接")}><Input value={s.url} onChange={(e) => set({ url: e.target.value })} placeholder="https://…" className="font-mono text-xs" /></Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="User-Agent" hint={tr("留空默认 clash.meta，多数机场会返回完整的 Clash.Meta 配置")}>
                <Input value={s.userAgent ?? ""} onChange={(e) => set({ userAgent: e.target.value })} placeholder="clash.meta" />
              </Field>
              <Field label={tr("自动刷新")}>
                <Select value={s.refreshInterval} onChange={(e) => set({ refreshInterval: +e.target.value })}>
                  <option value={0}>{tr("关闭")}</option>
                  <option value={3600}>{tr("每小时")}</option>
                  <option value={21600}>{tr("每 6 小时")}</option>
                  <option value={43200}>{tr("每 12 小时")}</option>
                  <option value={86400}>{tr("每天")}</option>
                </Select>
              </Field>
            </div>
          </>
        ) : (
          <>
            <Field label={tr("节点内容")} hint={tr("每行一个分享链接（ss:// vmess:// vless:// trojan:// hy2:// tuic:// wireguard:// anytls://），或粘贴 Clash proxies / Surge [Proxy] 段")}>
              <Textarea rows={10} value={s.content} onChange={(e) => { set({ content: e.target.value }); setPreview(null); }} />
            </Field>
            <div className="flex items-center gap-2">
              <Button size="sm" onClick={async () => setPreview(await api.parse(s.content ?? ""))}>{tr("解析预览")}</Button>
              {preview && <span className="text-xs text-zinc-500">{tr("格式")} {preview.format}{tr("，")}{preview.nodes.length} {tr("个节点")}{preview.errors.length > 0 && tr("，{0} 个错误", preview.errors.length)}</span>}
            </div>
            {preview && preview.errors.length > 0 && <ul className="list-disc pl-5 text-xs text-red-600">{preview.errors.map((e, i) => <li key={i}>{e}</li>)}</ul>}
            {preview && preview.nodes.length > 0 && <NodeTable nodes={preview.nodes} compact className="max-h-72" />}
          </>
        )}
        <div className="space-y-1">
          <Toggle checked={!!s.selfHosted} onChange={(v) => set({ selfHosted: v })} label={tr("自建节点")} />
          <p className="text-xs text-zinc-500">{tr("自建节点不参与按国家 / 地区分组（包括「其他」），在默认配置里单独归入「自建」策略组。")}</p>
        </div>
        <Toggle checked={!!s.enabled} onChange={(v) => set({ enabled: v })} label={tr("启用")} />
        <ErrorText error={err} />
      </div>
    </Modal>
  );
}
