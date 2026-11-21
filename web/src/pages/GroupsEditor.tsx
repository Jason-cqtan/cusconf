import { useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Plus, Trash2, ImageOff } from "lucide-react";
import type { Group, GroupType } from "../api";
import { Badge, Button, Card, Field, Input, Modal, Select, Toggle, cx } from "../components/ui";
import { SortableList } from "../components/Sortable";
import { FilterEditor } from "./FilterEditor";
import { SortableChips } from "../components/SortableChips";
import { MultiSelect } from "../components/MultiSelect";
import { uid, type EditableGroup, type EditableProfile, type EditorCtx } from "./ProfileEditor";

const ICON_BASE = "https://raw.githubusercontent.com/Koolson/Qure/master/IconSet/Color/";
const presetIcons = [
  "Proxy", "Auto", "Available", "Static", "Final", "Direct", "Reject", "Advertising", "Global", "China",
  "Hong_Kong", "Taiwan", "Japan", "Singapore", "United_States", "Korea", "United_Kingdom", "Germany", "Russia", "India",
  "ChatGPT", "Netflix", "YouTube", "Disney+", "Spotify", "TikTok", "Telegram", "Twitter", "Google_Search", "Apple",
  "Microsoft", "GitHub", "Steam", "Speedtest", "Bilibili", "HBO", "Prime_Video", "Hulu", "Streaming", "Game",
];

export const groupTypes: { value: GroupType; label: string; hint: string }[] = [
  { value: "select", label: "手动选择 select", hint: "在客户端中手动切换" },
  { value: "url-test", label: "自动测速 url-test", hint: "选择延迟最低的节点" },
  { value: "fallback", label: "故障转移 fallback", hint: "按顺序选择第一个可用节点" },
  { value: "load-balance", label: "负载均衡 load-balance", hint: "在可用节点间分配连接" },
  { value: "smart", label: "智能 smart（Surge）", hint: "Surge iOS 5.11 / Mac 5.7 起支持；Stash 与低版本 Surge 自动降级为 url-test" },
];

const typeColor: Record<GroupType, "zinc" | "indigo" | "green" | "amber"> = {
  select: "zinc", "url-test": "green", fallback: "amber", "load-balance": "amber", smart: "indigo",
};

type G = EditableGroup;

