import { useState } from "react";
import { ArrowDownToLine, ArrowUpToLine, ClipboardPaste, CornerDownRight, DatabaseZap, MessageSquareText, MoveVertical, Plus, Trash2, X } from "lucide-react";
import type { ReactNode } from "react";
import type { Rule, RuleSet } from "../api";
import { Badge, Button, Card, Field, Input, Modal, Select, Toggle, cx, useFlash } from "../components/ui";
import { SortableList } from "../components/Sortable";
import { uid, type EditableProfile, type EditableRule } from "./ProfileEditor";
import { presetSources } from "./rulePresets";
import { ImportRulesDialog, protocolValues, ruleTypes } from "./ImportRules";
import { isLogical, mapRule, maxLogicalDepth, ruleExpr, usesRuleSet } from "./logicalRules";
import MirrorDialog from "./MirrorDialog";
import { tr } from "../i18n";


type Upd = (f: (p: EditableProfile) => EditableProfile) => void;

export default function RulesEditor({ profile: p, update }: { profile: EditableProfile; update: Upd }) {
  return (
    <div className="space-y-8">
      <RuleSetsSection profile={p} update={update} />
      <RulesSection profile={p} update={update} />
    </div>
  );
}

function RulesSection({ profile: p, update }: { profile: EditableProfile; update: Upd }) {
  const policies = ["DIRECT", "REJECT", ...p.groups.map((g) => g.name)];
  // Every rule carries an editor-only _uid so drag-and-drop has stable ids.
  const ids = p.rules.map((r, i) => r._uid ?? `tmp${i}`);
  const setRules = (rules: EditableRule[]) => update((o) => ({ ...o, rules }));
  const patch = (i: number, r: Partial<Rule>) => setRules(p.rules.map((x, j) => (j === i ? { ...x, ...r } : x)));
  const [importing, setImporting] = useState(false);
  const [flashId, flash] = useFlash();
  // Rules whose comment field is open although still empty.
  const [commenting, setCommenting] = useState<Set<string>>(new Set());
  // Rules ticked for a batch move (by id), the last one clicked (anchor for shift-click ranges),
  // and whether the rows offer "move here" targets.
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [anchor, setAnchor] = useState<string | null>(null);
  const [picking, setPicking] = useState(false);
  const picked = ids.filter((id) => selected.has(id));
  const clearSelection = () => { setSelected(new Set()); setPicking(false); };

  const toggle = (i: number, shift: boolean) => {
    const id = ids[i];
    const a = anchor ? ids.indexOf(anchor) : -1;
    const n = new Set(selected);
    if (shift && a >= 0) {
      const on = !selected.has(id);
      for (let j = Math.min(a, i); j <= Math.max(a, i); j++) on ? n.add(ids[j]) : n.delete(ids[j]);
    } else if (n.has(id)) n.delete(id);
    else n.add(id);
    setSelected(n);
    setAnchor(id);
    if (!n.size) setPicking(false);
  };

  /** Moves the selected rules, keeping their order, in front of the rule at `before` (rules.length = the end). */
  const moveSelected = (before: number) => {
    const sel = (i: number) => selected.has(ids[i]);
    const moving = p.rules.filter((_, i) => sel(i));
    const rest = p.rules.filter((_, i) => !sel(i));
    const at = p.rules.slice(0, before).filter((_, i) => !sel(i)).length;
    setRules([...rest.slice(0, at), ...moving, ...rest.slice(at)]);
    setPicking(false);
    if (moving[0]?._uid) flash(moving[0]._uid);
  };
  const fin = p.rules.findIndex((x) => x.type === "FINAL");
  const toBottom = () => moveSelected(fin >= 0 && !selected.has(ids[fin]) ? fin : p.rules.length);

  return (
    <section className="space-y-2">
      <SectionHeader>
        <h3 className="font-medium">{tr("规则")}</h3>
        <div className="flex shrink-0 gap-2">
          <Button size="sm" onClick={() => setImporting(true)}><ClipboardPaste size={14} /> {tr("批量导入")}</Button>
          <Button size="sm" onClick={() => {
            const r: EditableRule = { type: "DOMAIN-SUFFIX", value: "", policy: p.groups[0]?.name ?? "DIRECT", _uid: uid() };
            const fin = p.rules.findIndex((x) => x.type === "FINAL");
            setRules(fin >= 0 ? [...p.rules.slice(0, fin), r, ...p.rules.slice(fin)] : [...p.rules, r]);
            flash(r._uid!);
          }}><Plus size={14} /> {tr("添加规则")}</Button>
        </div>
      </SectionHeader>
      <p className="text-xs text-zinc-500">{tr("从上到下匹配。最后一条应为 FINAL（Stash 中输出为 MATCH）。")}</p>
      <ImportRulesDialog
        open={importing}
        onClose={() => setImporting(false)}
        policies={policies}
        ruleSets={p.ruleSets}
        existing={p.rules}
        onImport={(rules, newSets, position) => {
          const add = rules.map((r) => ({ ...r, _uid: uid() }));
          if (add[0]) flash(add[0]._uid);
          update((o) => {
          const fin = o.rules.findIndex((x) => x.type === "FINAL");
          const merged = position === "top" ? [...add, ...o.rules]
            : fin >= 0 ? [...o.rules.slice(0, fin), ...add, ...o.rules.slice(fin)] : [...o.rules, ...add];
          return { ...o, ruleSets: [...o.ruleSets, ...newSets], rules: merged };
          });
        }}
      />
      <Card className="overflow-hidden">
        <SortableList
          items={p.rules}
          ids={ids}
          onReorder={setRules}
          // Dragging a ticked rule takes every ticked rule along.
          onMove={(from, to) => {
            if (!selected.has(ids[from]) || picked.length < 2) return false;
            moveSelected(to > from ? to + 1 : to);
            return true;
          }}
          render={(r, i, handle) => (
            <div data-flash={r._uid} className={cx("border-b border-zinc-100 dark:border-zinc-800 px-3 py-2", selected.has(ids[i]) ? "bg-indigo-50/70 dark:bg-indigo-950/40" : "bg-white dark:bg-zinc-900", flashId === r._uid && "flash-row")}>
            {picking && !selected.has(ids[i]) && (
              <button type="button" onClick={() => moveSelected(i)}
                className="mb-1 flex items-center gap-1 rounded border border-dashed sm:ml-11 border-indigo-400 px-2 py-0.5 text-xs text-indigo-600 hover:bg-indigo-50 dark:text-indigo-400 dark:hover:bg-indigo-950">
                <CornerDownRight size={12} /> {tr("移到这条上方")}
              </button>
            )}
            {commenting.has(ids[i]) ? (
              <div className="mb-1 flex items-center gap-1.5 font-mono sm:pl-11 text-xs text-emerald-700 dark:text-emerald-400">
                #
                <input className="min-w-0 flex-1 border-b border-emerald-500/60 bg-transparent py-0.5 font-sans outline-none" value={r.comment ?? ""} placeholder={tr("备注，生成配置时写在这条规则上方")} autoFocus
                  onChange={(e) => patch(i, { comment: e.target.value || undefined })}
                  onKeyDown={(e) => (e.key === "Enter" || e.key === "Escape") && e.currentTarget.blur()}
                  onBlur={() => setCommenting((s) => { const n = new Set(s); n.delete(ids[i]); return n; })} />
              </div>
            ) : r.comment && (
              // A comment, not a rule: shown like a code comment above its rule; click to edit.
              <button type="button" title={tr("点击编辑备注")} onClick={() => setCommenting((s) => new Set(s).add(ids[i]))}
                className="mb-1 block max-w-full truncate text-left sm:pl-11 font-mono text-xs text-emerald-700 hover:underline dark:text-emerald-400">
                # <span className="font-sans">{r.comment}</span>
              </button>
            )}
            {/* On phones: type and actions, then the value on its own line, then the policy. */}
            <div className="flex flex-wrap items-center gap-2 sm:flex-nowrap">
              <input type="checkbox" className="shrink-0" aria-label={tr("选择")} checked={selected.has(ids[i])}
                onChange={() => {}} onClick={(e) => toggle(i, e.shiftKey)} />
              {handle}
              <Select className="h-8 min-w-0 flex-1 text-xs sm:w-48 sm:flex-none sm:shrink-0" value={r.type} onChange={(e) => patch(i, changeType(r, e.target.value))}>
                {ruleTypes.map((t) => <option key={t}>{t}</option>)}
              </Select>
              <div className="order-2 flex w-full min-w-0 sm:order-none sm:w-auto sm:flex-1">
                {r.type === "FINAL" ? (
                  <span className="flex-1 text-xs text-zinc-500">{tr("其余所有流量")}</span>
                ) : isLogical(r.type) ? (
                  <span className="min-w-0 flex-1 truncate font-mono text-xs text-zinc-500" title={ruleExpr(r)}>{logicalHint(r.type)}</span>
                ) : (
                  <RuleValue r={r} ruleSets={p.ruleSets} onChange={(v) => patch(i, v)} />
                )}
              </div>
              <Select className="order-3 h-8 min-w-0 flex-1 text-xs sm:order-none sm:w-44 sm:flex-none sm:shrink-0" value={r.policy} onChange={(e) => patch(i, { policy: e.target.value })}>
                {policies.map((x) => <option key={x}>{x}</option>)}
              </Select>
              {noResolveTypes.includes(r.type) && (
                <label className="order-3 flex items-center gap-1 text-xs whitespace-nowrap sm:order-none"><input type="checkbox" checked={!!r.noResolve} onChange={(e) => patch(i, { noResolve: e.target.checked })} />no-resolve</label>
              )}
              <span className="order-1 flex shrink-0 sm:order-none">
                <Button variant="ghost" size="sm" className={r.comment ? "text-indigo-600" : "text-zinc-400"} title={tr("备注")} aria-label={tr("备注")}
                  onClick={() => setCommenting((s) => new Set(s).add(ids[i]))}><MessageSquareText size={14} /></Button>
                <Button variant="danger" size="sm" onClick={() => setRules(p.rules.filter((_, j) => j !== i))} aria-label={tr("删除")}><Trash2 size={14} /></Button>
              </span>
            </div>
            {isLogical(r.type) && (
              <ConditionsEditor rule={r} ruleSets={p.ruleSets} depth={1} onChange={(rules) => patch(i, { rules })} />
            )}
            </div>
          )}
        />
      </Card>
      {picked.length > 0 && (
        <div className="sticky bottom-4 z-20 flex flex-wrap items-center gap-2 rounded-md border border-indigo-200 bg-indigo-50 px-3 py-1.5 text-xs dark:border-indigo-900 shadow-lg dark:bg-indigo-950">
          <label className="flex items-center gap-1.5">
            <input type="checkbox" checked={picked.length === ids.length} ref={(el) => { if (el) el.indeterminate = picked.length < ids.length; }}
              onChange={(e) => (e.target.checked ? setSelected(new Set(ids)) : clearSelection())} />
            {tr("已选 {0} 条", picked.length)}
          </label>
          <span className="text-zinc-500">{picking ? tr("点击目标规则上的「移到这条上方」") : tr("拖动已选规则可整组移动，Shift 点击可连选")}</span>
          <div className="ml-auto flex gap-1.5">
            <Button size="sm" onClick={() => moveSelected(0)}><ArrowUpToLine size={14} /> {tr("移到顶部")}</Button>
            <Button size="sm" onClick={toBottom}><ArrowDownToLine size={14} /> {tr("移到底部")}</Button>
            <Button size="sm" variant={picking ? "primary" : "secondary"} onClick={() => setPicking((v) => !v)}><MoveVertical size={14} /> {tr("移到指定位置")}</Button>
            <Button size="sm" variant="ghost" onClick={clearSelection} aria-label={tr("取消选择")} title={tr("取消选择")}><X size={14} /></Button>
          </div>
        </div>
      )}
    </section>
  );
}

