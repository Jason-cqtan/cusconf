import { useEffect, useState } from "react";
import { RefreshCw, Trash2 } from "lucide-react";
import { api, formatBytes, formatTime, type MirrorEntry } from "../api";
import { Badge, Button, Empty, ErrorText, Modal } from "../components/ui";
import { tr } from "../i18n";

/** Lists the rule lists the mirror has cached, with manual refresh. */
export default function MirrorDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [items, setItems] = useState<MirrorEntry[] | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const load = () => api.mirror().then(setItems).catch((e) => setErr(e.message));
  useEffect(() => { if (open) load(); }, [open]);

  const refresh = async (id: string) => {
    setBusy(id);
    setErr(null);
    try { await api.refreshMirror(id); } catch (e) { setErr((e as Error).message); }
    setBusy(null);
    load();
  };

  return (
    <Modal open={open} onClose={onClose} title={tr("规则集镜像")} wide>
      <div className="space-y-3">
        <p className="text-xs text-zinc-500">
          {tr("生成配置时用到镜像的规则集会登记在这里。客户端请求时，超过 12 小时的副本会重新下载；最近 7 天被用过的列表也会在后台定时更新。原地址失败时继续提供上次的副本。")}
        </p>
        <ErrorText error={err} />
        {items && items.length === 0 && <Empty>{tr("还没有镜像的规则集。预览或导入 sing-box / Quantumult X 配置，或开启「所有客户端都通过本服务镜像加载规则集」后会出现在这里。")}</Empty>}
        {items && items.length > 0 && (
          <table className="w-full text-xs">
            <thead className="text-left text-zinc-500"><tr><th className="py-1">{tr("原地址")}</th><th>{tr("类型")}</th><th>{tr("大小")}</th><th>{tr("更新于")}</th><th>{tr("最近使用")}</th><th /></tr></thead>
            <tbody>{items.map((m) => (
              <tr key={m.id} className="border-t border-zinc-100 dark:border-zinc-800 align-top">
                <td className="w-full max-w-0 py-1.5 pr-3">
                  <div className="truncate font-mono" title={m.url}>{m.url}</div>
                  {m.error && <div className="text-red-600" title={m.error}>{tr("上次更新失败：")}{m.error}</div>}
                </td>
                <td className="pr-3"><Badge>{m.behavior}</Badge></td>
                <td className="pr-3 whitespace-nowrap">{m.size ? formatBytes(m.size) : "—"}</td>
                <td className="pr-3 whitespace-nowrap">{formatTime(m.fetchedAt)}</td>
                <td className="pr-3 whitespace-nowrap">{formatTime(m.usedAt)}</td>
                <td className="whitespace-nowrap text-right">
                  <Button size="sm" variant="ghost" disabled={busy === m.id} onClick={() => refresh(m.id)} title={tr("立即从原地址更新")}><RefreshCw size={14} className={busy === m.id ? "animate-spin" : ""} /></Button>
                  <Button size="sm" variant="danger" title={tr("删除缓存（再次使用时会重新登记并下载）")} onClick={async () => { await api.deleteMirror(m.id); load(); }}><Trash2 size={14} /></Button>
                </td>
              </tr>
            ))}</tbody>
          </table>
        )}
      </div>
    </Modal>
  );
}