export default function GroupsEditor({ profile: p, update, ctx }: { profile: EditableProfile; update: (f: (p: EditableProfile) => EditableProfile) => void; ctx: EditorCtx }) {
  const [open, setOpen] = useState<string | null>(null);

  const setGroups = (groups: G[]) => update((o) => ({ ...o, groups }));
  const patch = (i: number, g: Partial<G>) => {
    update((o) => {
      const old = o.groups[i];
      let groups = o.groups.map((x, j) => (j === i ? { ...x, ...g } : x));
      let rules = o.rules;
      // Keep references consistent when a group is renamed.
      if (g.name !== undefined && g.name !== old.name) {
        const ren = (m: string) => (m === old.name ? g.name! : m);
        groups = groups.map((x) => ({ ...x, members: x.members?.map(ren), order: x.order?.map(ren) }));
        rules = rules.map((r) => (r.policy === old.name ? { ...r, policy: g.name! } : r));
      }
      return { ...o, groups, rules };
    });
  };
  const remove = (i: number) => {
    const name = p.groups[i].name;
    const used = p.rules.filter((r) => r.policy === name).length;
    if (!confirm(`删除策略组「${name}」？${used ? `有 ${used} 条规则使用它，将改为 DIRECT。` : ""}`)) return;
    update((o) => ({
      ...o,
      groups: o.groups.filter((_, j) => j !== i).map((x) => ({ ...x, members: x.members?.filter((m) => m !== name), order: x.order?.filter((m) => m !== name) })),
      rules: o.rules.map((r) => (r.policy === name ? { ...r, policy: "DIRECT" } : r)),
    }));
  };
  // Unique nodes each group can reach, directly or through nested groups.
  // Smart groups only count their own nodes: Surge ignores nested groups in them.
  const reachable = useMemo(() => {
    const nodeNames = new Set(ctx.nodes.map((n) => n.name));
    const types = new Map(p.groups.map((g) => [g.name, g.type]));
    const memo = new Map<string, Set<string>>();
    const walk = (name: string, stack: Set<string>): Set<string> => {
      const hit = memo.get(name);
      if (hit) return hit;
      const out = new Set<string>();
      if (stack.has(name)) return out; // cycle; the server breaks it on output
      stack.add(name);
      for (const m of ctx.members[name] ?? []) {
        if (nodeNames.has(m)) out.add(m);
        else if (types.has(m) && types.get(name) !== "smart") walk(m, stack).forEach((x) => out.add(x));
      }
      stack.delete(name);
      memo.set(name, out);
      return out;
    };
    return new Map(p.groups.map((g) => [g.name, walk(g.name, new Set()).size]));
  }, [ctx.members, ctx.nodes, p.groups]);

  const add = () => {
    let n = 1;
    while (p.groups.some((g) => g.name === `新策略组 ${n}`)) n++;
    const g: G = { name: `新策略组 ${n}`, type: "select", members: [], nodes: {}, _uid: uid() };
    setGroups([...p.groups, g]);
    setOpen(g._uid!);
  };

  return (
    <div className="space-y-2">
      <p className="text-xs text-zinc-500">拖动排序。第一个策略组通常作为主选择；Surge 中的顺序即客户端显示顺序。</p>
      <SortableList
        items={p.groups}
        ids={p.groups.map((g) => g._uid!)}
        onReorder={setGroups}
        render={(g, i, handle) => (
          <GroupCard
            g={g}
            handle={handle}
            expanded={open === g._uid}
            onToggle={() => setOpen(open === g._uid ? null : g._uid!)}
            onChange={(x) => patch(i, x)}
            onRemove={() => remove(i)}
            allGroups={p.groups.map((x) => x.name)}
            resolved={ctx.members[g.name] ?? []}
            totalNodes={reachable.get(g.name) ?? 0}
            ctx={ctx}
          />
        )}
      />
      <Button onClick={add}><Plus size={16} /> 添加策略组</Button>
    </div>
  );
}