function RuleSetsSection({ profile: p, update }: { profile: EditableProfile; update: Upd }) {
  const [edit, setEdit] = useState<{ index: number; rs: RuleSet } | null>(null);
  const [presets, setPresets] = useState(false);
  const [mirror, setMirror] = useState(false);
  const usedBy = (name: string) => p.rules.filter((r) => usesRuleSet(r, name)).length;

  const save = () => {
    if (!edit) return;
    const old = p.ruleSets[edit.index];
    update((o) => {
      const ruleSets = edit.index < 0 ? [...o.ruleSets, edit.rs] : o.ruleSets.map((x, i) => (i === edit.index ? edit.rs : x));
      const rename = (r: Rule) => (r.type === "RULE-SET" && r.value === old.name ? { ...r, value: edit.rs.name } : r);
      const rules = old && old.name !== edit.rs.name ? o.rules.map((r) => mapRule(r, rename)) : o.rules;
      return { ...o, ruleSets, rules };
    });
    setEdit(null);
  };

  return (
    <section className="space-y-2">
      <SectionHeader>
        <h3 className="font-medium">{tr("规则集")}</h3>
        <div className="flex shrink-0 gap-2">
          <Button size="sm" variant="ghost" onClick={() => setMirror(true)} title={tr("查看镜像缓存的规则集")}><DatabaseZap size={14} /> {tr("镜像")}</Button>
          <Button size="sm" onClick={() => setPresets(true)}>{tr("从预设添加")}</Button>
          <Button size="sm" onClick={() => setEdit({ index: -1, rs: { name: "", behavior: "classical", stashFormat: "yaml", interval: 86400 } })}><Plus size={14} /> {tr("自定义")}</Button>
        </div>
      </SectionHeader>
      <p className="text-xs text-zinc-500">
        {tr("远程规则列表。Surge / Loon 使用 Surge 地址，Stash / Clash Meta / FlClash 使用 Stash 地址；sing-box 与 Quantumult X 由本服务的规则集镜像转换格式后提供（sing-box 也可单独填写 .srs 地址）。")}
      </p>
      <div className="space-y-1 pb-1">
        <Toggle checked={!!p.general?.mirrorRules} onChange={(v) => update((o) => ({ ...o, general: { ...o.general, mirrorRules: v || undefined } }))} label={tr("所有客户端都通过本服务镜像加载规则集")} />
        <p className="text-xs text-zinc-500">
          {tr("开启后，生成的配置里规则集地址都指向本服务（")}<code>/rs/…</code>{tr("）：由服务端下载并缓存原始列表、按客户端转换格式，客户端无需直接访问 GitHub 等原地址；原地址暂时不可用时继续提供上次的副本。 要求客户端能访问本服务的公开地址。")}
        </p>
      </div>
      <Card className="divide-y divide-zinc-100 dark:divide-zinc-800">
        {p.ruleSets.length === 0 && <div className="p-4 text-center text-sm text-zinc-500">{tr("没有规则集")}</div>}
        {p.ruleSets.map((rs, i) => (
          <div key={rs.name + i} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-3 py-2 text-sm sm:flex-nowrap">
            <span className="min-w-0 flex-1 truncate font-medium sm:w-32 sm:flex-none sm:shrink-0">{rs.name}</span>
            <Badge>{rs.behavior}</Badge>
            <span className="order-last flex w-full min-w-0 flex-wrap gap-1 sm:order-none sm:w-auto sm:flex-1">
              <Badge color={rs.surgeUrl ? "green" : "red"}>Surge / Loon</Badge>
              <Badge color={rs.stashUrl ? "green" : "red"}>Stash / Clash</Badge>
              {rs.singboxUrl
                ? <Badge color="green">sing-box</Badge>
                : <span title={tr("由本服务的规则集镜像转换格式后提供")}><Badge color={rs.surgeUrl || rs.stashUrl ? "indigo" : "red"}>{tr("sing-box / QX（镜像）")}</Badge></span>}
              <span className="self-center text-xs text-zinc-500 whitespace-nowrap sm:hidden">{usedBy(rs.name) ? tr("被 {0} 条规则引用", usedBy(rs.name)) : tr("未被引用")}</span>
            </span>
            <span className="hidden text-xs text-zinc-500 whitespace-nowrap sm:inline" title={tr("「规则」列表中有几条规则使用了这个规则集（不是规则集文件里的条目数）")}>
              {usedBy(rs.name) ? tr("被 {0} 条规则引用", usedBy(rs.name)) : tr("未被引用")}
            </span>
            <Button size="sm" variant="ghost" onClick={() => setEdit({ index: i, rs: { ...rs } })}>{tr("编辑")}</Button>
            <Button size="sm" variant="danger" onClick={() => {
              const n = usedBy(rs.name);
              if (n && !confirm(tr("{0} 条规则使用「{1}」，删除规则集会同时删除这些规则。", n, rs.name))) return;
              update((o) => ({ ...o, ruleSets: o.ruleSets.filter((_, j) => j !== i), rules: o.rules.filter((r) => !usesRuleSet(r, rs.name)) }));
            }}><Trash2 size={14} /></Button>
          </div>
        ))}
      </Card>

      <Modal open={!!edit} onClose={() => setEdit(null)} title={edit && edit.index >= 0 ? tr("编辑规则集") : tr("添加规则集")}
        footer={<><Button onClick={() => setEdit(null)}>{tr("取消")}</Button><Button variant="primary" disabled={!edit?.rs.name || p.ruleSets.some((x, i) => x.name === edit?.rs.name && i !== edit?.index)} onClick={save}>{tr("确定")}</Button></>}>
        {edit && (
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <Field label={tr("名称")}><Input value={edit.rs.name} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, name: e.target.value } })} /></Field>
              <Field label={tr("类型")} hint={tr("domain 在 Surge 中输出为 DOMAIN-SET")}>
                <Select value={edit.rs.behavior} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, behavior: e.target.value as RuleSet["behavior"] } })}>
                  <option value="classical">{tr("classical（混合规则）")}</option>
                  <option value="domain">{tr("domain（纯域名）")}</option>
                  <option value="ipcidr">{tr("ipcidr（纯 IP 段）")}</option>
                </Select>
              </Field>
            </div>
            <Field label={tr("Surge 地址")}><Input className="font-mono text-xs" value={edit.rs.surgeUrl ?? ""} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, surgeUrl: e.target.value } })} placeholder="https://…/Netflix.list" /></Field>
            <div className="grid grid-cols-[1fr_7rem] gap-3">
              <Field label={tr("Stash 地址")}><Input className="font-mono text-xs" value={edit.rs.stashUrl ?? ""} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, stashUrl: e.target.value } })} placeholder="https://…/Netflix.yaml" /></Field>
              <Field label={tr("Stash 格式")}>
                <Select value={edit.rs.stashFormat ?? "yaml"} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, stashFormat: e.target.value as "yaml" | "text" } })}>
                  <option value="yaml">yaml</option>
                  <option value="text">text</option>
                </Select>
              </Field>
            </div>
            <Field label={tr("sing-box 地址（可选）")} hint={tr("sing-box 规则集（.srs 二进制或 .json 源格式）。留空时由本服务镜像把 Surge / Stash 列表转换为 sing-box 格式。")}>
              <Input className="font-mono text-xs" value={edit.rs.singboxUrl ?? ""} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, singboxUrl: e.target.value.trim() || undefined } })} placeholder="https://…/geosite-netflix.srs" />
            </Field>
            <Field label={tr("更新间隔（秒）")}><Input type="number" value={edit.rs.interval ?? ""} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, interval: +e.target.value || undefined } })} /></Field>
          </div>
        )}
      </Modal>

      <MirrorDialog open={mirror} onClose={() => setMirror(false)} />
      <PresetPicker open={presets} onClose={() => setPresets(false)} existing={p.ruleSets.map((r) => r.name)} policies={["DIRECT", "REJECT", ...p.groups.map((g) => g.name)]}
        onAdd={(sets, ipOnly, policy, addRules) => {
          update((o) => {
            let rules = o.rules;
            if (addRules) {
              const fin = rules.findIndex((r) => r.type === "FINAL");
              const nr = sets.map((rs) => ({ type: "RULE-SET", value: rs.name, policy, noResolve: ipOnly.has(rs.name) || undefined, _uid: uid() }));
              rules = fin >= 0 ? [...rules.slice(0, fin), ...nr, ...rules.slice(fin)] : [...rules, ...nr];
            }
            return { ...o, ruleSets: [...o.ruleSets, ...sets], rules };
          });
          setPresets(false);
        }} />
    </section>
  );
}

