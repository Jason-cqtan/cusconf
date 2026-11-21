import { useState } from "react";
import { ClipboardPaste, Plus, Trash2 } from "lucide-react";
import type { Rule, RuleSet } from "../api";
import { Badge, Button, Card, Field, Input, Modal, Select, Toggle } from "../components/ui";
import { SortableList } from "../components/Sortable";
import { uid, type EditableProfile, type EditableRule } from "./ProfileEditor";
import { ImportRulesDialog, ruleTypes } from "./ImportRules";


const bm7Presets = [
  "OpenAI", "Claude", "Gemini", "Copilot", "Netflix", "Disney", "YouTube", "Spotify", "TikTok", "HBO", "PrimeVideo", "Hulu",
  "Telegram", "X", "Facebook", "Instagram", "Google", "GitHub", "Microsoft", "Apple", "AppleMedia", "Steam", "Epic",
  "PayPal", "Speedtest", "Advertising", "Privacy", "Global", "ChinaMax", "China", "Lan",
];

// Display name → folder name in blackmatrix7/ios_rule_script, where they differ.
const bm7Folder: Record<string, string> = { X: "Twitter" };

function bm7(name: string): RuleSet {
  const base = "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/";
  const dir = bm7Folder[name] ?? name;
  return { name, behavior: "classical", surgeUrl: `${base}Surge/${dir}/${dir}.list`, stashUrl: `${base}Clash/${dir}/${dir}.yaml`, stashFormat: "yaml", interval: 86400 };
}

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

  return (
    <section className="space-y-2">
      <div className="flex items-end justify-between">
        <div>
          <h3 className="font-medium">规则</h3>
          <p className="text-xs text-zinc-500">从上到下匹配。最后一条应为 FINAL（Stash 中输出为 MATCH）。</p>
        </div>
        <div className="flex gap-2">
          <Button size="sm" onClick={() => setImporting(true)}><ClipboardPaste size={14} /> 批量导入</Button>
          <Button size="sm" onClick={() => {
            const r: EditableRule = { type: "DOMAIN-SUFFIX", value: "", policy: p.groups[0]?.name ?? "DIRECT", _uid: uid() };
            const fin = p.rules.findIndex((x) => x.type === "FINAL");
            setRules(fin >= 0 ? [...p.rules.slice(0, fin), r, ...p.rules.slice(fin)] : [...p.rules, r]);
          }}><Plus size={14} /> 添加规则</Button>
        </div>
      </div>
      <ImportRulesDialog
        open={importing}
        onClose={() => setImporting(false)}
        policies={policies}
        ruleSets={p.ruleSets}
        existing={p.rules}
        onImport={(rules, newSets, position) => update((o) => {
          const add = rules.map((r) => ({ ...r, _uid: uid() }));
          const fin = o.rules.findIndex((x) => x.type === "FINAL");
          const merged = position === "top" ? [...add, ...o.rules]
            : fin >= 0 ? [...o.rules.slice(0, fin), ...add, ...o.rules.slice(fin)] : [...o.rules, ...add];
          return { ...o, ruleSets: [...o.ruleSets, ...newSets], rules: merged };
        })}
      />
      <Card className="overflow-hidden">
        <SortableList
          items={p.rules}
          ids={ids}
          onReorder={setRules}
          render={(r, i, handle) => (
            <div className="flex flex-wrap items-center gap-2 border-b border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-3 py-2 sm:flex-nowrap">
              {handle}
              <Select className="h-8 w-40 shrink-0 text-xs" value={r.type} onChange={(e) => patch(i, { type: e.target.value, value: "" })}>
                {ruleTypes.map((t) => <option key={t}>{t}</option>)}
              </Select>
              {r.type === "RULE-SET" ? (
                <Select className="h-8 min-w-0 flex-1 text-xs" value={r.value} onChange={(e) => patch(i, { value: e.target.value })}>
                  <option value="">选择规则集</option>
                  {p.ruleSets.map((s) => <option key={s.name}>{s.name}</option>)}
                </Select>
              ) : r.type === "FINAL" ? (
                <span className="flex-1 text-xs text-zinc-500">其余所有流量</span>
              ) : (
                <Input className="h-8 min-w-0 flex-1 font-mono text-xs" value={r.value} onChange={(e) => patch(i, { value: e.target.value })} placeholder={r.type === "GEOIP" ? "CN" : "example.com"} />
              )}
              <Select className="h-8 w-44 shrink-0 text-xs" value={r.policy} onChange={(e) => patch(i, { policy: e.target.value })}>
                {policies.map((x) => <option key={x}>{x}</option>)}
              </Select>
              {["IP-CIDR", "IP-CIDR6", "GEOIP", "RULE-SET"].includes(r.type) && (
                <label className="flex items-center gap-1 text-xs whitespace-nowrap"><input type="checkbox" checked={!!r.noResolve} onChange={(e) => patch(i, { noResolve: e.target.checked })} />no-resolve</label>
              )}
              <Button variant="danger" size="sm" onClick={() => setRules(p.rules.filter((_, j) => j !== i))} aria-label="删除"><Trash2 size={14} /></Button>
            </div>
          )}
        />
      </Card>
    </section>
  );
}

