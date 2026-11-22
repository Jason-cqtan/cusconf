import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { ChevronDown, ChevronRight, Plus, Trash2, ImageOff, LayoutTemplate, X } from "lucide-react";
import type { Group, GroupType } from "../api";
import { Badge, Button, Card, Field, Input, Modal, Select, Toggle, cx, useFlash } from "../components/ui";
import { SortableList } from "../components/Sortable";
import { FilterEditor } from "./FilterEditor";
import { SortableChips } from "../components/SortableChips";
import { MultiSelect } from "../components/MultiSelect";
import { uid, type EditableGroup, type EditableProfile, type EditorCtx } from "./ProfileEditor";
import { builtinIconSources, customIconSource, loadIcons, prefs, type Icon } from "./iconSources";
import { addTemplates, groupTemplates, insertIndex, type GroupTemplate } from "./groupTemplates";
import { tr } from "../i18n";

export const groupTypes: { value: GroupType; label: string; hint: string }[] = [
  { value: "select", label: tr("手动选择 select"), hint: tr("在客户端中手动切换") },
  { value: "url-test", label: tr("自动测速 url-test"), hint: tr("选择延迟最低的节点") },
  { value: "fallback", label: tr("故障转移 fallback"), hint: tr("按顺序选择第一个可用节点") },
  { value: "load-balance", label: tr("负载均衡 load-balance"), hint: tr("在可用节点间分配连接") },
  { value: "smart", label: tr("智能 smart（Surge）"), hint: tr("Surge iOS 5.11 / Mac 5.7 起支持；Stash 与低版本 Surge 自动降级为 url-test") },
];

const typeColor: Record<GroupType, "zinc" | "indigo" | "green" | "amber"> = {
  select: "zinc", "url-test": "green", fallback: "amber", "load-balance": "amber", smart: "indigo",
};

type G = EditableGroup;

