import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Check, ChevronDown, X } from "lucide-react";
import { cx } from "./ui";

export interface Option {
  value: string;
  label: string;
  /** Extra text that search also matches, e.g. a region code. */
  keywords?: string;
  count?: number;
}

/**
 * Dropdown with type-to-filter and multiple selection.
 * Keyboard: ↑/↓ to move, Enter to toggle, Esc to close.
 */
export function MultiSelect({ options, value, onChange, placeholder, className, trigger }: {
  options: Option[];
  value: string[];
  onChange: (v: string[]) => void;
  placeholder: string;
  className?: string;
  /** Compact fixed trigger (e.g. "+ 添加") instead of showing the selection summary. */
  trigger?: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLDivElement>(null);

  const shown = useMemo(() => {
    const k = q.trim().toLowerCase();
    if (!k) return options;
    return options.filter((o) => `${o.label} ${o.value} ${o.keywords ?? ""}`.toLowerCase().includes(k));
  }, [options, q]);

  useEffect(() => {
    if (!open) return;
    const h = (e: MouseEvent) => { if (!root.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("mousedown", h);
    return () => document.removeEventListener("mousedown", h);
  }, [open]);

  useEffect(() => setActive(0), [q, open]);
  useEffect(() => { if (!open) setQ(""); }, [open]); // start each opening with an empty search
  useEffect(() => {
    list.current?.querySelector<HTMLElement>(`[data-i="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active]);

  const toggle = (v: string) => onChange(value.includes(v) ? value.filter((x) => x !== v) : [...value, v]);

  const summary = value.length === 0
    ? placeholder
    : value.length <= 2
      ? value.map((v) => options.find((o) => o.value === v)?.label ?? v).join("、")
      : `已选 ${value.length} 项`;

  return (
    <div ref={root} className={cx("relative", className)}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className={cx(
          "flex w-full items-center gap-1 rounded-md border bg-white dark:bg-zinc-900 px-2.5 text-left",
          trigger ? "h-7 text-xs" : "h-9 text-sm",
          open ? "border-indigo-500 ring-2 ring-indigo-500/20" : "border-zinc-300 dark:border-zinc-700",
        )}
      >
        <span className={cx("flex-1 truncate", !trigger && value.length === 0 && "text-zinc-500")}>{trigger ?? summary}</span>
        {!trigger && value.length > 0 && (
          <span
            role="button"
            aria-label="清空"
            className="rounded p-0.5 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
            onClick={(e) => { e.stopPropagation(); onChange([]); }}
          >
            <X size={14} />
          </span>
        )}
        <ChevronDown size={14} className="text-zinc-400" />
      </button>
      {open && (
        <div className="absolute z-30 mt-1 w-full min-w-56 overflow-hidden rounded-md border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 shadow-lg">
          <input
            autoFocus
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="输入以搜索"
            className="w-full border-b border-zinc-200 dark:border-zinc-700 bg-transparent px-2.5 py-2 text-sm outline-none"
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(a + 1, shown.length - 1)); }
              else if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
              else if (e.key === "Enter") { e.preventDefault(); if (shown[active]) toggle(shown[active].value); }
              else if (e.key === "Escape") setOpen(false);
            }}
          />
          <div ref={list} className="max-h-72 overflow-y-auto py-1">
            {shown.length === 0 && <div className="px-3 py-2 text-sm text-zinc-500">无匹配项</div>}
            {shown.map((o, i) => {
              const on = value.includes(o.value);
              return (
                <div
                  key={o.value}
                  data-i={i}
                  onMouseEnter={() => setActive(i)}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => toggle(o.value)}
                  className={cx("flex cursor-pointer items-center gap-2 px-2.5 py-1.5 text-sm", i === active && "bg-zinc-100 dark:bg-zinc-800")}
                >
                  <span className={cx("flex h-4 w-4 shrink-0 items-center justify-center rounded border", on ? "border-indigo-600 bg-indigo-600 text-white" : "border-zinc-300 dark:border-zinc-600")}>
                    {on && <Check size={12} />}
                  </span>
                  <span className="flex-1 truncate">{o.label}</span>
                  {o.count !== undefined && <span className="text-xs text-zinc-500">{o.count}</span>}
                </div>
              );
            })}
          </div>
          {value.length > 0 && (
            <div className="flex justify-between border-t border-zinc-200 dark:border-zinc-700 px-2.5 py-1.5 text-xs">
              <span className="text-zinc-500">已选 {value.length} 项</span>
              <button type="button" className="text-indigo-600 hover:underline" onClick={() => onChange([])}>清空</button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
