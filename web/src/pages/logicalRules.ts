import type { Rule } from "../api";
import { tr } from "../i18n";

/** Rule types that combine other rules (conditions without a policy). */
export const logicalTypes = ["AND", "OR", "NOT"];
export const isLogical = (t: string) => logicalTypes.includes(t.toUpperCase());

/** The deepest nesting allowed (AND inside OR inside …); the server checks the same. */
export const maxLogicalDepth = 4;

/** The rule without its policy, in Surge syntax: AND,((PROTOCOL,STUN),(DOMAIN,a.com)). */
export function ruleExpr(r: Rule): string {
  if (!isLogical(r.type)) return `${r.type},${r.value}${r.noResolve ? ",no-resolve" : ""}`;
  return `${r.type},(${(r.rules ?? []).map((s) => `(${ruleExpr(s)})`).join(",")})`;
}

/** Whether the rule or any of its conditions satisfies f. */
export function someRule(r: Rule, f: (r: Rule) => boolean): boolean {
  return f(r) || (r.rules ?? []).some((s) => someRule(s, f));
}

/** Applies f to the rule and, depth first, to every condition, rebuilding the tree. */
export function mapRule(r: Rule, f: (r: Rule) => Rule): Rule {
  const m = f(r);
  return m.rules ? { ...m, rules: m.rules.map((s) => mapRule(s, f)) } : m;
}

export const usesRuleSet = (r: Rule, name: string) => someRule(r, (s) => s.type === "RULE-SET" && s.value === name);

/** Splits on commas that are not inside parentheses. */
export function splitTop(s: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let cur = "";
  for (const ch of s) {
    if (ch === "(") depth++;
    else if (ch === ")") depth--;
    if (ch === "," && depth === 0) {
      out.push(cur.trim());
      cur = "";
    } else cur += ch;
  }
  out.push(cur.trim());
  return out;
}

const unwrap = (s: string) => {
  const t = s.trim();
  return t.startsWith("(") && t.endsWith(")") ? t.slice(1, -1).trim() : null;
};

/**
 * Parses the conditions of a logical rule: "((A),(B))" (or "(A)" for a single one), where
 * each condition is TYPE,VALUE[,no-resolve] or itself logical. Throws on malformed text.
 * normalize converts one plain condition (type aliases, value cleanup) or throws.
 */
export function parseConditions(group: string, normalize: (c: Rule) => Rule, depth = 1): Rule[] {
  if (depth > maxLogicalDepth) throw new Error(tr("逻辑规则最多嵌套 {0} 层", maxLogicalDepth));
  const inner = unwrap(group);
  if (inner === null) throw new Error(tr("条件要放在括号里：((条件),(条件))"));
  // "((A),(B))" unwraps to "(A),(B)"; a lone "(A)" unwraps to "A".
  const items = inner.startsWith("(") ? splitTop(inner).map((x) => unwrap(x)) : [inner];
  return items.map((item) => {
    if (!item) throw new Error(tr("有空的条件"));
    const parts = splitTop(item);
    const type = parts[0].toUpperCase();
    if (isLogical(type)) {
      if (!parts[1]) throw new Error(tr("{0} 没有条件", type));
      return { type, value: "", policy: "", rules: parseConditions(parts[1], normalize, depth + 1) };
    }
    if (!parts[1]) throw new Error(tr("{0} 缺少值", type));
    const noResolve = parts.slice(2).some((o) => o.toLowerCase() === "no-resolve") || undefined;
    return normalize({ type, value: parts[1], policy: "", noResolve });
  });
}

/** Checks the shape the server requires: NOT has one condition, AND / OR at least one. */
export function conditionsError(r: Rule): string | null {
  const n = r.rules?.length ?? 0;
  if (r.type === "NOT" && n !== 1) return tr("NOT 只能有一个条件");
  if (n === 0) return tr("{0} 没有条件", r.type);
  for (const s of r.rules ?? []) {
    if (isLogical(s.type)) {
      const e = conditionsError(s);
      if (e) return e;
    } else if (!s.value.trim()) return tr("{0} 缺少值", s.type);
  }
  return null;
}