export default function GroupsEditor({ profile: p, update, ctx, toolbar }: { profile: EditableProfile; update: (f: (p: EditableProfile) => EditableProfile) => void; ctx: EditorCtx; toolbar: HTMLElement | null }) {
  const [open, setOpen] = useState<string | null>(null);
  const [picking, setPicking] = useState(false);
  const [flashId, flash] = useFlash();

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
    if (!confirm(tr("删除策略组「{0}」？{1}", name, used ? tr("有 {0} 条规则使用它，将改为 DIRECT。", used) : ""))) return;
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
    while (p.groups.some((g) => g.name === tr("新策略组 {0}", n))) n++;
    const g: G = { name: tr("新策略组 {0}", n), type: "select", members: [], nodes: {}, _uid: uid() };
    const at = insertIndex(p.groups);
    setGroups([...p.groups.slice(0, at), g, ...p.groups.slice(at)]);
    setOpen(g._uid!);
    flash(g._uid!);
  };
  const addFromTemplates = (picked: GroupTemplate[]) => {
    const [next, ids] = addTemplates(p, picked);
    update(() => next);
    if (ids[0]) flash(ids[0]);
  };

  return (
    <div className="space-y-2">
      {toolbar && createPortal(
        <>
          <Button size="sm" variant="primary" onClick={add}><Plus size={14} /> {tr("添加策略组")}</Button>
          <Button size="sm" onClick={() => setPicking(true)}><LayoutTemplate size={14} /> {tr("从模板添加")}</Button>
          <span className="text-xs text-zinc-500">{tr("拖动排序。新策略组添加在预置组之后、节点分组之前；Surge 中的顺序即客户端显示顺序。")}</span>
        </>,
        toolbar,
      )}
      <TemplatePicker open={picking} onClose={() => setPicking(false)} existing={p.groups.map((g) => g.name)} onAdd={addFromTemplates} />
      <SortableList
        items={p.groups}
        ids={p.groups.map((g) => g._uid!)}
        onReorder={setGroups}
        render={(g, i, handle) => (
          <GroupCard
            g={g}
            flashing={flashId === g._uid}
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
    </div>
  );
}

function GroupCard({ g, flashing, handle, expanded, onToggle, onChange, onRemove, allGroups, resolved, totalNodes, ctx }: {
  g: G; flashing: boolean; handle: React.ReactNode; expanded: boolean; onToggle: () => void; onChange: (g: Partial<Group>) => void; onRemove: () => void;
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
      <div
        data-flash={g._uid}
        className={cx("flex items-center gap-2 rounded-t-lg px-3 py-2", !expanded && "rounded-b-lg", flashing && "flash-row", expanded && "sticky z-20 border-b border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900")}
        style={expanded ? { top: "var(--sticky-h, 0px)" } : undefined}
      >
        {handle}
        <button className="flex min-w-0 flex-1 items-center gap-2 text-left" onClick={onToggle}>
          {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          <GroupIcon url={g.icon} />
          <span className="truncate font-medium">{g.name}</span>
          <Badge color={typeColor[g.type]}>{g.type}</Badge>
          <span className="text-xs text-zinc-500 whitespace-nowrap" title={tr("直接包含 {0} 个节点，加上包含的策略组中的节点共 {1} 个（去重）", nodeCount, totalNodes)}>
            {otherCount > 0 && tr("{0} 个策略 · ", otherCount)}{totalNodes} {tr("个节点")}
            {totalNodes !== nodeCount && nodeCount > 0 && tr("（直接 {0}）", nodeCount)}
          </span>
          {g.hidden && <Badge>{tr("隐藏")}</Badge>}
        </button>
        <Button variant="danger" size="sm" onClick={onRemove} aria-label={tr("删除")}><Trash2 size={14} /></Button>
      </div>
      {expanded && (
        <div className="space-y-5 p-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={tr("名称")}><Input value={g.name} onChange={(e) => onChange({ name: e.target.value })} /></Field>
            <Field label={tr("类型")} hint={groupTypes.find((t) => t.value === g.type)?.hint}>
              <Select value={g.type} onChange={(e) => onChange({ type: e.target.value as GroupType })}>
                {groupTypes.map((t) => <option key={t.value} value={t.value}>{t.label}</option>)}
              </Select>
            </Field>
          </div>
          <IconField value={g.icon} onChange={(icon) => onChange({ icon })} />

          <Field
            label={tr("最终成员（{0}）", final.length)}
            hint={tr("生成配置时的实际顺序，可拖动调整。由下方「包含的策略」与「匹配条件的节点」组成；之后新增的节点会排在末尾。")}
          >
            {final.length === 0 ? (
              <p className="text-xs text-zinc-500">{tr("还没有成员，生成配置时会使用 DIRECT。")}</p>
            ) : (
              <div className="max-h-48 overflow-y-auto rounded-md border border-zinc-200 dark:border-zinc-800 p-2">
                <SortableChips items={final} tone={tone} onReorder={reorderFinal} />
              </div>
            )}
            {!!g.order?.length && (
              <button type="button" className="mt-1 text-xs text-indigo-600 hover:underline" onClick={() => onChange({ order: undefined })}>{tr("恢复默认顺序")}</button>
            )}
          </Field>

          <MembersField
            value={g.members ?? []}
            onChange={setMembers}
            tone={tone}
            options={["DIRECT", "REJECT", ...allGroups.filter((n) => n !== g.name)]}
            smart={g.type === "smart"}
          />

          {(g.memberFilter || members.some((m) => groupSet.has(m))) && (
            <div className="space-y-3">
              <Toggle checked={!!g.memberFilter} onChange={(v) => onChange({ memberFilter: v ? {} : null })} label={tr("再筛选包含的策略组中的节点")} />
              {g.memberFilter ? (
                <div className="rounded-md bg-zinc-50 dark:bg-zinc-950/50 p-3">
                  <FilterEditor filter={g.memberFilter} onChange={(f) => onChange({ memberFilter: f })} ctx={ctx} showNames={false} probeOpen />
                  <p className="mt-3 text-xs text-zinc-500">
                    {tr("包含的策略组（如「新加坡」）会展开为其中满足条件的节点，例如只保留可用服务含 Claude 的新加坡节点。 客户端无法在嵌套策略组内筛选，所以生成的配置里直接列出这些节点；DIRECT 和单独选择的节点不受影响。")}
                  </p>
                </div>
              ) : (
                <p className="text-xs text-zinc-500">{tr("例如包含「新加坡」，再要求「必须可用的服务」为 Claude。")}</p>
              )}
            </div>
          )}

          <div className="space-y-3">
            <Toggle checked={!!g.nodes} onChange={(v) => onChange({ nodes: v ? {} : null })} label={tr("追加匹配条件的节点")} />
            {g.nodes && (
              <div className="rounded-md bg-zinc-50 dark:bg-zinc-950/50 p-3">
                <FilterEditor filter={g.nodes} onChange={(f) => onChange({ nodes: f })} ctx={ctx} />
                <p className="mt-3 text-xs text-zinc-500">{tr("条件均为空时包含全部节点。当前匹配")} {nodeCount} {tr("个。")}</p>
              </div>
            )}
          </div>

          {auto && (
            <div className="grid gap-3 sm:grid-cols-4">
              <Field label={tr("测速间隔（秒）")}><Input type="number" value={g.interval ?? ""} placeholder="600" onChange={(e) => onChange({ interval: +e.target.value || undefined })} /></Field>
              {(g.type === "url-test" || g.type === "smart") && <Field label={tr("容差（ms）")}><Input type="number" value={g.tolerance ?? ""} placeholder="100" onChange={(e) => onChange({ tolerance: +e.target.value || undefined })} /></Field>}
              <Field label={tr("超时（秒）")}><Input type="number" value={g.timeout ?? ""} onChange={(e) => onChange({ timeout: +e.target.value || undefined })} /></Field>
              <Field label={tr("测速 URL")}><Input value={g.url ?? ""} placeholder={tr("使用通用设置")} onChange={(e) => onChange({ url: e.target.value })} /></Field>
            </div>
          )}
          <div className="flex flex-wrap gap-4">
            <Toggle checked={!!g.hidden} onChange={(v) => onChange({ hidden: v })} label={tr("在客户端隐藏")} />
            {g.type === "load-balance" && <Toggle checked={!!g.persistent} onChange={(v) => onChange({ persistent: v })} label={tr("同一域名固定节点")} />}
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
    <Field label={tr("图标")} hint={tr("Surge Mac 6.5+ 与 Stash 显示策略组图标；不支持的版本会自动省略")}>
      <div className="flex gap-2">
        <Button type="button" className="h-9 w-9 shrink-0 p-0" onClick={() => setOpen(true)}><GroupIcon url={value} /></Button>
        <Input className="font-mono text-xs" value={value ?? ""} onChange={(e) => onChange(e.target.value)} placeholder={tr("图标 URL")} />
      </div>
      {open && <IconPicker value={value} onPick={(url) => { onChange(url); setOpen(false); }} onClose={() => setOpen(false)} />}
    </Field>
  );
}

function IconPicker({ value, onPick, onClose }: { value?: string; onPick: (url: string) => void; onClose: () => void }) {
  const [custom, setCustom] = useState(prefs.custom);
  const sources = [...builtinIconSources, ...custom.map(customIconSource)];
  const [srcId, setSrcId] = useState(() => (sources.some((s) => s.id === prefs.last()) ? prefs.last() : sources[0].id));
  const src = sources.find((s) => s.id === srcId) ?? sources[0];
  const [icons, setIcons] = useState<Icon[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const [adding, setAdding] = useState("");

  useEffect(() => {
    let live = true;
    setIcons(null);
    setErr(null);
    loadIcons(src).then((v) => live && setIcons(v)).catch((e) => live && setErr((e as Error).message));
    return () => { live = false; };
  }, [src.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const choose = (id: string) => { setSrcId(id); prefs.setLast(id); };
  const addCustom = () => {
    const url = adding.trim();
    if (!/^https?:\/\//.test(url)) return;
    const next = custom.includes(url) ? custom : [...custom, url];
    setCustom(next);
    prefs.setCustom(next);
    setAdding("");
    choose(`custom:${url}`);
  };
  const removeCustom = (url: string) => {
    const next = custom.filter((u) => u !== url);
    setCustom(next);
    prefs.setCustom(next);
    if (srcId === `custom:${url}`) choose(builtinIconSources[0].id);
  };
  const k = q.trim().toLowerCase();
  const shown = (icons ?? []).filter((i) => !k || i.name.toLowerCase().includes(k));

  return (
    <Modal open onClose={onClose} title={tr("选择图标")} wide>
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-1.5">
          {sources.map((s) => (
            <span key={s.id} className="inline-flex">
              <Button size="sm" variant={s.id === src.id ? "primary" : "secondary"} onClick={() => choose(s.id)} className={s.id.startsWith("custom:") ? "rounded-r-none" : undefined}>{s.name}</Button>
              {s.id.startsWith("custom:") && (
                <Button size="sm" variant={s.id === src.id ? "primary" : "secondary"} className="rounded-l-none border-l-0 px-1.5" aria-label={tr("移除此图标源")} title={tr("移除此图标源")} onClick={() => removeCustom(s.id.slice(7))}><X size={12} /></Button>
              )}
            </span>
          ))}
        </div>
        <div className="flex flex-wrap gap-2">
          <Input className="h-8 w-full text-xs sm:w-56" placeholder={tr("搜索图标名称，如 claude、hong")} value={q} onChange={(e) => setQ(e.target.value)} autoFocus />
          <div className="flex min-w-0 flex-1 gap-2">
            <Input className="h-8 min-w-0 font-mono text-xs" placeholder={tr("添加图标订阅 JSON 地址（QuanX / Loon / Stash 格式）")} value={adding} onChange={(e) => setAdding(e.target.value)} onKeyDown={(e) => e.key === "Enter" && addCustom()} />
            <Button size="sm" className="h-8 shrink-0" disabled={!/^https?:\/\//.test(adding.trim())} onClick={addCustom}><Plus size={14} /> {tr("添加源")}</Button>
          </div>
        </div>
        {err ? <p className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300">{tr("加载失败：")}{err}</p>
          : !icons ? <p className="py-8 text-center text-sm text-zinc-500">{tr("加载中…")}</p>
          : shown.length === 0 ? <p className="py-8 text-center text-sm text-zinc-500">{tr("没有匹配的图标")}</p>
          : (
            <div className="grid max-h-[50vh] grid-cols-6 gap-1 overflow-y-auto sm:grid-cols-10">
              {shown.map((i) => (
                <button key={i.url} title={i.name} onClick={() => onPick(i.url)}
                  className={cx("flex flex-col items-center gap-1 rounded-md border p-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800", value === i.url ? "border-indigo-500" : "border-transparent")}>
                  <img src={i.url} alt="" className="h-8 w-8 object-contain" loading="lazy" />
                  <span className="w-full truncate text-[10px] text-zinc-500">{i.name}</span>
                </button>
              ))}
            </div>
          )}
        <p className="text-xs text-zinc-500">
          {icons && tr("{0} / {1} 个图标 · ", shown.length, icons.length)}{tr("来源")} {src.home ? <a className="text-indigo-600 hover:underline" href={src.home} target="_blank" rel="noreferrer">{src.home.replace(/^https?:\/\/(github\.com\/)?/, "")}</a> : src.name}{tr("。 也可以在输入框中直接填写任意图片 URL；自定义图标源只保存在当前浏览器。")}
        </p>
      </div>
    </Modal>
  );
}

function MembersField({ value, onChange, options, smart, tone }: { value: string[]; onChange: (v: string[]) => void; options: string[]; smart: boolean; tone: (m: string) => "group" | "node" | "builtin" }) {
  // The picker shows every option checked/unchecked; toggling keeps existing order and appends new ones.
  const pick = (next: string[]) => onChange([
    ...value.filter((m) => next.includes(m) || !options.includes(m)),
    ...next.filter((n) => !value.includes(n)),
  ]);
  return (
    <Field label={tr("包含的策略 / 策略组")} hint={smart ? tr("Smart 组只使用代理节点，其中的策略组和 DIRECT/REJECT 会被 Surge 忽略") : tr("拖动调整顺序；默认排在匹配节点之前")}>
      <SortableChips items={value} tone={tone} onReorder={onChange} onRemove={(m) => onChange(value.filter((x) => x !== m))}>
        <MultiSelect
          className="w-28"
          trigger={<span className="inline-flex items-center gap-1"><Plus size={12} /> {tr("添加")}</span>}
          placeholder={tr("添加")}
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

function TemplatePicker({ open, onClose, existing, onAdd }: { open: boolean; onClose: () => void; existing: string[]; onAdd: (t: GroupTemplate[]) => void }) {
  const [sel, setSel] = useState<string[]>([]);
  const close = () => { setSel([]); onClose(); };
  const all = groupTemplates.flatMap((c) => c.items);
  return (
    <Modal open={open} onClose={close} title={tr("从模板添加策略组")} wide
      footer={<><Button onClick={close}>{tr("取消")}</Button><Button variant="primary" disabled={!sel.length} onClick={() => { onAdd(all.filter((t) => sel.includes(t.name))); close(); }}>{tr("添加")} {sel.length || ""}</Button></>}>
      <div className="space-y-4">
        {groupTemplates.map((c) => (
          <div key={c.category}>
            <div className="mb-1.5 text-xs font-medium text-zinc-600 dark:text-zinc-400">{c.category}</div>
            <div className="flex flex-wrap gap-1.5">
              {c.items.map((t) => {
                const has = existing.includes(t.name);
                const on = sel.includes(t.name);
                return (
                  <button key={t.name} type="button" disabled={has} title={has ? tr("已存在同名策略组") : undefined}
                    onClick={() => setSel((cur) => (on ? cur.filter((x) => x !== t.name) : [...cur, t.name]))}
                    className={cx("inline-flex items-center gap-1.5 rounded-full border py-0.5 pr-2.5 pl-1 text-xs disabled:opacity-40",
                      on ? "border-indigo-500 bg-indigo-50 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300" : "border-zinc-300 hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800")}>
                    <img src={t.icon} alt="" className="h-4 w-4 rounded" loading="lazy" />
                    {t.name}
                  </button>
                );
              })}
            </div>
          </div>
        ))}
        <p className="text-xs text-zinc-500">
          {tr("每个模板会添加一个手动选择的策略组（成员为各国家分组），同时添加对应的规则集（优先 ACL4SSR，没有时用 blackmatrix7）； 已有指向该规则集的规则会改用新策略组，没有则插入到 GEOIP / FINAL 等兜底规则之前。")}
        </p>
      </div>
    </Modal>
  );
}
