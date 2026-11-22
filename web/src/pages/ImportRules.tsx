import { useMemo, useState } from "react";
import type { Rule, RuleSet } from "../api";
import { Badge, Button, Field, Modal, Select, Textarea, Toggle } from "../components/ui";
import { tr } from "../i18n";

/** Rule types the editor offers. Imported lines may use any of them. */
export const ruleTypes = [
  "RULE-SET", "DOMAIN-SET", "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "DOMAIN-WILDCARD",
  "IP-CIDR", "IP-CIDR6", "IP-ASN", "GEOIP", "SRC-IP", "DST-PORT", "IN-PORT",
  "PROCESS-NAME", "PROCESS-PATH", "USER-AGENT", "URL-REGEX", "FINAL",
];

const aliases: Record<string, string> = { MATCH: "FINAL", "DEST-PORT": "DST-PORT", "SRC-IP-CIDR": "SRC-IP", "IP6-CIDR": "IP-CIDR6" };

interface Parsed {
  rule: Rule;
  /** Rule set to create for RULE-SET / DOMAIN-SET lines that point to a URL. */
  newSet?: RuleSet;
  line: number;
}

interface ParseResult {
  items: Parsed[];
  errors: { line: number; text: string; reason: string }[];
}

/**
 * Parses Surge / Clash style rule lines:
 *   TYPE,VALUE,POLICY[,no-resolve][,other options]
 *   FINAL,POLICY  /  MATCH,POLICY
 * Accepts YAML list items ("- DOMAIN,..."), and skips blank lines and [Section] headers.
 * Comment lines ("# …") are kept as the comment of the rule that follows them.
 */
