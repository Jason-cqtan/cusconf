import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { ArrowLeft, Save } from "lucide-react";
import { api, type Capabilities, type Group, type Node, type Profile, type Rule, type Source, type Warning } from "../api";
import { Button, Chips, ErrorText, Field, Input, StickyBar, Textarea, Toggle, cx } from "../components/ui";
import GroupsEditor from "./GroupsEditor";
import RulesEditor from "./RulesEditor";
import PreviewPanel from "./PreviewPanel";
import { FilterEditor } from "./FilterEditor";
import { HostsEditor } from "./HostsEditor";
import { ExtraConfigEditor } from "./ExtraConfigEditor";
import { tr } from "../i18n";

export type EditableGroup = Group & { _uid?: string };
export type EditableRule = Rule & { _uid?: string };
export type EditableProfile = Omit<Profile, "groups" | "rules"> & { groups: EditableGroup[]; rules: EditableRule[] };

export interface EditorCtx {
  sources: Source[];
  regions: { Code: string; Name: string }[];
  caps: Capabilities | null;
  nodes: Node[]; // nodes after the profile's source selection / global filter
  scopeNodes: Node[]; // nodes after the source selection only (choices for the global filter)
  members: Record<string, string[]>; // resolved group members
}

let uidSeq = 0;
export const uid = () => `u${++uidSeq}`;

const tabs = [
  { id: "groups", label: tr("策略组") },
  { id: "rules", label: tr("规则") },
  { id: "nodes", label: tr("节点范围") },
  { id: "general", label: tr("通用") },
  { id: "preview", label: tr("生成预览") },
] as const;
type Tab = (typeof tabs)[number]["id"];

export default function ProfileEditor({ id, onBack, onSaved }: { id: string; onBack: () => void; onSaved: (p: Profile) => void }) {
  const [p, setP] = useState<EditableProfile | null>(null);
  const [tab, setTab] = useState<Tab>("groups");
  const [err, setErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [ctx, setCtx] = useState<EditorCtx>({ sources: [], regions: [], caps: null, nodes: [], scopeNodes: [], members: {} });
  const [resolveWarnings, setResolveWarnings] = useState<Warning[]>([]);
  // Where the pinned header ends, so expanded group titles can pin right below it. Sticky
  // offsets count from <main>'s content edge; the bar's negative margin is that padding.
  const [barH, setBarH] = useState(0);
  const [bar, setBar] = useState<HTMLDivElement | null>(null);
  const [toolbar, setToolbar] = useState<HTMLDivElement | null>(null);
  useEffect(() => {
    if (!bar) return;
    const ro = new ResizeObserver(() => setBarH(bar.offsetHeight + parseFloat(getComputedStyle(bar).marginTop)));
    ro.observe(bar);
    return () => ro.disconnect();
  }, [bar]);

  useEffect(() => {
    const load = id === "new" ? api.template().then((t) => ({ ...t, id: "" })) : api.profile(id);
    load.then((pr) => setP({ ...pr, groups: pr.groups.map((g) => ({ ...g, _uid: uid() })), rules: pr.rules.map((r) => ({ ...r, _uid: uid() })) })).catch((e) => setErr(e.message));
    Promise.all([api.sources(), api.regions(), api.capabilities()]).then(([sources, regions, caps]) => setCtx((c) => ({ ...c, sources, regions, caps })));
  }, [id]);

  // Re-resolve group membership when the profile changes (debounced).
  const timer = useRef<number>(0);
  useEffect(() => {
    if (!p) return;
    clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      api.resolve(strip(p))
        .then((r) => { setCtx((c) => ({ ...c, nodes: r.nodes, scopeNodes: r.scopeNodes ?? r.nodes, members: r.members })); setResolveWarnings(r.warnings ?? []); setErr(null); })
        .catch((e) => setErr(e.message));
    }, 400);
  }, [p]);

  useEffect(() => {
    if (!dirty) return;
    const h = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", h);
    return () => window.removeEventListener("beforeunload", h);
  }, [dirty]);

  const update = (f: (p: EditableProfile) => EditableProfile) => { setP((o) => (o ? f(o) : o)); setDirty(true); };

  const save = async () => {
    if (!p) return;
    setSaving(true);
    setErr(null);
    try {
      const saved = await api.saveProfile(strip(p));
      setDirty(false);
      setP({
        ...saved,
        groups: saved.groups.map((g, i) => ({ ...g, _uid: p.groups[i]?._uid ?? uid() })),
        rules: saved.rules.map((r, i) => ({ ...r, _uid: p.rules[i]?._uid ?? uid() })),
      });
      if (!p.id) onSaved(saved);
    } catch (e) {
      setErr((e as Error).message);
    }
    setSaving(false);
  };

  if (!p) return <ErrorText error={err} />;

  return (
    <div className="space-y-4" style={{ "--sticky-h": `${barH}px` } as CSSProperties}>
      <StickyBar barRef={setBar} className="border-b border-zinc-200 dark:border-zinc-800">
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="ghost" size="sm" onClick={() => (!dirty || confirm(tr("有未保存的修改，确定离开？"))) && onBack()}><ArrowLeft size={16} /></Button>
        <Input className="max-w-xs font-medium" value={p.name} onChange={(e) => update((o) => ({ ...o, name: e.target.value }))} placeholder={tr("配置名称")} />
        <span className="text-xs text-zinc-500"><NodeCount count={ctx.nodes.length} /> · {p.groups.length} {tr("个策略组")}</span>
        <Button variant="primary" className="ml-auto" disabled={saving || !p.name} onClick={save}>
          <Save size={16} /> {saving ? tr("保存中…") : dirty ? tr("保存") : tr("已保存")}
        </Button>
      </div>
      {err && <div className="mt-3"><ErrorText error={err} /></div>}
      <div className="mt-3 flex gap-1 overflow-x-auto overflow-y-hidden">
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={cx("-mb-px border-b-2 px-3 py-2 text-sm whitespace-nowrap", tab === t.id ? "border-indigo-600 font-medium text-indigo-700 dark:text-indigo-300" : "border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200")}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === "groups" && <div ref={setToolbar} className="flex flex-wrap items-center gap-2 border-t border-zinc-200 py-2 dark:border-zinc-800" />}
      </StickyBar>

      {resolveWarnings.length > 0 && tab === "groups" && (
        <ul className="rounded-md bg-amber-50 dark:bg-amber-950/40 px-3 py-2 text-xs text-amber-800 dark:text-amber-300">
          {resolveWarnings.map((w, i) => <li key={i}>{w.name}{tr("：")}{w.message}</li>)}
        </ul>
      )}

      {tab === "groups" && <GroupsEditor profile={p} update={update} ctx={ctx} toolbar={toolbar} />}
      {tab === "rules" && <RulesEditor profile={p} update={update} />}
      {tab === "nodes" && <NodeScope profile={p} update={update} ctx={ctx} />}
      {tab === "general" && <GeneralEditor profile={p} update={update} />}
      {tab === "preview" && <PreviewPanel profile={strip(p)} caps={ctx.caps} />}
    </div>
  );
}