function PresetPicker({ open, onClose, existing, policies, onAdd }: {
  open: boolean;
  onClose: () => void;
  existing: string[];
  policies: string[];
  onAdd: (sets: RuleSet[], ipOnly: Set<string>, policy: string, addRules: boolean) => void;
}) {
  const [srcId, setSrcId] = useState(presetSources[0].id);
  const [sel, setSel] = useState<string[]>([]);
  const [q, setQ] = useState("");
  const [policy, setPolicy] = useState(policies[2] ?? "DIRECT");
  const [addRules, setAddRules] = useState(true);
  const src = presetSources.find((x) => x.id === srcId)!;
  const k = q.trim().toLowerCase();
  const commonNames = new Set(src.common.map((c) => c.name));
  const common = src.common.filter((c) => !k || `${c.name} ${c.label}`.toLowerCase().includes(k));
  const rest = src.all.filter((n) => !commonNames.has(n) && (!k || n.toLowerCase().includes(k)));

  const chip = (name: string, label?: string) => {
    const has = existing.includes(name);
    const on = sel.includes(name);
    return (
      <button key={name} type="button" disabled={has} title={has ? tr("已存在同名规则集") : src.ipOnly.has(name) ? tr("纯 IP 列表，规则会加 no-resolve") : undefined}
        onClick={() => setSel((cur) => (cur.includes(name) ? cur.filter((x) => x !== name) : [...cur, name]))}
        className={`rounded-full border px-2.5 py-0.5 text-xs disabled:opacity-40 ${on ? "border-indigo-500 bg-indigo-50 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300" : "border-zinc-300 dark:border-zinc-700"}`}>
        {name}{label && label !== name && <span className="ml-1 text-zinc-500">{label}</span>}
      </button>
    );
  };

  const close = () => { setSel([]); setQ(""); onClose(); };
  return (
    <Modal open={open} onClose={close} title={tr("从预设添加规则集")} wide
      footer={<><Button onClick={close}>{tr("取消")}</Button><Button variant="primary" disabled={!sel.length} onClick={() => { onAdd(sel.map(src.build), src.ipOnly, policy, addRules); close(); }}>{tr("添加")} {sel.length || ""}</Button></>}>
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          {presetSources.map((x) => (
            <Button key={x.id} size="sm" variant={x.id === srcId ? "primary" : "secondary"} onClick={() => { setSrcId(x.id); setSel([]); }}>
              {x.name} <span className="opacity-70">{x.all.length}</span>
            </Button>
          ))}
          <a className="text-xs text-indigo-600 hover:underline" href={`https://github.com/${src.repo}`} target="_blank" rel="noreferrer">{src.repo}</a>
          <Input className="ml-auto h-8 w-48 text-xs" placeholder={tr("搜索规则集")} value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        {common.length > 0 && (
          <div>
            <div className="mb-1.5 text-xs font-medium text-zinc-600 dark:text-zinc-400">{tr("常用")}</div>
            <div className="flex flex-wrap gap-1.5">{common.map((c) => chip(c.name, c.label))}</div>
          </div>
        )}
        {rest.length > 0 && (
          <div>
            <div className="mb-1.5 text-xs font-medium text-zinc-600 dark:text-zinc-400">{tr("全部（")}{rest.length}{tr("）")}</div>
            <div className="flex max-h-56 flex-wrap gap-1.5 overflow-y-auto">{rest.map((n) => chip(n))}</div>
          </div>
        )}
        {!common.length && !rest.length && <p className="text-sm text-zinc-500">{tr("没有匹配的规则集")}</p>}
        {src.id === "acl4ssr" && <p className="text-xs text-zinc-500">{tr("ACL4SSR 的规则是通用 .list 格式，Surge 与 Stash 共用同一个地址（Stash 以 text 格式加载）。")}</p>}
        <Toggle checked={addRules} onChange={setAddRules} label={tr("同时添加规则（插入到 FINAL 之前）")} />
        {addRules && <Field label={tr("规则使用的策略")}><Select value={policy} onChange={(e) => setPolicy(e.target.value)}>{policies.map((x) => <option key={x}>{x}</option>)}</Select></Field>}
        {sel.length > 0 && <p className="text-xs text-zinc-500">{tr("已选：")}{sel.join(tr("、"))}</p>}
      </div>
    </Modal>
  );
}

