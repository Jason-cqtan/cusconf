import { useCallback, useEffect, useRef, useState } from "react";
import { Activity, Gauge, Plus, Settings2, Square, Trash2 } from "lucide-react";
import { api, formatTime, type IPSource, type Node, type ServiceStatus, type Source, type TestJob, type TestResult, type TestSettings } from "../api";
import { MultiSelect } from "../components/MultiSelect";
import { Badge, Button, ErrorText, Field, Input, Modal, Toggle, cx } from "../components/ui";

// --- service list (cached for the session) ---

let servicesCache: { id: string; name: string }[] | null = null;
let servicesPromise: Promise<{ id: string; name: string }[]> | null = null;

export function useServices() {
  const [list, setList] = useState(servicesCache ?? []);
  useEffect(() => {
    if (servicesCache) return;
    servicesPromise ??= api.testServices().then((r) => (servicesCache = r.services));
    servicesPromise.then(setList).catch(() => {});
  }, []);
  return list;
}

export const serviceName = (id: string, list: { id: string; name: string }[]) => list.find((s) => s.id === id)?.name ?? id;

// --- cells ---

export function latencyColor(ms: number) {
  if (ms <= 0) return "text-red-600";
  if (ms < 150) return "text-emerald-600 dark:text-emerald-400";
  if (ms < 400) return "text-amber-600 dark:text-amber-400";
  return "text-red-600";
}

export function LatencyCell({ r }: { r?: TestResult }) {
  if (!r) return <span className="text-zinc-400">—</span>;
  const warn = r.warning && <span className="ml-1 text-amber-500" title={r.warning}>⚠</span>;
  if (r.latency <= 0) return <span className="text-xs text-red-600" title={[r.error, r.warning].filter(Boolean).join("\n")}>超时{warn}</span>;
  return <span className={cx("font-mono text-xs", latencyColor(r.latency))}>{r.latency} ms{warn}</span>;
}

export function scoreColor(v?: number): "green" | "amber" | "red" | "zinc" {
  if (v === undefined || v === null) return "zinc";
  if (v >= 75) return "green";
  if (v >= 40) return "amber";
  return "red";
}

/** One source's safety score (0–100, higher is safer). */
export function ScoreBadge({ v }: { v?: number }) {
  if (v === undefined || v === null) return <span className="text-zinc-400">—</span>;
  return <Badge color={scoreColor(v)}>{v}</Badge>;
}

/** Labels reported about the exit IP (residential, data center, VPN ...). */
export function FlagsCell({ r }: { r?: TestResult }) {
  const f = r?.exit?.flags;
  if (!f?.length) return <span className="text-zinc-400">—</span>;
  return <span className="inline-flex gap-1">{f.map((x) => <Badge key={x}>{x}</Badge>)}</span>;
}

/** IP data sources that appear in results, built-ins first. */
export function reportSources(results: Record<string, TestResult>): string[] {
  const seen = new Set<string>();
  for (const r of Object.values(results)) for (const rep of r.reports ?? []) seen.add(rep.source);
  const order = ["ip-api.com", "proxycheck.io", "IPPure", "Net.Coffee"];
  return [...seen].sort((a, b) => ((order.indexOf(a) + 1 || 99) - (order.indexOf(b) + 1 || 99)) || a.localeCompare(b));
}

const statusStyle: Record<ServiceStatus, { color: "green" | "amber" | "red" | "zinc"; mark: string }> = {
  yes: { color: "green", mark: "" },
  partial: { color: "amber", mark: "½ " },
  no: { color: "red", mark: "✕ " },
  unknown: { color: "zinc", mark: "? " },
  error: { color: "zinc", mark: "! " },
};