/** Drops editor-only fields before sending to the API. */
export function strip(p: EditableProfile): Profile {
  return {
    ...p,
    groups: p.groups.map(({ _uid, ...g }) => g),
    rules: p.rules.map(({ _uid, ...r }) => r),
    // Rows just added and still blank are not errors yet.
    general: { ...p.general, hosts: p.general?.hosts?.filter((h) => h.domain || h.value) },
  };
}

function NodeScope({ profile: p, update, ctx }: { profile: EditableProfile; update: (f: (p: EditableProfile) => EditableProfile) => void; ctx: EditorCtx }) {
  const sel = p.sources ?? [];
  const renames = p.rename ?? [];
  const setRename = (r: typeof renames) => update((o) => ({ ...o, rename: r }));
  return (
    <div className="grid items-start gap-6 xl:grid-cols-2">
      <Field label={tr("订阅源")} hint={tr("不选表示使用全部启用的订阅源")}>
        <Chips options={ctx.sources.map((s) => ({ value: s.id, label: `${s.name} (${s.nodeCount})` }))} value={sel} onChange={(v) => update((o) => ({ ...o, sources: v }))} />
      </Field>
      <RenameEditor renames={renames} setRename={setRename} />
      <div className="xl:col-span-2">
        <div className="mb-2 text-xs font-medium text-zinc-600 dark:text-zinc-400">{tr("全局节点过滤（先于策略组生效）")}</div>
        <FilterEditor filter={p.filter} onChange={(f) => update((o) => ({ ...o, filter: f }))} ctx={{ ...ctx, nodes: ctx.scopeNodes }} showNames={false} />
      </div>
    </div>
  );
}

