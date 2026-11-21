import { useState } from "react";
import type { Filter } from "../api";
import { Chips, Field, Input } from "../components/ui";
import type { EditorCtx } from "./ProfileEditor";
import { regionLabel } from "./Nodes";

const nodeTypes = ["ss", "ssr", "vmess", "vless", "trojan", "hysteria2", "tuic", "wireguard", "snell", "anytls", "http", "socks5"];

export function FilterEditor({ filter, onChange, ctx, showNames = true }: { filter: Filter; onChange: (f: Filter) => void; ctx: EditorCtx; showNames?: boolean }) {
  const f = filter ?? {};
  const set = (p: Partial<Filter>) => onChange({ ...f, ...p });
  const [q, setQ] = useState("");
  // Regions present in the current nodes (plus any already selected), most common first.
  const regionCounts = new Map<string, number>();
  for (const n of ctx.nodes) if (n.region) regionCounts.set(n.region, (regionCounts.get(n.region) ?? 0) + 1);
  for (const r of f.regions ?? []) if (!regionCounts.has(r)) regionCounts.set(r, 0);
  const regionOptions = [...regionCounts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([code, count]) => ({ value: code, label: `${regionLabel(code)} ${count}` }));
  const names = f.names ?? [];

  return (
    <div className="space-y-3">
      <Field label="地区">
        <Chips
          options={regionOptions}
          value={f.regions ?? []}
          onChange={(v) => set({ regions: v })}
        />
      </Field>
      <Field label="协议类型">
        <Chips options={nodeTypes.map((t) => ({ value: t, label: t }))} value={f.types ?? []} onChange={(v) => set({ types: v })} />
      </Field>
      {ctx.sources.length > 1 && (
        <Field label="订阅源">
          <Chips options={ctx.sources.map((s) => ({ value: s.id, label: s.name }))} value={f.sources ?? []} onChange={(v) => set({ sources: v })} />
        </Field>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="名称包含（正则）"><Input className="font-mono text-xs" value={f.include ?? ""} onChange={(e) => set({ include: e.target.value })} placeholder="例如 (?i)netflix|nf" /></Field>
        <Field label="名称排除（正则）"><Input className="font-mono text-xs" value={f.exclude ?? ""} onChange={(e) => set({ exclude: e.target.value })} placeholder="例如 过期|剩余|官网|倍率" /></Field>
      </div>
      {showNames && (
        <details className="rounded-md border border-zinc-200 dark:border-zinc-800 p-2">
          <summary className="cursor-pointer text-xs font-medium text-zinc-600 dark:text-zinc-400">
            指定节点 {names.length > 0 && `（已选 ${names.length}）`}
            <span className="ml-1 font-normal text-zinc-500">— 只选指定节点时，其它条件不生效</span>
          </summary>
          <Input className="my-2" placeholder="搜索节点" value={q} onChange={(e) => setQ(e.target.value)} />
          <div className="max-h-56 space-y-0.5 overflow-y-auto">
            {ctx.nodes.filter((n) => !q || n.name.toLowerCase().includes(q.toLowerCase())).map((n) => (
              <label key={n.name} className="flex items-center gap-2 rounded px-1 py-0.5 text-sm hover:bg-zinc-100 dark:hover:bg-zinc-800">
                <input type="checkbox" checked={names.includes(n.name)} onChange={(e) => set({ names: e.target.checked ? [...names, n.name] : names.filter((x) => x !== n.name) })} />
                <span>{n.name}</span>
                <span className="text-xs text-zinc-500">{n.type}</span>
              </label>
            ))}
          </div>
        </details>
      )}
    </div>
  );
}