function GroupCard({ g, handle, expanded, onToggle, onChange, onRemove, allGroups, resolved, totalNodes, ctx }: {
  g: G; handle: React.ReactNode; expanded: boolean; onToggle: () => void; onChange: (g: Partial<Group>) => void; onRemove: () => void;
  allGroups: string[]; resolved: string[]; totalNodes: number; ctx: EditorCtx;
}) {
  const nodeNames = new Set(ctx.nodes.map((n) => n.name));
  const nodeCount = resolved.filter((m) => nodeNames.has(m)).length;
  const otherCount = resolved.length - nodeCount;
  const auto = g.type !== "select";
  const groupSet = new Set(allGroups);
  const tone = (m: string) => (groupSet.has(m) ? "group" : m === "DIRECT" || m === "REJECT" ? "builtin" : "node") as "group" | "node" | "builtin";

  // Server-resolved members, re-sorted locally so drags show up before the next resolve.
  const final = applyOrder(resolved, g.order ?? []);
  const members = g.members ?? [];
  const reorderFinal = (next: string[]) =>
    onChange({ order: next, members: applyRelative(members, next.filter((m) => members.includes(m))) });
  const setMembers = (next: string[]) => {
    if (!g.order?.length) return onChange({ members: next });
    const removed = members.filter((m) => !next.includes(m));
    const order = g.order.filter((m) => !removed.includes(m));
    onChange({ members: next, order: applyRelative(order, next.filter((m) => order.includes(m))) });
  };

  return (
    <Card className="mb-2">
      <div className="flex items-center gap-2 px-3 py-2">
        {handle}
        <button className="flex min-w-0 flex-1 items-center gap-2 text-left" onClick={onToggle}>
          {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          <GroupIcon url={g.icon} />
          <span className="truncate font-medium">{g.name}</span>
          <Badge color={typeColor[g.type]}>{g.type}</Badge>
          <span className="text-xs text-zinc-500 whitespace-nowrap" title={`直接包含 ${nodeCount} 个节点，加上包含的策略组中的节点共 ${totalNodes} 个（去重）`}>
            {otherCount > 0 && `${otherCount} 个策略 · `}{totalNodes} 个节点
            {totalNodes !== nodeCount && nodeCount > 0 && `（直接 ${nodeCount}）`}
          </span>
          {g.hidden && <Badge>隐藏</Badge>}
        </button>
        <Button variant="danger" size="sm" onClick={onRemove} aria-label="删除"><Trash2 size={14} /></Button>
      </div>
      {expanded && (
        <div className="space-y-5 border-t border-zinc-200 dark:border-zinc-800 p-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="名称"><Input value={g.name} onChange={(e) => onChange({ name: e.target.value })} /></Field>
            <Field label="类型" hint={groupTypes.find((t) => t.value === g.type)?.hint}>
              <Select value={g.type} onChange={(e) => onChange({ type: e.target.value as GroupType })}>
                {groupTypes.map((t) => <option key={t.value} value={t.value}>{t.label}</option>)}
              </Select>
            </Field>
          </div>
          <IconField value={g.icon} onChange={(icon) => onChange({ icon })} />

          <Field
            label={`最终成员（${final.length}）`}
            hint="生成配置时的实际顺序，可拖动调整。由下方「包含的策略」与「匹配条件的节点」组成；之后新增的节点会排在末尾。"
          >
            {final.length === 0 ? (
              <p className="text-xs text-zinc-500">还没有成员，生成配置时会使用 DIRECT。</p>
            ) : (
              <div className="max-h-48 overflow-y-auto rounded-md border border-zinc-200 dark:border-zinc-800 p-2">
                <SortableChips items={final} tone={tone} onReorder={reorderFinal} />
              </div>
            )}
            {!!g.order?.length && (
              <button type="button" className="mt-1 text-xs text-indigo-600 hover:underline" onClick={() => onChange({ order: undefined })}>恢复默认顺序</button>
            )}
          </Field>

          <MembersField
            value={g.members ?? []}
            onChange={setMembers}
            tone={tone}
            options={["DIRECT", "REJECT", ...allGroups.filter((n) => n !== g.name)]}
            smart={g.type === "smart"}
          />

          <div className="space-y-3">
            <Toggle checked={!!g.nodes} onChange={(v) => onChange({ nodes: v ? {} : null })} label="追加匹配条件的节点" />
            {g.nodes && (
              <div className="rounded-md bg-zinc-50 dark:bg-zinc-950/50 p-3">
                <FilterEditor filter={g.nodes} onChange={(f) => onChange({ nodes: f })} ctx={ctx} />
                <p className="mt-3 text-xs text-zinc-500">条件均为空时包含全部节点。当前匹配 {nodeCount} 个。</p>
              </div>
            )}
          </div>

          {auto && (
            <div className="grid gap-3 sm:grid-cols-4">
              <Field label="测速间隔（秒）"><Input type="number" value={g.interval ?? ""} placeholder="600" onChange={(e) => onChange({ interval: +e.target.value || undefined })} /></Field>
              {(g.type === "url-test" || g.type === "smart") && <Field label="容差（ms）"><Input type="number" value={g.tolerance ?? ""} placeholder="100" onChange={(e) => onChange({ tolerance: +e.target.value || undefined })} /></Field>}
              <Field label="超时（秒）"><Input type="number" value={g.timeout ?? ""} onChange={(e) => onChange({ timeout: +e.target.value || undefined })} /></Field>
              <Field label="测速 URL"><Input value={g.url ?? ""} placeholder="使用通用设置" onChange={(e) => onChange({ url: e.target.value })} /></Field>
            </div>
          )}
          <div className="flex flex-wrap gap-4">
            <Toggle checked={!!g.hidden} onChange={(v) => onChange({ hidden: v })} label="在客户端隐藏" />
            {g.type === "load-balance" && <Toggle checked={!!g.persistent} onChange={(v) => onChange({ persistent: v })} label="同一域名固定节点" />}
          </div>

        </div>
      )}
    </Card>
  );
}

function GroupIcon({ url }: { url?: string }) {
  const [bad, setBad] = useState(false);
  if (!url) return <span className="h-5 w-5 rounded bg-zinc-200 dark:bg-zinc-700" />;
  if (bad) return <ImageOff size={18} className="text-zinc-400" />;
  return <img src={url} alt="" className="h-5 w-5 rounded" onError={() => setBad(true)} />;
}

function IconField({ value, onChange }: { value?: string; onChange: (v: string) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <Field label="图标" hint="Surge Mac 6.5+ 与 Stash 显示策略组图标；不支持的版本会自动省略">
      <div className="flex gap-2">
        <Button type="button" className="h-9 w-9 shrink-0 p-0" onClick={() => setOpen(true)}><GroupIcon url={value} /></Button>
        <Input className="font-mono text-xs" value={value ?? ""} onChange={(e) => onChange(e.target.value)} placeholder="图标 URL" />
      </div>
      <Modal open={open} onClose={() => setOpen(false)} title="选择图标">
        <div className="grid grid-cols-6 gap-2 sm:grid-cols-8">
          {presetIcons.map((n) => {
            const url = `${ICON_BASE}${n}.png`;
            return (
              <button key={n} title={n} className={cx("rounded-md border p-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800", value === url ? "border-indigo-500" : "border-transparent")} onClick={() => { onChange(url); setOpen(false); }}>
                <img src={url} alt={n} className="h-8 w-8" loading="lazy" />
              </button>
            );
          })}
        </div>
        <p className="mt-3 text-xs text-zinc-500">图标来自 Koolson/Qure，也可以填写任意图片 URL。</p>
      </Modal>
    </Field>
  );
}

function MembersField({ value, onChange, options, smart, tone }: { value: string[]; onChange: (v: string[]) => void; options: string[]; smart: boolean; tone: (m: string) => "group" | "node" | "builtin" }) {
  // The picker shows every option checked/unchecked; toggling keeps existing order and appends new ones.
  const pick = (next: string[]) => onChange([
    ...value.filter((m) => next.includes(m) || !options.includes(m)),
    ...next.filter((n) => !value.includes(n)),
  ]);
  return (
    <Field label="包含的策略 / 策略组" hint={smart ? "Smart 组只使用代理节点，其中的策略组和 DIRECT/REJECT 会被 Surge 忽略" : "拖动调整顺序；默认排在匹配节点之前"}>
      <SortableChips items={value} tone={tone} onReorder={onChange} onRemove={(m) => onChange(value.filter((x) => x !== m))}>
        <MultiSelect
          className="w-28"
          trigger={<span className="inline-flex items-center gap-1"><Plus size={12} /> 添加</span>}
          placeholder="添加"
          options={options.map((o) => ({ value: o, label: o }))}
          value={value.filter((m) => options.includes(m))}
          onChange={pick}
        />
      </SortableChips>
    </Field>
  );
}

/** Same rule as the server: names in `order` first (in that order), the rest keep their position. */
function applyOrder(items: string[], order: string[]): string[] {
  if (!order.length) return items;
  const rank = new Map<string, number>();
  order.forEach((n, i) => { if (!rank.has(n)) rank.set(n, i); });
  return items
    .map((m, i) => ({ m, i, r: rank.get(m) }))
    .sort((a, b) => (a.r !== undefined && b.r !== undefined ? a.r - b.r : a.r !== undefined ? -1 : b.r !== undefined ? 1 : a.i - b.i))
    .map((x) => x.m);
}

/** Rewrites the slots `subset` occupies in `list` so they follow `subset`'s order. */
function applyRelative(list: string[], subset: string[]): string[] {
  const set = new Set(subset);
  let k = 0;
  return list.map((x) => (set.has(x) ? subset[k++] : x));
}