export function UnlockCell({ r, services, all }: { r?: TestResult; services: { id: string; name: string }[]; all?: boolean }) {
  const s = r?.services;
  if (!s || !Object.keys(s).length) return <span className="text-zinc-400">—</span>;
  const ids = services.map((x) => x.id).filter((id) => s[id] && (all || s[id].status === "yes" || s[id].status === "partial"));
  if (!ids.length) return <span className="text-xs text-zinc-500">无</span>;
  return (
    <span className="inline-flex flex-wrap gap-1">
      {ids.map((id) => {
        const v = s[id];
        const st = statusStyle[v.status];
        return (
          <Badge key={id} color={st.color}>
            <span title={[v.region && `地区 ${v.region}`, v.note].filter(Boolean).join(" · ") || undefined}>
              {st.mark}{serviceName(id, services)}{v.region ? ` ${v.region}` : ""}
            </span>
          </Badge>
        );
      })}
    </span>
  );
}

// --- detail ---

export function TestDetail({ r, onClose }: { r: TestResult | null; onClose: () => void }) {
  const services = useServices();
  return (
    <Modal open={!!r} onClose={onClose} title={r ? `测试结果 · ${r.name}` : ""} wide>
      {r && (
        <div className="space-y-5 text-sm">
          <div className="flex flex-wrap gap-x-6 gap-y-1">
            <span>延迟 <LatencyCell r={r} /></span>
            <span className="text-zinc-500">测试于 {formatTime(r.testedAt)}</span>
            {r.error && <span className="text-red-600">{r.error}</span>}
          </div>
          {r.warning && <p className="rounded-md bg-amber-50 dark:bg-amber-950/40 px-3 py-2 text-xs text-amber-900 dark:text-amber-200">⚠ {r.warning}</p>}
          {r.exit && (
            <div>
              <h4 className="mb-1 font-medium">出口 IP</h4>
              <div className="grid gap-x-6 gap-y-1 sm:grid-cols-2">
                <span>IP：<code>{r.exit.ip ?? "—"}</code></span>
                <span>地区：{r.exit.country ?? "—"} {r.exit.city}</span>
                <span>运营商：{r.exit.isp ?? "—"}</span>
                <span>ASN：{r.exit.asn ?? "—"}</span>
                <span className="sm:col-span-2 flex flex-wrap items-center gap-x-3 gap-y-1">
                  安全分：
                  {Object.entries(r.exit.scores ?? {}).map(([k, v]) => <span key={k} className="inline-flex items-center gap-1 text-xs">{k} <ScoreBadge v={v} /></span>)}
                  {!Object.keys(r.exit.scores ?? {}).length && <span className="text-zinc-400">—</span>}
                </span>
                <span className="sm:col-span-2">标记：<FlagsCell r={r} /></span>
              </div>
            </div>
          )}
          {!!r.reports?.length && (
            <div>
              <h4 className="mb-1 font-medium">各数据源</h4>
              <table className="w-full text-xs">
                <thead className="text-left text-zinc-500"><tr><th className="py-1">来源</th><th>IP</th><th>地区</th><th>安全分</th><th>标记</th><th>运营商</th></tr></thead>
                <tbody>
                  {r.reports.map((x) => (
                    <tr key={x.source} className="border-t border-zinc-100 dark:border-zinc-800">
                      <td className="py-1 pr-2">{x.source}</td>
                      {x.error ? <td colSpan={5} className="text-red-600">{x.error}</td> : (
                        <>
                          <td className="pr-2 font-mono">{x.ip}</td>
                          <td className="pr-2">{x.country} {x.city}</td>
                          <td className="pr-2"><ScoreBadge v={x.score} /></td>
                          <td className="pr-2">{x.flags?.join("、") || "—"}</td>
                          <td className="truncate">{x.isp}</td>
                        </>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {r.services && (
            <div>
              <h4 className="mb-1 font-medium">服务可用性</h4>
              <div className="grid gap-1 sm:grid-cols-2">
                {services.filter((s) => r.services![s.id]).map((s) => {
                  const v = r.services![s.id];
                  return (
                    <div key={s.id} className="flex items-center gap-2">
                      <span className="w-32">{s.name}</span>
                      <Badge color={statusStyle[v.status].color}>{{ yes: "可用", partial: "部分", no: "不可用", unknown: "未知", error: "失败" }[v.status]}</Badge>
                      {v.region && <span className="text-xs">{v.region}</span>}
                      {v.note && <span className="text-xs text-zinc-500">{v.note}</span>}
                    </div>
                  );
                })}
              </div>
              <p className="mt-2 text-xs text-zinc-500">服务检测基于公开页面的特征判断，网站改版时可能不准。标「按出口地区推断」的项目无法直接探测，按出口所在地区判断。</p>
            </div>
          )}
        </div>
      )}
    </Modal>
  );
}

// --- toolbar with progress ---

/**
 * Test controls for the nodes currently shown. Airport info entries are never tested; the
 * source picker narrows what is tested (and is saved, so scheduled tests use it too).
 */
export function TestToolbar({ nodes, sources, onFinished }: { nodes: Node[]; sources: Source[]; onFinished: () => void }) {
  const [job, setJob] = useState<TestJob | null>(null);
  const [settings, setSettings] = useState<TestSettings | null>(null);
  useEffect(() => { api.testSettings().then(setSettings).catch(() => {}); }, []);
  const testSources = settings?.sources ?? [];
  const setTestSources = async (v: string[]) => {
    if (!settings) return;
    const next = { ...settings, sources: v };
    setSettings(next);
    try { setSettings(await api.saveTestSettings(next)); } catch (e) { setErr((e as Error).message); }
  };
  const keys = [...new Set(nodes.filter((n) => !n.info && (!testSources.length || testSources.includes(n.source ?? ""))).map((n) => n.key))];
  const sourceOptions = sources.map((s) => ({ value: s.id, label: s.name, count: nodes.filter((n) => n.source === s.id && !n.info).length }));
  const [err, setErr] = useState<string | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const wasRunning = useRef(false);

  const poll = useCallback(() => api.testJob().then((j) => {
    setJob(j);
    if (wasRunning.current && !j.running) onFinished();
    wasRunning.current = j.running;
  }).catch(() => {}), [onFinished]);

  useEffect(() => {
    poll();
    const t = setInterval(poll, job?.running ? 1000 : 5000);
    return () => clearInterval(t);
  }, [poll, job?.running]);

  const start = async (full: boolean) => {
    setErr(null);
    try {
      const j = await api.startTest(keys, full, full);
      setJob(j);
      wasRunning.current = true;
    } catch (e) { setErr((e as Error).message); }
  };

  const running = !!job?.running;
  return (
    <div className="flex flex-wrap items-center gap-2">
      {running ? (
        <>
          <div className="flex min-w-48 items-center gap-2 text-xs text-zinc-500">
            <div className="h-1.5 w-32 overflow-hidden rounded bg-zinc-200 dark:bg-zinc-800">
              <div className="h-full bg-indigo-500 transition-all" style={{ width: `${job!.total ? (job!.done / job!.total) * 100 : 0}%` }} />
            </div>
            {job!.done} / {job!.total}{job!.scheduled ? "（定时）" : ""}
          </div>
          <Button size="sm" onClick={() => api.stopTest().then(setJob)}><Square size={12} /> 停止</Button>
        </>
      ) : (
        <>
          <Button size="sm" onClick={() => start(false)} disabled={!keys.length} title="只测延迟"><Gauge size={14} /> 测延迟</Button>
          <Button size="sm" variant="primary" onClick={() => start(true)} disabled={!keys.length} title="延迟、出口 IP 与风险、服务可用性"><Activity size={14} /> 完整测试 {keys.length}</Button>
        </>
      )}
      {sources.length > 1 && (
        <MultiSelect
          className="w-36"
          trigger={<span className="truncate">{testSources.length ? `测试来源 ${testSources.length}` : "测试全部来源"}</span>}
          placeholder="测试来源"
          options={sourceOptions}
          value={testSources}
          onChange={setTestSources}
        />
      )}
      <Button size="sm" variant="ghost" onClick={() => setSettingsOpen(true)} title="测试设置"><Settings2 size={14} /></Button>
      {err && <span className="text-xs text-red-600">{err}</span>}
      {!running && job?.finishedAt && <span className="text-xs text-zinc-500">上次 {formatTime(job.finishedAt)} · 可用 {job.reachable}/{job.done}</span>}
      {settingsOpen && <TestSettingsDialog onClose={() => { setSettingsOpen(false); api.testSettings().then(setSettings).catch(() => {}); }} />}
    </div>
  );
}

// --- settings ---

function TestSettingsDialog({ onClose }: { onClose: () => void }) {
  const [s, setS] = useState<TestSettings | null>(null);
  const [defaults, setDefaults] = useState<IPSource[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const services = useServices();
  useEffect(() => {
    api.testSettings().then(setS).catch((e) => setErr(e.message));
    api.testServices().then((r) => setDefaults(r.defaultIpSources));
  }, []);
  const set = (p: Partial<TestSettings>) => setS((o) => (o ? { ...o, ...p } : o));
  const setSrc = (i: number, p: Partial<IPSource>) => s && set({ ipSources: s.ipSources.map((x, j) => (j === i ? { ...x, ...p } : x)) });

  const save = async () => {
    if (!s) return;
    try { await api.saveTestSettings(s); onClose(); } catch (e) { setErr((e as Error).message); }
  };

  return (
    <Modal open onClose={onClose} title="测试设置" wide footer={<><Button onClick={onClose}>取消</Button><Button variant="primary" onClick={save} disabled={!s}>保存</Button></>}>
      {!s ? <ErrorText error={err} /> : (
        <div className="space-y-6">
          <div className="grid gap-3 sm:grid-cols-4">
            <Field label="并发数"><Input type="number" min={1} max={64} value={s.concurrency} onChange={(e) => set({ concurrency: +e.target.value })} /></Field>
            <Field label="单次请求超时（秒）"><Input type="number" min={1} value={s.timeoutSec} onChange={(e) => set({ timeoutSec: +e.target.value })} /></Field>
            <Field label="定时完整测试（小时）" hint="0 为关闭"><Input type="number" min={0} value={s.scheduleHours} onChange={(e) => set({ scheduleHours: +e.target.value })} /></Field>
            <Field label="测速地址"><Input value={s.latencyUrl} onChange={(e) => set({ latencyUrl: e.target.value })} /></Field>
          </div>

          <div>
            <div className="mb-2 text-xs font-medium text-zinc-600 dark:text-zinc-400">服务可用性检测</div>
            <div className="flex flex-wrap gap-4">
              {services.map((x) => (
                <label key={x.id} className="flex items-center gap-1.5 text-sm">
                  <input type="checkbox" checked={s.services.includes(x.id)} onChange={(e) => set({ services: e.target.checked ? [...s.services, x.id] : s.services.filter((y) => y !== x.id) })} />
                  {x.name}
                </label>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-end justify-between">
              <div>
                <div className="text-xs font-medium text-zinc-600 dark:text-zinc-400">出口 IP / 风险评分源</div>
                <p className="text-xs text-zinc-500">
                  地址不含 <code>{"{ip}"}</code> 时通过节点访问，返回的就是出口 IP 的信息；含 <code>{"{ip}"}</code> 时由服务器直接访问并代入出口 IP，适合需要 API Key 的服务。
                  字段填 JSON 路径，如 <code>location.country_code</code>，<code>{"{ip}"}</code> 段代表出口 IP。评分乘以倍率后应落在 0–100。每个网站的分数含义按网站自身定义换算成安全分（越大越安全）：IPPure、proxycheck.io 给的是风险分，安全分 = 100 − 风险分；Net.Coffee 给的是信任分，直接使用；自定义的源按风险分处理。各来源的安全分分别显示，不做合并。
                </p>
              </div>
              <div className="flex shrink-0 gap-2">
                <Button size="sm" variant="ghost" onClick={() => set({ ipSources: defaults })}>恢复默认</Button>
                <Button size="sm" onClick={() => set({ ipSources: [...s.ipSources, { name: "", enabled: true, url: "" }] })}><Plus size={14} /> 添加</Button>
              </div>
            </div>
            {s.ipSources.map((src, i) => (
              <div key={i} className="space-y-2 rounded-md border border-zinc-200 dark:border-zinc-800 p-3">
                <div className="flex items-center gap-2">
                  <Toggle checked={src.enabled} onChange={(v) => setSrc(i, { enabled: v })} />
                  <Input className="h-8 w-36" placeholder="名称" value={src.name} onChange={(e) => setSrc(i, { name: e.target.value })} />
                  <Input className="h-8 min-w-0 flex-1 font-mono text-xs" placeholder="https://example.com/{ip}?key=..." value={src.url} onChange={(e) => setSrc(i, { url: e.target.value })} />
                  <Button size="sm" variant="danger" onClick={() => set({ ipSources: s.ipSources.filter((_, j) => j !== i) })} aria-label="删除"><Trash2 size={14} /></Button>
                </div>
                <details>
                  <summary className="cursor-pointer text-xs text-zinc-500">字段映射</summary>
                  <div className="mt-2 grid gap-2 sm:grid-cols-4">
                    {(["ip", "country", "city", "isp", "asn", "score"] as const).map((k) => (
                      <Field key={k} label={{ ip: "IP", country: "国家代码", city: "城市", isp: "运营商", asn: "ASN", score: "评分" }[k]}>
                        <Input className="h-8 font-mono text-xs" value={src[k] ?? ""} onChange={(e) => setSrc(i, { [k]: e.target.value })} />
                      </Field>
                    ))}
                    <Field label="评分倍率"><Input className="h-8" type="number" step="any" value={src.scoreScale ?? ""} placeholder="1" onChange={(e) => setSrc(i, { scoreScale: e.target.value ? +e.target.value : undefined })} /></Field>
                    <Field label="标记（名称=路径，逗号分隔）" className="sm:col-span-3">
                      <KVInput value={src.flags} sep="=" onChange={(flags) => setSrc(i, { flags })} />
                    </Field>
                    <Field label="请求头（名称: 值，逗号分隔）" className="sm:col-span-4">
                      <KVInput value={src.headers} sep=":" onChange={(headers) => setSrc(i, { headers })} />
                    </Field>
                  </div>
                </details>
              </div>
            ))}
          </div>
          <ErrorText error={err} />
        </div>
      )}
    </Modal>
  );
}

/** Edits a string map as "k=v, k2=v2"; parsed when the field loses focus so typing is not interrupted. */
function KVInput({ value, sep, onChange }: { value?: Record<string, string>; sep: string; onChange: (v: Record<string, string>) => void }) {
  const show = (v?: Record<string, string>) => Object.entries(v ?? {}).map(([k, x]) => `${k}${sep === ":" ? ": " : sep}${x}`).join(", ");
  const [text, setText] = useState(() => show(value));
  return (
    <Input
      className="h-8 font-mono text-xs"
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={() => {
        const out: Record<string, string> = {};
        for (const part of text.split(",")) {
          const j = part.indexOf(sep);
          if (j > 0 && part.slice(j + 1).trim()) out[part.slice(0, j).trim()] = part.slice(j + 1).trim();
        }
        onChange(out);
        setText(show(out));
      }}
    />
  );
}

/** What one IP data source reported: safety score and labels, or why it failed. */
export function SourceCell({ r, src }: { r?: TestResult; src: string }) {
  const rep = r?.reports?.find((x) => x.source === src);
  if (!rep) return <span className="text-zinc-400" title={r ? "该次测试未查询此来源" : undefined}>—</span>;
  if (rep.error) return <span title={rep.error}><Badge color="red">失败</Badge></span>;
  if (rep.score === undefined && !rep.flags?.length) return <span className="text-xs text-zinc-500" title="此来源没有报告任何标记">无标记</span>;
  return (
    <span className="inline-flex items-center gap-1">
      {rep.score !== undefined && <ScoreBadge v={rep.score} />}
      {rep.flags?.map((f) => <Badge key={f}>{f}</Badge>)}
    </span>
  );
}