/**
 * A section title with its actions. It pins below the editor header while its section
 * scrolls by (the editor sets --sticky-h to the header's height).
 */
function SectionHeader({ children }: { children: ReactNode }) {
  return (
    <div className="sticky z-20 -mx-4 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b border-zinc-200 bg-zinc-50/95 px-4 py-2 backdrop-blur md:-mx-8 md:px-8 dark:border-zinc-800 dark:bg-zinc-950/95"
      style={{ top: "var(--sticky-h, 0px)" }}>
      {children}
    </div>
  );
}

/** Rule types whose rules take no-resolve. */
const noResolveTypes = ["IP-CIDR", "IP-CIDR6", "IP-ASN", "GEOIP", "RULE-SET"];

const logicalHint = (t: string) =>
  t === "AND" ? tr("以下条件全部满足时匹配") : t === "OR" ? tr("以下任一条件满足时匹配") : tr("以下条件不满足时匹配");

/**
 * The fields to change when a rule or condition switches type. Turning a plain rule into
 * AND / OR / NOT keeps it as the first condition; leaving AND / OR / NOT drops the conditions.
 */
function changeType(r: Rule, type: string): Partial<Rule> {
  if (isLogical(type)) {
    const conds: Rule[] = isLogical(r.type) ? (r.rules ?? [])
      : [{ type: r.type === "FINAL" ? "DOMAIN-SUFFIX" : r.type, value: r.type === "FINAL" ? "" : r.value, policy: "", noResolve: r.noResolve }];
    return { type, value: "", noResolve: undefined, rules: type === "NOT" ? conds.slice(0, 1) : conds };
  }
  return { type, value: "", rules: undefined };
}