function RenameEditor({ renames, setRename }: { renames: { pattern: string; replace: string }[]; setRename: (r: { pattern: string; replace: string }[]) => void }) {
  return (
    <div className="space-y-2">
      <div className="text-xs font-medium text-zinc-600 dark:text-zinc-400">{tr("节点重命名（正则替换，按顺序执行）")}</div>
      {renames.map((r, i) => (
        <div key={i} className="flex gap-2">
          <Input className="font-mono text-xs" placeholder={tr("正则，如 ^\\[.*?\\]\\s*")} value={r.pattern} onChange={(e) => setRename(renames.map((x, j) => (j === i ? { ...x, pattern: e.target.value } : x)))} />
          <Input className="font-mono text-xs" placeholder={tr("替换为")} value={r.replace} onChange={(e) => setRename(renames.map((x, j) => (j === i ? { ...x, replace: e.target.value } : x)))} />
          <Button variant="danger" size="sm" className="h-9" onClick={() => setRename(renames.filter((_, j) => j !== i))}>{tr("删除")}</Button>
        </div>
      ))}
      <Button size="sm" onClick={() => setRename([...renames, { pattern: "", replace: "" }])}>{tr("添加重命名规则")}</Button>
    </div>
  );
}

/** Node count that blinks, with the change, whenever it changes (e.g. after editing a filter). */
function NodeCount({ count }: { count: number }) {
  const prev = useRef<number | null>(null);
  const [pulse, setPulse] = useState<{ key: number; delta: number } | null>(null);
  useEffect(() => {
    // The first non-empty result is the baseline (the count starts at 0 while loading).
    if (prev.current === null) {
      if (count > 0) prev.current = count;
      return;
    }
    if (prev.current !== count) {
      const delta = count - prev.current;
      setPulse((p) => ({ key: (p?.key ?? 0) + 1, delta }));
    }
    prev.current = count;
  }, [count]);
  useEffect(() => {
    if (!pulse) return;
    const t = window.setTimeout(() => setPulse(null), 2400);
    return () => clearTimeout(t);
  }, [pulse]);
  return (
    <span key={pulse?.key} className={cx("rounded px-1 py-0.5", pulse && "count-pulse font-medium text-zinc-900 dark:text-zinc-100")}>
      {count} {tr("个节点")}
      {pulse && <span className={cx("ml-1", pulse.delta > 0 ? "text-emerald-600" : "text-red-600")}>{pulse.delta > 0 ? `+${pulse.delta}` : pulse.delta}</span>}
    </span>
  );
}

function GeneralEditor({ profile: p, update }: { profile: EditableProfile; update: (f: (p: EditableProfile) => EditableProfile) => void }) {
  const g = p.general ?? {};
  const set = (patch: Partial<typeof g>) => update((o) => ({ ...o, general: { ...o.general, ...patch } }));
  const list = (v?: string[]) => (v ?? []).join(", ");
  const parse = (s: string) => s.split(",").map((x) => x.trim()).filter(Boolean);
  const extra = useMemo(() => g.extra ?? {}, [g.extra]);
  return (
    <div className="space-y-8">
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <Field label={tr("DNS 服务器")} hint={tr("逗号分隔；Surge 可用 system")}><Input value={list(g.dns)} onChange={(e) => set({ dns: parse(e.target.value) })} placeholder="system, 223.5.5.5, 119.29.29.29" /></Field>
      <Field label={tr("代理测速 URL")}><Input value={g.testUrl ?? ""} onChange={(e) => set({ testUrl: e.target.value })} placeholder="https://cp.cloudflare.com/generate_204" /></Field>
      <Field label={tr("直连测速 URL（Surge）")}><Input value={g.internetTestUrl ?? ""} onChange={(e) => set({ internetTestUrl: e.target.value })} placeholder="http://www.apple.com/library/test/success.html" /></Field>
      <Field label={tr("配置自动更新间隔（秒）")}><Input type="number" value={g.updateInterval ?? ""} onChange={(e) => set({ updateInterval: +e.target.value || undefined })} placeholder="86400" /></Field>
      <Field label={tr("跳过代理")} className="sm:col-span-2 xl:col-span-4"><Input value={list(g.skipProxy)} onChange={(e) => set({ skipProxy: parse(e.target.value) })} placeholder={tr("留空使用默认局域网地址")} /></Field>
      <div className="sm:col-span-2"><Toggle checked={!!g.ipv6} onChange={(v) => set({ ipv6: v })} label={tr("启用 IPv6")} /></div>
      <div className="sm:col-span-2 space-y-1">
        <Toggle checked={!g.disableServerDirect} onChange={(v) => set({ disableServerDirect: !v || undefined })} label={tr("节点服务器与订阅地址直连（推荐）")} />
        <p className="text-xs text-zinc-500">{tr("在所有规则最前面，为所有启用订阅源（不只是本配置用到的）的节点服务器地址和订阅链接域名加上 DIRECT 规则，避免这些连接被规则转进其它代理（代理套代理会导致超时，订阅也可能更新失败；本机运行 cusconf 测试其它机场节点时同样需要）。机场信息节点和局域网地址不会生成规则。")}</p>
      </div>
      <div className="sm:col-span-2 xl:col-span-4">
        <Field label={tr("节点与订阅域名的解析 DNS")} hint={<>
          {tr("生成配置时，为每个节点服务器和订阅地址的域名自动写入 [Host]：")}<code>{tr("域名 = server:此地址")}</code>{tr("，并在规则最前面让这个 DNS 本身直连。 使用经代理的加密 DNS（如")} <code>encrypted-dns-follow-outbound-mode</code>{tr("）时可避免「解析节点需要先连节点」的死循环，换机场也无需手动维护。留空则不生成；Host 中已手动映射的域名不会被覆盖。")}
        </>}>
          <Input className="font-mono text-xs" value={g.serverDns ?? ""} onChange={(e) => set({ serverDns: e.target.value.trim() || undefined })} placeholder="https://dns.alidns.com/dns-query" />
        </Field>
      </div>
      <div className="sm:col-span-2 xl:col-span-4"><HostsEditor hosts={g.hosts ?? []} onChange={(hosts) => set({ hosts })} /></div>
    </div>
    <ExtraConfigEditor
      client="surge"
      label={tr("Surge [General] 追加内容")}
      hint={tr("每行 key = value。与上方生成的同名项会覆盖生成值，其余追加到 [General] 末尾。点击右侧可配置项快速插入。")}
      value={extra.surge ?? ""}
      onChange={(v) => set({ extra: { ...extra, surge: v } })}
      placeholder={tr("例如：\nshow-error-page-for-reject = true")}
    />
    <ExtraConfigEditor
      client="stash"
      label={tr("Stash 追加 YAML")}
      hint={tr("顶层 YAML，会合并进生成的配置：映射按键合并（如 dns 下的字段），其它值覆盖或新增。YAML 有误时忽略并在预览中提示。")}
      value={extra.stash ?? ""}
      onChange={(v) => set({ extra: { ...extra, stash: v } })}
      placeholder={tr("例如：\ndns:\n  fake-ip-filter:\n    - '*.lan'")}
    />
    <OtherExtra extra={extra} onChange={(e) => set({ extra: e })} />
    </div>
  );
}

