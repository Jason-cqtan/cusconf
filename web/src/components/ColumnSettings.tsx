import { useState } from "react";
import { ArrowDownToLine, ArrowUpToLine } from "lucide-react";
import { Button, Modal, Toggle } from "./ui";
import { SortableList } from "./Sortable";
import { tr } from "../i18n";

/** Per-table column preferences. `order` may list ids a given table doesn't show. */
export interface ColumnPrefs {
  order: string[];
  hidden: string[];
  frozen: string[];
}

export const emptyPrefs: ColumnPrefs = { order: [], hidden: [], frozen: [] };

export function loadPrefs(key: string): ColumnPrefs {
  try {
    const v = JSON.parse(localStorage.getItem(key) ?? "null");
    if (v && Array.isArray(v.order)) return { order: v.order, hidden: v.hidden ?? [], frozen: v.frozen ?? [] };
  } catch { /* ignore */ }
  return emptyPrefs;
}

export function savePrefs(key: string, p: ColumnPrefs) {
  try { localStorage.setItem(key, JSON.stringify(p)); } catch { /* ignore */ }
}

/** Sorts columns by the saved order; unknown columns keep their default position after known ones. */
export function orderColumns<T extends { id: string }>(cols: T[], order: string[]): T[] {
  const rank = (id: string) => { const i = order.indexOf(id); return i < 0 ? 1000 + cols.findIndex((c) => c.id === id) : i; };
  return [...cols].sort((a, b) => rank(a.id) - rank(b.id));
}

/**
 * Dialog to reorder, hide and freeze columns. Edits a draft; nothing changes until "确定".
 * At least one column always stays visible.
 */
export function ColumnSettings({ open, ...p }: {
  open: boolean;
  onClose: () => void;
  columns: { id: string; label: string }[]; // in current display order
  defaultOrder: string[]; // order restored by "恢复默认"
  prefs: ColumnPrefs;
  onSave: (p: ColumnPrefs) => void;
}) {
  return open ? <Dialog {...p} /> : null;
}

function Dialog({ onClose, columns, defaultOrder, prefs, onSave }: { onClose: () => void; columns: { id: string; label: string }[]; defaultOrder: string[]; prefs: ColumnPrefs; onSave: (p: ColumnPrefs) => void }) {
  const [list, setList] = useState(columns);
  const [hidden, setHidden] = useState(() => new Set(prefs.hidden));
  const [frozen, setFrozen] = useState(() => new Set(prefs.frozen));

  const visibleCount = list.filter((c) => !hidden.has(c.id)).length;
  const toggle = (set: Set<string>, setter: (s: Set<string>) => void, id: string, on: boolean) => {
    const n = new Set(set);
    if (on) n.add(id); else n.delete(id);
    setter(n);
  };
  const move = (i: number, to: "top" | "bottom") => {
    const l = [...list];
    const [c] = l.splice(i, 1);
    if (to === "top") l.unshift(c); else l.push(c);
    setList(l);
  };

  const save = () => {
    const ids = list.map((c) => c.id);
    onSave({
      // keep ids of columns this table doesn't render, so other tables sharing the prefs keep their place
      order: [...ids, ...prefs.order.filter((id) => !ids.includes(id))],
      hidden: [...hidden],
      frozen: [...frozen],
    });
    onClose();
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={tr("列设置")}
      footer={
        <>
          <Button variant="ghost" className="mr-auto" onClick={() => { setList(orderColumns(columns, defaultOrder)); setHidden(new Set()); setFrozen(new Set()); }}>{tr("恢复默认")}</Button>
          <Button onClick={onClose}>{tr("取消")}</Button>
          <Button variant="primary" onClick={save}>{tr("确定")}</Button>
        </>
      }
    >
      <div className="mb-3 flex flex-wrap gap-1">
        <Button size="sm" variant="ghost" onClick={() => setHidden(new Set())}>{tr("全部显示")}</Button>
        <Button size="sm" variant="ghost" onClick={() => setFrozen(new Set())}>{tr("取消全部固定")}</Button>
      </div>
      <p className="mb-2 text-xs text-zinc-500">{tr("拖动调整顺序。固定的列始终排在最左侧，横向滚动时保持可见。")}</p>
      <div className="overflow-hidden rounded-md border border-zinc-200 dark:border-zinc-800">
        <div className="grid grid-cols-[1.5rem_1fr_4rem_4rem_3.5rem] items-center gap-2 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950/50 px-3 py-2 text-xs font-medium text-zinc-500">
          <span />
          <span>{tr("列名")}</span>
          <span className="text-center">{tr("隐藏")}</span>
          <span className="text-center">{tr("固定")}</span>
          <span />
        </div>
        <SortableList
          items={list}
          ids={list.map((c) => c.id)}
          onReorder={setList}
          render={(c, i, handle) => {
            const isHidden = hidden.has(c.id);
            return (
              <div className="grid grid-cols-[1.5rem_1fr_4rem_4rem_3.5rem] items-center gap-2 border-b border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-3 py-2 text-sm last:border-b-0">
                {handle}
                <span className={isHidden ? "text-zinc-400 line-through" : ""}>{c.label}</span>
                <span className="flex justify-center">
                  <Toggle checked={isHidden} onChange={(v) => (!v || visibleCount > 1) && toggle(hidden, setHidden, c.id, v)} />
                </span>
                <span className="flex justify-center">
                  <Toggle checked={frozen.has(c.id)} onChange={(v) => toggle(frozen, setFrozen, c.id, v)} />
                </span>
                <span className="flex justify-end gap-0.5">
                  <button type="button" title={tr("置顶")} disabled={i === 0} className="rounded p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700 disabled:opacity-30 dark:hover:bg-zinc-800" onClick={() => move(i, "top")}><ArrowUpToLine size={14} /></button>
                  <button type="button" title={tr("置底")} disabled={i === list.length - 1} className="rounded p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700 disabled:opacity-30 dark:hover:bg-zinc-800" onClick={() => move(i, "bottom")}><ArrowDownToLine size={14} /></button>
                </span>
              </div>
            );
          }}
        />
      </div>
      {visibleCount === 1 && <p className="mt-2 text-xs text-zinc-500">{tr("至少保留一列可见。")}</p>}
    </Modal>
  );

}