/** The value control of a plain rule or condition: a rule set or protocol picker, or a text input. */
function RuleValue({ r, ruleSets, onChange }: { r: Rule; ruleSets: RuleSet[]; onChange: (v: Partial<Rule>) => void }) {
  if (r.type === "RULE-SET") {
    return (
      <Select className="h-8 min-w-0 flex-1 text-xs" value={r.value} onChange={(e) => onChange({ value: e.target.value })}>
        <option value="">{tr("选择规则集")}</option>
        {ruleSets.map((s) => <option key={s.name}>{s.name}</option>)}
      </Select>
    );
  }
  if (r.type === "PROTOCOL") {
    return (
      <Select className="h-8 min-w-0 flex-1 text-xs" value={r.value} onChange={(e) => onChange({ value: e.target.value })}>
        <option value="">{tr("选择协议")}</option>
        {protocolValues.map((v) => <option key={v}>{v}</option>)}
      </Select>
    );
  }
  return <Input className="h-8 min-w-0 flex-1 font-mono text-xs" value={r.value} onChange={(e) => onChange({ value: e.target.value })} placeholder={r.type === "GEOIP" ? "CN" : "example.com"} />;
}

/** Edits the conditions of an AND / OR / NOT rule; conditions may be logical themselves. */
function ConditionsEditor({ rule, ruleSets, depth, onChange }: { rule: Rule; ruleSets: RuleSet[]; depth: number; onChange: (rules: Rule[]) => void }) {
  const conds = rule.rules ?? [];
  const types = ruleTypes.filter((t) => t !== "FINAL" && (depth < maxLogicalDepth || !isLogical(t)));
  const set = (j: number, v: Partial<Rule>) => onChange(conds.map((c, k) => (k === j ? { ...c, ...v } : c)));
  const canAdd = rule.type !== "NOT" || conds.length === 0;
  return (
    <div className="mt-2 space-y-1.5 border-l-2 border-indigo-200 pl-3 sm:ml-11 dark:border-indigo-900">
      {conds.length === 0 && <p className="text-xs text-amber-600">{tr("还没有条件，至少添加一个才能保存。")}</p>}
      {conds.map((c, j) => (
        <div key={j}>
          <div className="flex flex-wrap items-center gap-2 sm:flex-nowrap">
            <Select className="h-8 min-w-0 flex-1 text-xs sm:w-40 sm:flex-none sm:shrink-0" value={c.type} onChange={(e) => set(j, changeType(c, e.target.value))}>
              {types.map((t) => <option key={t}>{t}</option>)}
            </Select>
            <div className="order-2 flex w-full min-w-0 sm:order-none sm:w-auto sm:flex-1">
              {isLogical(c.type)
                ? <span className="min-w-0 flex-1 truncate text-xs text-zinc-500" title={ruleExpr(c)}>{logicalHint(c.type)}</span>
                : <RuleValue r={c} ruleSets={ruleSets} onChange={(v) => set(j, v)} />}
            </div>
            {noResolveTypes.includes(c.type) && (
              <label className="order-3 flex items-center gap-1 text-xs whitespace-nowrap sm:order-none"><input type="checkbox" checked={!!c.noResolve} onChange={(e) => set(j, { noResolve: e.target.checked || undefined })} />no-resolve</label>
            )}
            <Button variant="ghost" size="sm" className="order-1 text-zinc-400 sm:order-none" aria-label={tr("删除条件")} title={tr("删除条件")}
              onClick={() => onChange(conds.filter((_, k) => k !== j))}><X size={14} /></Button>
          </div>
          {isLogical(c.type) && <ConditionsEditor rule={c} ruleSets={ruleSets} depth={depth + 1} onChange={(rules) => set(j, { rules })} />}
        </div>
      ))}
      {canAdd && (
        <Button variant="ghost" size="sm" className="text-indigo-600 dark:text-indigo-400"
          onClick={() => onChange([...conds, { type: "DOMAIN-SUFFIX", value: "", policy: "" }])}>
          <Plus size={14} /> {tr("添加条件")}
        </Button>
      )}
    </div>
  );
}