function RuleSetsSection({ profile: p, update }: { profile: EditableProfile; update: Upd }) {
  const [edit, setEdit] = useState<{ index: number; rs: RuleSet } | null>(null);
  const [presets, setPresets] = useState(false);
  const usedBy = (name: string) => p.rules.filter((r) => r.type === "RULE-SET" && r.value === name).length;

  const save = () => {
    if (!edit) return;
    const old = p.ruleSets[edit.index];
    update((o) => {
      const ruleSets = edit.index < 0 ? [...o.ruleSets, edit.rs] : o.ruleSets.map((x, i) => (i === edit.index ? edit.rs : x));
      const rules = old && old.name !== edit.rs.name ? o.rules.map((r) => (r.type === "RULE-SET" && r.value === old.name ? { ...r, value: edit.rs.name } : r)) : o.rules;
      return { ...o, ruleSets, rules };
    });
    setEdit(null);
  };

  return (
    <section className="space-y-2">
      <div className="flex items-end justify-between">
        <div>
          <h3 className="font-medium">规则集</h3>
          <p className="text-xs text-zinc-500">远程规则列表。Surge 与 Stash 需要不同格式的文件，分别填写地址。</p>
        </div>
        <div className="flex gap-2">
          <Button size="sm" onClick={() => setPresets(true)}>从预设添加</Button>
          <Button size="sm" onClick={() => setEdit({ index: -1, rs: { name: "", behavior: "classical", stashFormat: "yaml", interval: 86400 } })}><Plus size={14} /> 自定义</Button>
        </div>
      </div>
      <Card className="divide-y divide-zinc-100 dark:divide-zinc-800">
        {p.ruleSets.length === 0 && <div className="p-4 text-center text-sm text-zinc-500">没有规则集</div>}
        {p.ruleSets.map((rs, i) => (
          <div key={rs.name + i} className="flex items-center gap-3 px-3 py-2 text-sm">
            <span className="w-32 shrink-0 truncate font-medium">{rs.name}</span>
            <Badge>{rs.behavior}</Badge>
            <span className="flex min-w-0 flex-1 gap-1">
              <Badge color={rs.surgeUrl ? "green" : "red"}>Surge</Badge>
              <Badge color={rs.stashUrl ? "green" : "red"}>Stash</Badge>
            </span>
            <span className="text-xs text-zinc-500 whitespace-nowrap" title="「规则」列表中有几条规则使用了这个规则集（不是规则集文件里的条目数）">
              {usedBy(rs.name) ? `被 ${usedBy(rs.name)} 条规则引用` : "未被引用"}
            </span>
            <Button size="sm" variant="ghost" onClick={() => setEdit({ index: i, rs: { ...rs } })}>编辑</Button>
            <Button size="sm" variant="danger" onClick={() => {
              const n = usedBy(rs.name);
              if (n && !confirm(`${n} 条规则使用「${rs.name}」，删除规则集会同时删除这些规则。`)) return;
              update((o) => ({ ...o, ruleSets: o.ruleSets.filter((_, j) => j !== i), rules: o.rules.filter((r) => !(r.type === "RULE-SET" && r.value === rs.name)) }));
            }}><Trash2 size={14} /></Button>
          </div>
        ))}
      </Card>

      <Modal open={!!edit} onClose={() => setEdit(null)} title={edit && edit.index >= 0 ? "编辑规则集" : "添加规则集"}
        footer={<><Button onClick={() => setEdit(null)}>取消</Button><Button variant="primary" disabled={!edit?.rs.name || p.ruleSets.some((x, i) => x.name === edit?.rs.name && i !== edit?.index)} onClick={save}>确定</Button></>}>
        {edit && (
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <Field label="名称"><Input value={edit.rs.name} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, name: e.target.value } })} /></Field>
              <Field label="类型" hint="domain 在 Surge 中输出为 DOMAIN-SET">
                <Select value={edit.rs.behavior} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, behavior: e.target.value as RuleSet["behavior"] } })}>
                  <option value="classical">classical（混合规则）</option>
                  <option value="domain">domain（纯域名）</option>
                  <option value="ipcidr">ipcidr（纯 IP 段）</option>
                </Select>
              </Field>
            </div>
            <Field label="Surge 地址"><Input className="font-mono text-xs" value={edit.rs.surgeUrl ?? ""} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, surgeUrl: e.target.value } })} placeholder="https://…/Netflix.list" /></Field>
            <div className="grid grid-cols-[1fr_7rem] gap-3">
              <Field label="Stash 地址"><Input className="font-mono text-xs" value={edit.rs.stashUrl ?? ""} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, stashUrl: e.target.value } })} placeholder="https://…/Netflix.yaml" /></Field>
              <Field label="Stash 格式">
                <Select value={edit.rs.stashFormat ?? "yaml"} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, stashFormat: e.target.value as "yaml" | "text" } })}>
                  <option value="yaml">yaml</option>
                  <option value="text">text</option>
                </Select>
              </Field>
            </div>
            <Field label="更新间隔（秒）"><Input type="number" value={edit.rs.interval ?? ""} onChange={(e) => setEdit({ ...edit, rs: { ...edit.rs, interval: +e.target.value || undefined } })} /></Field>
          </div>
        )}
      </Modal>

      <PresetPicker open={presets} onClose={() => setPresets(false)} existing={p.ruleSets.map((r) => r.name)} policies={["DIRECT", "REJECT", ...p.groups.map((g) => g.name)]}
        onAdd={(names, policy, addRules) => {
          update((o) => {
            const sets = names.map(bm7);
            let rules = o.rules;
            if (addRules) {
              const fin = rules.findIndex((r) => r.type === "FINAL");
              const nr = names.map((n) => ({ type: "RULE-SET", value: n, policy, _uid: uid() }));
              rules = fin >= 0 ? [...rules.slice(0, fin), ...nr, ...rules.slice(fin)] : [...rules, ...nr];
            }
            return { ...o, ruleSets: [...o.ruleSets, ...sets], rules };
          });
          setPresets(false);
        }} />
    </section>
  );
}