const otherExtras = [
  { key: "mihomo", label: tr("Clash Meta / FlClash 追加 YAML"), hint: tr("顶层 YAML，合并规则同 Stash：映射按键合并，其它值覆盖或新增。"), placeholder: tr("例如：\nsniffer:\n  enable: true\ntun:\n  enable: true\n  stack: mixed") },
  { key: "sing-box", label: tr("sing-box 追加 JSON"), hint: tr("一个 JSON 对象，合并进生成的配置：对象按键合并，数组和其它值整体覆盖。"), placeholder: tr("例如：\n{\"log\": {\"level\": \"warn\"}, \"experimental\": {\"clash_api\": {\"external_ui\": \"ui\"}}}") },
  { key: "loon", label: tr("Loon [General] 追加内容"), hint: tr("每行 key = value；与生成的同名项会覆盖生成值，其余追加到 [General] 末尾。"), placeholder: tr("例如：\nipv6-vif = auto") },
  { key: "qx", label: tr("Quantumult X [general] 追加内容"), hint: tr("每行 key=value；与生成的同名项会覆盖生成值，其余追加到 [general] 末尾。"), placeholder: tr("例如：\nprofile_img_url=https://…") },
];

/** Raw extra config for the clients without an option list, one at a time. */
function OtherExtra({ extra, onChange }: { extra: Record<string, string>; onChange: (e: Record<string, string>) => void }) {
  const [key, setKey] = useState(otherExtras.find((o) => extra[o.key])?.key ?? otherExtras[0].key);
  const o = otherExtras.find((x) => x.key === key)!;
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-1.5">
        {otherExtras.map((x) => (
          <Button key={x.key} size="sm" variant={x.key === key ? "primary" : "secondary"} onClick={() => setKey(x.key)}>
            {x.label.split(/ 追加| extra/)[0]}{extra[x.key]?.trim() ? " •" : ""}
          </Button>
        ))}
      </div>
      <Field label={o.label} hint={o.hint}>
        <Textarea rows={8} className="font-mono text-xs" value={extra[o.key] ?? ""} placeholder={o.placeholder} onChange={(e) => onChange({ ...extra, [o.key]: e.target.value })} />
      </Field>
    </div>
  );
}
