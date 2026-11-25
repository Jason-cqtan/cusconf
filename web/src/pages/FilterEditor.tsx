import { useState } from "react";
import type { Filter } from "../api";
import { Chips, Field, Input, Toggle } from "../components/ui";
import { useServices } from "./Testing";
import type { EditorCtx } from "./ProfileEditor";
import { regionLabel } from "./Nodes";
import { tr, trMsg } from "../i18n";

const nodeTypes = ["ss", "ssr", "vmess", "vless", "trojan", "hysteria2", "tuic", "wireguard", "snell", "anytls", "http", "socks5"];

export function FilterEditor({ filter, onChange, ctx, showNames = true, probeOpen = false }: { filter: Filter; onChange: (f: Filter) => void; ctx: EditorCtx; showNames?: boolean; probeOpen?: boolean }) {
  const f = filter ?? {};
  const set = (p: Partial<Filter>) => onChange({ ...f, ...p });
  const [q, setQ] = useState("");
  // Regions present in the current nodes (plus any already selected), most common first.
  const regionCounts = new Map<string, number>();
  for (const n of ctx.nodes) if (n.region) regionCounts.set(n.region, (regionCounts.get(n.region) ?? 0) + 1);
  for (const r of [...(f.regions ?? []), ...(f.excludeRegions ?? [])]) if (!regionCounts.has(r)) regionCounts.set(r, 0);
  const regionOptions = [...regionCounts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([code, count]) => ({ value: code, label: `${regionLabel(code)} ${count}` }));
  const names = f.names ?? [];

  return (
    <div className="grid items-start gap-3 xl:grid-cols-2">
      <Field label={tr("地区")}>
        <Chips
          options={regionOptions}
          value={f.regions ?? []}
          onChange={(v) => set({ regions: v })}
        />
      </Field>
      <Field label={tr("排除地区")} hint={tr("排除这些地区的节点；节点名识别不出地区的节点不受影响。")}>
        <Chips options={regionOptions} value={f.excludeRegions ?? []} onChange={(v) => set({ excludeRegions: v })} />
      </Field>
      <Field label={tr("节点类别")} hint={tr("按地区的条件（地区、排除地区）不会选中自建节点；要用自建节点请选「自建」。")}>
        <Chips
          options={[{ value: "airport", label: tr("机场") }, { value: "selfhosted", label: tr("自建") }]}
          value={f.nodeKind ? [f.nodeKind] : []}
          onChange={(v) => set({ nodeKind: (v.find((x) => x !== f.nodeKind) ?? "") as Filter["nodeKind"] })}
        />
      </Field>
      <Field label={tr("协议类型")}>
        <Chips options={nodeTypes.map((t) => ({ value: t, label: t }))} value={f.types ?? []} onChange={(v) => set({ types: v })} />
      </Field>
      {ctx.sources.length > 1 && (
        <Field label={tr("订阅源")}>
          <Chips options={ctx.sources.map((s) => ({ value: s.id, label: s.name }))} value={f.sources ?? []} onChange={(v) => set({ sources: v })} />
        </Field>
      )}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:col-span-2">
        <Field label={tr("名称包含（正则）")}><Input className="font-mono text-xs" value={f.include ?? ""} onChange={(e) => set({ include: e.target.value })} placeholder={tr("例如 (?i)netflix|nf")} /></Field>
        <Field label={tr("名称排除（正则）")}><Input className="font-mono text-xs" value={f.exclude ?? ""} onChange={(e) => set({ exclude: e.target.value })} placeholder={tr("例如 过期|剩余|官网|倍率")} /></Field>
      </div>
      <ProbeConditions f={f} set={set} ctx={ctx} defaultOpen={probeOpen} />
      {showNames && (
        <details className="rounded-md border border-zinc-200 dark:border-zinc-800 p-2 xl:col-span-2">
          <summary className="cursor-pointer text-xs font-medium text-zinc-600 dark:text-zinc-400">
            {tr("指定节点")} {names.length > 0 && tr("（已选 {0}）", names.length)}
            <span className="ml-1 font-normal text-zinc-500">{tr("— 只选指定节点时，其它条件不生效")}</span>
          </summary>
          <Input className="my-2" placeholder={tr("搜索节点")} value={q} onChange={(e) => setQ(e.target.value)} />
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

/** Conditions on the latest node test (latency, exit region, safety score, unlocked services). */
function ProbeConditions({ f, set, ctx, defaultOpen }: { f: Filter; set: (p: Partial<Filter>) => void; ctx: EditorCtx; defaultOpen?: boolean }) {
  const services = useServices();
  const exits = new Map<string, number>();
  for (const n of ctx.nodes) if (n.probe?.exitCountry) exits.set(n.probe.exitCountry, (exits.get(n.probe.exitCountry) ?? 0) + 1);
  for (const r of f.exitRegions ?? []) if (!exits.has(r)) exits.set(r, 0);
  const tested = ctx.nodes.filter((n) => n.probe).length;
  // Exit IP labels seen in test results (plus any already excluded here), most common first.
  const flags = new Map<string, number>();
  for (const n of ctx.nodes) for (const fl of n.probe?.flags ?? []) flags.set(fl, (flags.get(fl) ?? 0) + 1);
  for (const fl of f.excludeFlags ?? []) if (!flags.has(fl)) flags.set(fl, 0);
  // Score sources seen in test results (plus any already used here), built-ins first.
  const order = ["proxycheck.io", "IPPure", "Net.Coffee"];
  const scoreSrc = [...new Set([...ctx.nodes.flatMap((n) => Object.keys(n.probe?.scores ?? {})), ...Object.keys(f.minScores ?? {})])]
    .sort((a, b) => ((order.indexOf(a) + 1 || 99) - (order.indexOf(b) + 1 || 99)) || a.localeCompare(b));
  const setMin = (src: string, v: string) => {
    const next = { ...(f.minScores ?? {}) };
    if (v === "") delete next[src]; else next[src] = +v;
    set({ minScores: Object.keys(next).length ? next : null });
  };
  const active = !!(f.reachable || f.maxLatency || Object.keys(f.minScores ?? {}).length || f.exitRegions?.length || f.unlocks?.length || f.excludeFlags?.length);
  // Starts open when conditions are set; afterwards only the user opens or closes it.
  const [open, setOpen] = useState(active || !!defaultOpen);
  return (
    <details open={open} onToggle={(e) => setOpen(e.currentTarget.open)} className="rounded-md border border-zinc-200 dark:border-zinc-800 p-2 xl:col-span-2">
      <summary className="cursor-pointer text-xs font-medium text-zinc-600 dark:text-zinc-400">
        {tr("按测试结果筛选")}
        <span className="ml-1 font-normal text-zinc-500">{tr("— 已测试")} {tested}/{ctx.nodes.length} {tr("个节点；未测试的节点不满足这些条件")}</span>
      </summary>
      <div className="mt-3 grid items-start gap-3 xl:grid-cols-2">
        <div className="grid items-end gap-3 sm:grid-cols-2">
          <div className="pb-2"><Toggle checked={!!f.reachable} onChange={(v) => set({ reachable: v || undefined })} label={tr("仅可用节点")} /></div>
          <Field label={tr("最大延迟（ms）")}><Input type="number" min={0} value={f.maxLatency ?? ""} placeholder={tr("不限")} onChange={(e) => set({ maxLatency: +e.target.value || undefined })} /></Field>
        </div>
        <Field label={tr("最低安全分（0–100，越大越安全；按来源分别设置，留空不限）")}>
          {scoreSrc.length === 0 ? <p className="text-xs text-zinc-500">{tr("还没有安全分数据，请先在「节点」页做完整测试。")}</p> : (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 2xl:grid-cols-3">
              {scoreSrc.map((src) => (
                <label key={src} className="flex items-center gap-2 text-sm">
                  <span className="w-28 shrink-0 truncate text-xs text-zinc-600 dark:text-zinc-400">{src} ≥</span>
                  <Input type="number" min={0} max={100} value={f.minScores?.[src] ?? ""} placeholder={tr("不限")} onChange={(e) => setMin(src, e.target.value)} />
                </label>
              ))}
            </div>
          )}
        </Field>
        <Field label={tr("出口地区（按出口 IP，而非节点名）")}>
          {exits.size === 0 ? <p className="text-xs text-zinc-500">{tr("还没有出口 IP 数据，请先在「节点」页做完整测试。")}</p> : (
            <Chips
              options={[...exits.entries()].sort((a, b) => b[1] - a[1]).map(([c, n]) => ({ value: c, label: `${regionLabel(c)} ${n}` }))}
              value={f.exitRegions ?? []}
              onChange={(v) => set({ exitRegions: v })}
            />
          )}
        </Field>
        <Field label={tr("排除标签")} hint={tr("出口 IP 带有任一所选标签（如 滥用、机房、VPN）的节点会被排除。")}>
          {flags.size === 0 ? <p className="text-xs text-zinc-500">{tr("还没有标签数据，请先在「节点」页做完整测试。")}</p> : (
            <Chips
              options={[...flags.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([fl, n]) => ({ value: fl, label: `${trMsg(fl)} ${n}` }))}
              value={f.excludeFlags ?? []}
              onChange={(v) => set({ excludeFlags: v })}
            />
          )}
        </Field>
        <Field label={tr("必须可用的服务（全部满足）")}>
          <Chips options={services.map((s) => ({ value: s.id, label: s.name }))} value={f.unlocks ?? []} onChange={(v) => set({ unlocks: v })} />
        </Field>
      </div>
    </details>
  );
}