function PresetPicker({ open, onClose, existing, policies, onAdd }: { open: boolean; onClose: () => void; existing: string[]; policies: string[]; onAdd: (names: string[], policy: string, addRules: boolean) => void }) {
  const [sel, setSel] = useState<string[]>([]);
  const [policy, setPolicy] = useState(policies[2] ?? "DIRECT");
  const [addRules, setAddRules] = useState(true);
  return (
    <Modal open={open} onClose={onClose} title="从 blackmatrix7/ios_rule_script 添加"
      footer={<><Button onClick={onClose}>取消</Button><Button variant="primary" disabled={!sel.length} onClick={() => { onAdd(sel, policy, addRules); setSel([]); }}>添加 {sel.length || ""}</Button></>}>
      <div className="space-y-4">
        <div className="flex flex-wrap gap-1.5">
          {bm7Presets.map((n) => {
            const has = existing.includes(n);
            const on = sel.includes(n);
            return (
              <button key={n} disabled={has} onClick={() => setSel((cur) => (cur.includes(n) ? cur.filter((x) => x !== n) : [...cur, n]))}
                className={`rounded-full border px-2.5 py-0.5 text-xs disabled:opacity-40 ${on ? "border-indigo-500 bg-indigo-50 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300" : "border-zinc-300 dark:border-zinc-700"}`}>
                {n}
              </button>
            );
          })}
        </div>
        <Toggle checked={addRules} onChange={setAddRules} label="同时添加规则（插入到 FINAL 之前）" />
        {addRules && <Field label="规则使用的策略"><Select value={policy} onChange={(e) => setPolicy(e.target.value)}>{policies.map((x) => <option key={x}>{x}</option>)}</Select></Field>}
      </div>
    </Modal>
  );
}