export function parseRules(text: string): ParseResult {
  const items: Parsed[] = [];
  const errors: ParseResult["errors"] = [];
  let note: string[] = [];
  // Attaches the pending comment lines to the rule.
  const push = (p: Parsed) => {
    if (note.length) p.rule = { ...p.rule, comment: note.join("\n") };
    note = [];
    items.push(p);
  };
  text.split(/\r?\n/).forEach((raw, idx) => {
    const line = idx + 1;
    let t = raw.trim().replace(/^-\s+/, "").replace(/^['"]|['"]$/g, "");
    const c = t.match(/^(?:#|\/\/|;)\s*(.*)$/);
    if (c) { if (c[1].trim()) note.push(c[1].trim()); return; }
    if (!t) return;
    if (/^\[.*\]$/.test(t)) { note = []; return; }
    t = t.replace(/\s+(#|\/\/).*$/, ""); // trailing comment
    const parts = t.split(",").map((x) => x.trim());
    let type = parts[0].toUpperCase();
    type = aliases[type] ?? type;
    if (!ruleTypes.includes(type)) {
      errors.push({ line, text: raw.trim(), reason: tr("不支持的规则类型 {0}", parts[0]) });
      return;
    }
    if (type === "FINAL") {
      if (!parts[1]) return void errors.push({ line, text: raw.trim(), reason: tr("缺少策略") });
      push({ line, rule: { type, value: "", policy: parts[1] } });
      return;
    }
    if (parts.length < 3 || !parts[1] || !parts[2]) {
      errors.push({ line, text: raw.trim(), reason: tr("格式应为 类型,值,策略") });
      return;
    }
    const opts = parts.slice(3).map((o) => o.toLowerCase());
    // Domains pasted as links ("http://example.com") lose the scheme and path.
    const value = type.startsWith("DOMAIN") && type !== "DOMAIN-SET" ? parts[1].replace(/^[a-z]+:\/\//i, "").replace(/\/.*$/, "") : parts[1];
    const rule: Rule = { type, value, policy: parts[2], noResolve: opts.includes("no-resolve") || undefined };
    if (type === "RULE-SET" || type === "DOMAIN-SET") {
      if (!/^https?:\/\//i.test(parts[1])) {
        // A rule-set name; it must already exist in the profile (checked by the caller).
        push({ line, rule: { ...rule, type: "RULE-SET" } });
        return;
      }
      const url = parts[1];
      const name = decodeURIComponent(url.split(/[?#]/)[0].split("/").pop() ?? "rules").replace(/\.(list|txt|yaml|yml|conf)$/i, "") || "rules";
      const behavior = type === "DOMAIN-SET" ? "domain" : "classical";
      const newSet: RuleSet = { name, behavior, surgeUrl: url, stashUrl: url, stashFormat: /\.ya?ml$/i.test(url.split(/[?#]/)[0]) ? "yaml" : "text", interval: 86400 };
      push({ line, rule: { ...rule, type: "RULE-SET", value: name }, newSet });
      return;
    }
    push({ line, rule });
  });
  return { items, errors };
}

export function ImportRulesDialog({ open, onClose, policies, ruleSets, existing, onImport }: {
  open: boolean;
  onClose: () => void;
  policies: string[]; // DIRECT, REJECT and group names
  ruleSets: RuleSet[];
  existing: Rule[];
  onImport: (rules: Rule[], newSets: RuleSet[], position: "top" | "beforeFinal") => void;
}) {
  const [text, setText] = useState("");
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [skipDup, setSkipDup] = useState(true);
  const [position, setPosition] = useState<"top" | "beforeFinal">("top");

  const parsed = useMemo(() => parseRules(text), [text]);
  const exact = (p: string) => policies.find((x) => x === p) ?? policies.find((x) => x.toLowerCase() === p.toLowerCase());
  const unknown = useMemo(
    () => [...new Set(parsed.items.map((i) => i.rule.policy).filter((p) => !exact(p)))],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [parsed, policies],
  );
  const guess = (p: string) => (/direct/i.test(p) ? "DIRECT" : /reject|block|广告|^ads?$/i.test(p) ? "REJECT" : policies[2] ?? "DIRECT");
  const policyFor = (p: string) => exact(p) ?? mapping[p] ?? guess(p);

  const setNames = new Set(ruleSets.map((r) => r.name));
  const key = (r: Rule) => `${r.type}|${r.value.toLowerCase()}`;
  const have = new Set(existing.map(key));
  const result = useMemo(() => {
    const rules: Rule[] = [];
    const newSets: RuleSet[] = [];
    const skipped: string[] = [];
    const seen = new Set(have);
    const taken = new Set(setNames);
    for (const it of parsed.items) {
      const r = { ...it.rule, policy: policyFor(it.rule.policy) };
      if (r.type === "FINAL") {
        if (existing.some((x) => x.type === "FINAL")) { skipped.push(tr("第 {0} 行：已有 FINAL 规则", it.line)); continue; }
      } else if (r.type === "RULE-SET" && !it.newSet && !taken.has(r.value)) {
        skipped.push(tr("第 {0} 行：规则集「{1}」不存在", it.line, r.value));
        continue;
      }
      if (it.newSet) {
        let name = it.newSet.name;
        for (let n = 2; taken.has(name) && !ruleSets.some((s) => s.name === name && s.surgeUrl === it.newSet!.surgeUrl); n++) name = `${it.newSet.name} ${n}`;
        if (!taken.has(name)) { newSets.push({ ...it.newSet, name }); taken.add(name); }
        r.value = name;
      }
      if (skipDup && seen.has(key(r))) { skipped.push(tr("第 {0} 行：重复的 {1},{2}", it.line, r.type, r.value)); continue; }
      seen.add(key(r));
      rules.push(r);
    }
    return { rules, newSets, skipped };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [parsed, mapping, skipDup, existing, ruleSets, policies]);

  const close = () => { setText(""); setMapping({}); onClose(); };

  return (
    <Modal
      open={open}
      onClose={close}
      title={tr("批量导入规则")}
      wide
      footer={
        <>
          <Button onClick={close}>{tr("取消")}</Button>
          <Button variant="primary" disabled={!result.rules.length} onClick={() => { onImport(result.rules, result.newSets, position); close(); }}>
            {tr("导入")} {result.rules.length || ""} {tr("条")}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label={tr("粘贴规则")} hint={tr("每行一条，Surge / Clash 格式均可：类型,值,策略[,no-resolve]。# 开头的注释行会作为下一条规则的备注保留，并写入生成的配置。")}>
          <Textarea
            rows={8}
            autoFocus
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder={tr("DOMAIN,code.example.com,DIRECT\nDOMAIN-SUFFIX,example.com,节点选择\nIP-CIDR,10.0.0.0/8,DIRECT,no-resolve\nRULE-SET,https://example.com/rules.list,节点选择")}
          />
        </Field>

        {unknown.length > 0 && (
          <div className="space-y-2 rounded-md bg-amber-50 dark:bg-amber-950/40 p-3">
            <p className="text-xs text-amber-900 dark:text-amber-200">{tr("以下策略在当前配置中不存在，请选择对应的策略（之后也可以在规则列表里修改）：")}</p>
            {unknown.map((u) => (
              <div key={u} className="flex items-center gap-2 text-sm">
                <code className="min-w-32 rounded bg-white/60 dark:bg-black/30 px-1.5 py-0.5 text-xs">{u}</code>
                <span className="text-zinc-500">→</span>
                <Select className="h-8 w-56 text-xs" value={policyFor(u)} onChange={(e) => setMapping({ ...mapping, [u]: e.target.value })}>
                  {policies.map((x) => <option key={x}>{x}</option>)}
                </Select>
              </div>
            ))}
          </div>
        )}

        {text.trim() && (
          <div className="space-y-2 text-sm">
            <div className="flex flex-wrap items-center gap-2">
              <Badge color="green">{tr("可导入")} {result.rules.length}</Badge>
              {result.newSets.length > 0 && <Badge color="indigo">{tr("新建规则集")} {result.newSets.length}</Badge>}
              {result.skipped.length > 0 && <Badge color="amber">{tr("跳过")} {result.skipped.length}</Badge>}
              {parsed.errors.length > 0 && <Badge color="red">{tr("无法识别")} {parsed.errors.length}</Badge>}
              <span className="ml-auto flex items-center gap-4">
                <Select className="h-8 w-auto text-xs" value={position} onChange={(e) => setPosition(e.target.value as "top" | "beforeFinal")} title={tr("规则从上到下匹配，自定义规则通常放在最前面，避免被前面的规则集先匹配")}>
                  <option value="top">{tr("插入到最前面")}</option>
                  <option value="beforeFinal">{tr("插入到 FINAL 之前")}</option>
                </Select>
                <Toggle checked={skipDup} onChange={setSkipDup} label={tr("跳过重复规则")} />
              </span>
            </div>
            {result.rules.length > 0 && (
              <div className="max-h-48 overflow-y-auto rounded-md border border-zinc-200 dark:border-zinc-800">
                {result.rules.map((r, i) => (
                  <div key={i} className="flex gap-3 border-b border-zinc-100 dark:border-zinc-800 px-3 py-1 font-mono text-xs last:border-0">
                    <span className="w-28 shrink-0 text-zinc-500">{r.type}</span>
                    <span className="min-w-0 flex-1 truncate">{r.value}</span>
                    <span className="shrink-0">{r.policy}{r.noResolve ? " · no-resolve" : ""}</span>
                  </div>
                ))}
              </div>
            )}
            {[...result.skipped, ...parsed.errors.map((e) => tr("第 {0} 行：{1}（{2}）", e.line, e.reason, e.text))].length > 0 && (
              <ul className="list-disc space-y-0.5 pl-5 text-xs text-zinc-500">
                {result.skipped.map((s, i) => <li key={`s${i}`}>{s}</li>)}
                {parsed.errors.map((e, i) => <li key={`e${i}`} className="text-red-600">{tr("第")} {e.line} {tr("行：")}{e.reason}{tr("（")}{e.text}{tr("）")}</li>)}
              </ul>
            )}
          </div>
        )}
      </div>
    </Modal>
  );
}
