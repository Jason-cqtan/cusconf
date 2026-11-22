import { useCallback, useEffect, useRef, useState } from "react";
import { Activity, Gauge, Plus, Settings2, Square, Trash2 } from "lucide-react";
import { api, formatTime, type IPSource, type Node, type ServiceStatus, type Source, type TestJob, type SyncStatus, type TestResult, type TestSettings } from "../api";
import { MultiSelect } from "../components/MultiSelect";
import { Badge, Button, CopyButton, ErrorText, Field, Input, Modal, Toggle, cx } from "../components/ui";
import { tr, trMsg } from "../i18n";

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
  const warn = r.warning && <span className="ml-1 text-amber-500" title={trMsg(r.warning)}>⚠</span>;
  if (r.latency <= 0) return <span className="text-xs text-red-600" title={[trMsg(r.error), trMsg(r.warning)].filter(Boolean).join("\n")}>{tr("超时")}{warn}</span>;
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
  return <span className="inline-flex gap-1">{f.map((x) => <Badge key={x}>{trMsg(x)}</Badge>)}</span>;
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
  if (!ids.length) return <span className="text-xs text-zinc-500">{tr("无")}</span>;
  return (
    <span className="inline-flex flex-wrap gap-1">
      {ids.map((id) => {
        const v = s[id];
        const st = statusStyle[v.status];
        return (
          <Badge key={id} color={st.color}>
            <span title={[v.region && tr("地区 {0}", v.region), trMsg(v.note)].filter(Boolean).join(" · ") || undefined}>
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
    <Modal open={!!r} onClose={onClose} title={r ? tr("测试结果 · {0}", r.name) : ""} wide>
      {r && (
        <div className="space-y-5 text-sm">
          <div className="flex flex-wrap gap-x-6 gap-y-1">
            <span>{tr("延迟")} <LatencyCell r={r} /></span>
            <span className="text-zinc-500">{tr("测试于")} {formatTime(r.testedAt)}{r.synced && <> · {tr("来自同步")}</>}</span>
            {r.error && <span className="text-red-600">{trMsg(r.error)}</span>}
          </div>
          {r.warning && <p className="rounded-md bg-amber-50 dark:bg-amber-950/40 px-3 py-2 text-xs text-amber-900 dark:text-amber-200">⚠ {trMsg(r.warning)}</p>}
          {r.exit && (
            <div>
              <h4 className="mb-1 font-medium">{tr("出口 IP")}</h4>
              <div className="grid gap-x-6 gap-y-1 sm:grid-cols-2">
                <span>{tr("IP：")}<code>{r.exit.ip ?? "—"}</code></span>
                <span>{tr("地区：")}{r.exit.country ?? "—"} {r.exit.city}</span>
                <span>{tr("运营商：")}{r.exit.isp ?? "—"}</span>
                <span>{tr("ASN：")}{r.exit.asn ?? "—"}</span>
                <span className="sm:col-span-2 flex flex-wrap items-center gap-x-3 gap-y-1">
                  {tr("安全分：")}
                  {Object.entries(r.exit.scores ?? {}).map(([k, v]) => <span key={k} className="inline-flex items-center gap-1 text-xs">{k} <ScoreBadge v={v} /></span>)}
                  {!Object.keys(r.exit.scores ?? {}).length && <span className="text-zinc-400">—</span>}
                </span>
                <span className="sm:col-span-2">{tr("标记：")}<FlagsCell r={r} /></span>
              </div>
            </div>
          )}
          {!!r.reports?.length && (
            <div>
              <h4 className="mb-1 font-medium">{tr("各数据源")}</h4>
              <table className="w-full text-xs">
                <thead className="text-left text-zinc-500"><tr><th className="py-1">{tr("来源")}</th><th>IP</th><th>{tr("地区")}</th><th>{tr("安全分")}</th><th>{tr("标记")}</th><th>{tr("运营商")}</th></tr></thead>
                <tbody>
                  {r.reports.map((x) => (
                    <tr key={x.source} className="border-t border-zinc-100 dark:border-zinc-800">
                      <td className="py-1 pr-2">{x.source}</td>
                      {x.error ? <td colSpan={5} className="text-red-600">{trMsg(x.error)}</td> : (
                        <>
                          <td className="pr-2 font-mono">{x.ip}</td>
                          <td className="pr-2">{x.country} {x.city}</td>
                          <td className="pr-2"><ScoreBadge v={x.score} /></td>
                          <td className="pr-2">{x.flags?.map(trMsg).join(tr("、")) || "—"}</td>
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
              <h4 className="mb-1 font-medium">{tr("服务可用性")}</h4>
              <div className="grid gap-1 sm:grid-cols-2">
                {services.filter((s) => r.services![s.id]).map((s) => {
                  const v = r.services![s.id];
                  return (
                    <div key={s.id} className="flex items-center gap-2">
                      <span className="w-32">{s.name}</span>
                      <Badge color={statusStyle[v.status].color}>{{ yes: tr("可用"), partial: tr("部分"), no: tr("不可用"), unknown: tr("未知"), error: tr("失败") }[v.status]}</Badge>
                      {v.region && <span className="text-xs">{v.region}</span>}
                      {v.note && <span className="text-xs text-zinc-500">{trMsg(v.note)}</span>}
                    </div>
                  );
                })}
              </div>
              <p className="mt-2 text-xs text-zinc-500">{tr("服务检测基于公开页面的特征判断，网站改版时可能不准。标「按出口地区推断」的项目无法直接探测，按出口所在地区判断。")}</p>
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
            {job!.done} / {job!.total}{job!.scheduled ? tr("（定时）") : ""}
          </div>
          <Button size="sm" onClick={() => api.stopTest().then(setJob)}><Square size={12} /> {tr("停止")}</Button>
        </>
      ) : (
        <>
          <Button size="sm" onClick={() => start(false)} disabled={!keys.length} title={tr("只测延迟")}><Gauge size={14} /> {tr("测延迟")}</Button>
          <Button size="sm" variant="primary" onClick={() => start(true)} disabled={!keys.length} title={tr("延迟、出口 IP 与风险、服务可用性")}><Activity size={14} /> {tr("完整测试")} {keys.length}</Button>
        </>
      )}
      {sources.length > 1 && (
        <MultiSelect
          className="w-36"
          trigger={<span className="truncate">{testSources.length ? tr("测试来源 {0}", testSources.length) : tr("测试全部来源")}</span>}
          placeholder={tr("测试来源")}
          options={sourceOptions}
          value={testSources}
          onChange={setTestSources}
        />
      )}
      <Button size="sm" variant="ghost" onClick={() => setSettingsOpen(true)} title={tr("测试设置")}><Settings2 size={14} /></Button>
      {err && <span className="text-xs text-red-600">{err}</span>}
      {!running && job?.error && <span className="text-xs text-red-600" title={trMsg(job.error)}>{tr("测试未能运行：")}{trMsg(job.error)}</span>}
      {!running && !job?.error && job?.finishedAt && <span className="text-xs text-zinc-500">{tr("上次")} {formatTime(job.finishedAt)} {tr("· 可用")} {job.reachable}/{job.done}</span>}
      {job?.sync && <span className={cx("text-xs", job.sync.error ? "text-red-600" : "text-zinc-500")} title={formatTime(job.sync.at)}>{job.sync.error ? tr("推送失败：") + trMsg(job.sync.error) : tr("已推送 {0} 条", job.sync.count)}</span>}
      {settingsOpen && <TestSettingsDialog onClose={() => { setSettingsOpen(false); api.testSettings().then(setSettings).catch(() => {}); }} />}
    </div>
  );
}

// --- settings ---

/** Pushing results to another cusconf, and this instance's key for receiving them. */
function SyncSettings({ s, set, save }: { s: TestSettings; set: (p: Partial<TestSettings>) => void; save: () => Promise<unknown> }) {
  const sync = s.sync ?? {};
  const setSync = (p: Partial<NonNullable<TestSettings["sync"]>>) => set({ sync: { ...sync, ...p } });
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => { api.syncKey().then((r) => setKey(r.key)).catch(() => {}); }, []);

  const push = async () => {
    setBusy(true);
    setErr(null);
    try { await save(); setStatus(await api.syncPush()); } catch (e) { setErr((e as Error).message); }
    setBusy(false);
  };

  return (
    <div className="space-y-3 rounded-md border border-zinc-200 dark:border-zinc-800 p-3">
      <div>
        <div className="text-xs font-medium text-zinc-600 dark:text-zinc-400">{tr("同步测试结果")}</div>
        <p className="mt-1 text-xs text-zinc-500">
          {tr("有些节点只接受中国大陆网络的连接，部署在境外的 cusconf 测不通它们。可以在能连上的地方（如家里的电脑）运行 cusconf 测试，再把结果推送到服务器；服务器本地测试失败时不会覆盖同步来的可用结果。")}
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
        <Field label={tr("推送到（对方 cusconf 地址）")}>
          <Input className="font-mono text-xs" placeholder="https://sub.example.com" value={sync.url ?? ""} onChange={(e) => setSync({ url: e.target.value.trim() || undefined })} />
        </Field>
        <Field label={tr("对方的同步密钥")}>
          <Input className="font-mono text-xs" type="password" autoComplete="off" value={sync.key ?? ""} onChange={(e) => setSync({ key: e.target.value.trim() || undefined })} />
        </Field>
        <Button disabled={busy || !sync.url || !sync.key} onClick={push}>{busy ? tr("同步中…") : tr("保存并立即推送")}</Button>
      </div>
      <div className="flex flex-wrap gap-x-6 gap-y-2">
        <Toggle checked={!!sync.auto} onChange={(v) => setSync({ auto: v || undefined })} label={tr("每次测试完成后自动推送")} />
        <Toggle checked={!sync.noNodes} onChange={(v) => setSync({ noNodes: !v || undefined })} label={tr("同时推送订阅源的节点")} />
      </div>
      {!sync.noNodes && <p className="text-xs text-zinc-500">{tr("对方拉不到的订阅（如只允许国内 IP 拉取）也能有节点：按订阅地址匹配对方的订阅源，没有就自动创建；对方自己拉取成功且更新时，以对方为准。")}</p>}
      <ErrorText error={err} />
      {status && (status.error
        ? <p className="text-xs text-red-600">{tr("推送失败：")}{trMsg(status.error)}</p>
        : <p className="text-xs text-emerald-600">{tr("已推送，对方更新了 {0} 条结果、{1} 个订阅源的节点（较旧的数据不会覆盖对方更新的数据）", status.count, status.sources ?? 0)}</p>)}
      <div className="border-t border-zinc-100 dark:border-zinc-800 pt-3">
        <Field label={tr("本机的同步密钥")} hint={tr("在推送方填写本机地址和这个密钥。重新生成后，使用旧密钥的推送会失败。")}>
          <div className="flex gap-2">
            <Input readOnly className="font-mono text-xs" type="password" value={key} onFocus={(e) => { e.currentTarget.type = "text"; e.currentTarget.select(); }} onBlur={(e) => { e.currentTarget.type = "password"; }} />
            <CopyButton text={key} />
            <Button size="sm" variant="ghost" onClick={async () => { if (confirm(tr("重新生成同步密钥？使用旧密钥的推送将失败。"))) setKey((await api.syncKey(true)).key); }}>{tr("重新生成")}</Button>
          </div>
        </Field>
      </div>
    </div>
  );
}

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
    <Modal open onClose={onClose} title={tr("测试设置")} wide footer={<><Button onClick={onClose}>{tr("取消")}</Button><Button variant="primary" onClick={save} disabled={!s}>{tr("保存")}</Button></>}>
      {!s ? <ErrorText error={err} /> : (
        <div className="space-y-6">
          <div className="grid gap-3 sm:grid-cols-4">
            <Field label={tr("并发数")}><Input type="number" min={1} max={64} value={s.concurrency} onChange={(e) => set({ concurrency: +e.target.value })} /></Field>
            <Field label={tr("单次请求超时（秒）")}><Input type="number" min={1} value={s.timeoutSec} onChange={(e) => set({ timeoutSec: +e.target.value })} /></Field>
            <Field label={tr("定时完整测试（小时）")} hint={tr("0 为关闭")}><Input type="number" min={0} value={s.scheduleHours} onChange={(e) => set({ scheduleHours: +e.target.value })} /></Field>
            <Field label={tr("测速地址")}><Input value={s.latencyUrl} onChange={(e) => set({ latencyUrl: e.target.value })} /></Field>
          </div>

          <SyncSettings s={s} set={set} save={() => api.saveTestSettings(s)} />

          <div>
            <div className="mb-2 text-xs font-medium text-zinc-600 dark:text-zinc-400">{tr("服务可用性检测")}</div>
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
                <div className="text-xs font-medium text-zinc-600 dark:text-zinc-400">{tr("出口 IP / 风险评分源")}</div>
                <p className="text-xs text-zinc-500">
                  {tr("所有来源都通过被测节点访问，不依赖本机的 DNS 和代理设置。地址不含")} <code>{"{ip}"}</code> {tr("时，返回的就是出口 IP 的信息；含")} <code>{"{ip}"}</code> {tr("时代入前面来源查到的出口 IP，适合按 IP 查询或需要 API Key 的服务。 字段填 JSON 路径，如")} <code>location.country_code</code>{tr("，")}<code>{"{ip}"}</code> {tr("段代表出口 IP。评分乘以倍率后应落在 0–100。每个网站的分数含义按网站自身定义换算成安全分（越大越安全）：IPPure、proxycheck.io 给的是风险分，安全分 = 100 − 风险分；Net.Coffee 给的是信任分，直接使用；自定义的源按风险分处理。各来源的安全分分别显示，不做合并。")}
                </p>
              </div>
              <div className="flex shrink-0 gap-2">
                <Button size="sm" variant="ghost" onClick={() => set({ ipSources: defaults })}>{tr("恢复默认")}</Button>
                <Button size="sm" onClick={() => set({ ipSources: [...s.ipSources, { name: "", enabled: true, url: "" }] })}><Plus size={14} /> {tr("添加")}</Button>
              </div>
            </div>
            {s.ipSources.map((src, i) => (
              <div key={i} className="space-y-2 rounded-md border border-zinc-200 dark:border-zinc-800 p-3">
                <div className="flex items-center gap-2">
                  <Toggle checked={src.enabled} onChange={(v) => setSrc(i, { enabled: v })} />
                  <Input className="h-8 w-36" placeholder={tr("名称")} value={src.name} onChange={(e) => setSrc(i, { name: e.target.value })} />
                  <Input className="h-8 min-w-0 flex-1 font-mono text-xs" placeholder="https://example.com/{ip}?key=..." value={src.url} onChange={(e) => setSrc(i, { url: e.target.value })} />
                  <Button size="sm" variant="danger" onClick={() => set({ ipSources: s.ipSources.filter((_, j) => j !== i) })} aria-label={tr("删除")}><Trash2 size={14} /></Button>
                </div>
                <details>
                  <summary className="cursor-pointer text-xs text-zinc-500">{tr("字段映射")}</summary>
                  <div className="mt-2 grid gap-2 sm:grid-cols-4">
                    {(["ip", "country", "city", "isp", "asn", "score"] as const).map((k) => (
                      <Field key={k} label={{ ip: "IP", country: tr("国家代码"), city: tr("城市"), isp: tr("运营商"), asn: "ASN", score: tr("评分") }[k]}>
                        <Input className="h-8 font-mono text-xs" value={src[k] ?? ""} onChange={(e) => setSrc(i, { [k]: e.target.value })} />
                      </Field>
                    ))}
                    <Field label={tr("评分倍率")}><Input className="h-8" type="number" step="any" value={src.scoreScale ?? ""} placeholder="1" onChange={(e) => setSrc(i, { scoreScale: e.target.value ? +e.target.value : undefined })} /></Field>
                    <Field label={tr("标记（名称=路径，逗号分隔）")} className="sm:col-span-3">
                      <KVInput value={src.flags} sep="=" onChange={(flags) => setSrc(i, { flags })} />
                    </Field>
                    <Field label={tr("请求头（名称: 值，逗号分隔）")} className="sm:col-span-4">
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
  if (!rep) return <span className="text-zinc-400" title={r ? tr("该次测试未查询此来源") : undefined}>—</span>;
  if (rep.error) return <span title={trMsg(rep.error)}><Badge color="red">{tr("失败")}</Badge></span>;
  if (rep.score === undefined && !rep.flags?.length) return <span className="text-xs text-zinc-500" title={tr("此来源没有报告任何标记")}>{tr("无标记")}</span>;
  return (
    <span className="inline-flex items-center gap-1">
      {rep.score !== undefined && <ScoreBadge v={rep.score} />}
      {rep.flags?.map((f) => <Badge key={f}>{trMsg(f)}</Badge>)}
    </span>
  );
}
