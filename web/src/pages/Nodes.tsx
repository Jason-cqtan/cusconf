import { Fragment, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronDown, ChevronRight, Loader2, Pin, Play, Settings } from "lucide-react";
import { api, formatBytes, formatTime, type Node, type Source, type TestResult } from "../api";
import { LatencyCell, SourceCell, TestDetail, TestToolbar, UnlockCell, reportSources, useServices } from "./Testing";
import { Badge, Card, Empty, ErrorText, Input, PageHeader, Toggle, cx } from "../components/ui";
import { MultiSelect } from "../components/MultiSelect";
import { ColumnSettings, loadPrefs, orderColumns, savePrefs, type ColumnPrefs } from "../components/ColumnSettings";
import { lang, tr } from "../i18n";

// Short names where the browser's built-in ones are long or differ from common usage.
// Short Chinese names where Intl's are long or formal; English uses Intl as is.
const regionOverrides: Record<string, string> = lang === "zh" ? { HK: "香港", MO: "澳门", TW: "台湾", AE: "阿联酋", GB: "英国", US: "美国", KR: "韩国" } : {};
let displayNames: Intl.DisplayNames | null = null;
try { displayNames = new Intl.DisplayNames([lang === "zh" ? "zh-CN" : "en"], { type: "region" }); } catch { /* old browser */ }

/** Localized name for any ISO 3166 region code, e.g. "MD" → "摩尔多瓦" / "Moldova". */
export function regionLabel(code: string): string {
  if (regionOverrides[code]) return regionOverrides[code];
  try { return displayNames?.of(code) ?? code; } catch { return code; }
}

/** Protocol details worth seeing at a glance: cipher/plugin for ss, transport + TLS for the rest. */
export function nodeDetail(n: Node): string {
  const parts: string[] = [];
  if (n.type === "ss" || n.type === "ssr") {
    if (n.cipher) parts.push(n.cipher);
    if (n.plugin) parts.push(n.plugin);
  }
  const tr = n.transport?.type;
  if (tr && tr !== "tcp") parts.push(tr);
  if (n.tls?.realityPubKey) parts.push("reality");
  else if (n.tls?.enabled && !["trojan", "hysteria2", "tuic", "anytls"].includes(n.type)) parts.push("tls");
  if (n.flow) parts.push(n.flow);
  return parts.join(" · ");
}

type ColId = string; // fixed ids plus "score:<source>" for each score source
interface Col {
  id: ColId;
  label: string;
  title?: string;
  sortKey: (n: Node) => string;
  cell: (n: Node) => ReactNode;
}

const PREFS_KEY = "cusconf.nodeTable";
const LEGACY_ORDER_KEY = "cusconf.nodeColumns";
const collator = new Intl.Collator("zh-CN", { numeric: true, sensitivity: "base" });

function initialPrefs(): ColumnPrefs {
  const p = loadPrefs(PREFS_KEY);
  if (p.order.length) return p;
  try {
    const legacy = JSON.parse(localStorage.getItem(LEGACY_ORDER_KEY) ?? "null");
    if (Array.isArray(legacy)) return { ...p, order: legacy };
  } catch { /* ignore */ }
  return p;
}

/**
 * Node table with a sticky header.
 * - Click a header to sort (ascending → descending → off); drag a header onto another to reorder.
 * - The gear at the end of the header opens column settings: order, hide, freeze.
 * - Frozen columns are pinned to the left while scrolling horizontally.
 * Preferences are kept in this browser. The wrapper is the scroll container, so give it a height via className.
 */
export function NodeTable({ nodes, sources, compact, className, groupBySource, results, onShowResult, onTestNode, testing }: {
  nodes: Node[];
  sources?: Source[];
  compact?: boolean;
  className?: string;
  /** Show nodes under collapsible per-source group rows (the 来源 column is then dropped). */
  groupBySource?: boolean;
  /** Test results by node key; adds the test columns when given. */
  results?: Record<string, TestResult>;
  onShowResult?: (r: TestResult) => void;
  /** Adds a per-row "test now" button. */
  onTestNode?: (n: Node) => void;
  testing?: Set<string>; // node keys being tested
}) {
  const services = useServices();
  const srcName = (id?: string) => sources?.find((s) => s.id === id)?.name ?? "";
  const cols = useMemo<Col[]>(() => {
    const all: (Col | false)[] = [
      { id: "name", label: tr("名称"), sortKey: (n) => n.name, cell: (n) => <>{n.name}{n.info && <Badge color="amber" className="ml-2">{tr("机场信息")}</Badge>}{n.selfHosted && <Badge color="green" className="ml-2">{tr("自建")}</Badge>}</> },
      { id: "region", label: tr("地区"), sortKey: (n) => n.region || "￿", cell: (n) => n.region ? <span className="text-xs">{regionLabel(n.region)} {n.region}</span> : <span className="text-zinc-400">—</span> },
      { id: "type", label: tr("类型"), sortKey: (n) => n.type, cell: (n) => <Badge color="indigo">{n.type}</Badge> },
      !compact && { id: "address", label: tr("地址"), sortKey: (n) => `${n.server}:${String(n.port).padStart(5, "0")}`, cell: (n) => <span className="font-mono text-xs text-zinc-500">{n.server}:{n.port}</span> },
      { id: "detail", label: tr("协议参数"), title: tr("Shadowsocks 显示加密方式与插件；其它协议显示传输层（ws / grpc 等）和 TLS / REALITY"), sortKey: (n) => nodeDetail(n) || "￿", cell: (n) => <span className="font-mono text-xs text-zinc-500">{nodeDetail(n) || "—"}</span> },
      !!sources && !groupBySource && { id: "source", label: tr("来源"), sortKey: (n) => srcName(n.source), cell: (n) => <span className="text-xs text-zinc-500">{srcName(n.source)}</span> },
    ];
    const base = all.filter(Boolean) as Col[];
    if (!results) return base;
    // Test columns come right after the region by default (saved column orders still win).
    const at = base.findIndex((c) => c.id === "region") + 1;
    return [...base.slice(0, at), ...testColumns(results, services, onShowResult), ...base.slice(at)];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [compact, sources, groupBySource, results, services]);

  const [prefs, setPrefsState] = useState<ColumnPrefs>(initialPrefs);
  const setPrefs = (p: ColumnPrefs) => { setPrefsState(p); savePrefs(PREFS_KEY, p); };
  const [settingsOpen, setSettingsOpen] = useState(false);

  const ordered = useMemo(() => orderColumns(cols, prefs.order), [cols, prefs.order]);
  // Displayed: visible columns, frozen ones first. Never hide everything.
  const shownCols = useMemo(() => {
    let vis = ordered.filter((c) => !prefs.hidden.includes(c.id));
    if (!vis.length) vis = ordered.slice(0, 1);
    return [...vis.filter((c) => prefs.frozen.includes(c.id)), ...vis.filter((c) => !prefs.frozen.includes(c.id))];
  }, [ordered, prefs.hidden, prefs.frozen]);
  const frozenCount = shownCols.filter((c) => prefs.frozen.includes(c.id)).length;

  // Left offsets of frozen columns, measured from the rendered header cells.
  const thRefs = useRef<(HTMLTableCellElement | null)[]>([]);
  const tableRef = useRef<HTMLTableElement>(null);
  const theadRef = useRef<HTMLTableSectionElement>(null);
  const [lefts, setLefts] = useState<number[]>([]);
  const [headH, setHeadH] = useState(0); // group rows stick right below the header
  useLayoutEffect(() => {
    const measure = () => {
      const out: number[] = [];
      let x = 0;
      for (let i = 0; i < frozenCount; i++) {
        out.push(x);
        x += thRefs.current[i]?.offsetWidth ?? 0;
      }
      setLefts((prev) => (prev.length === out.length && prev.every((v, i) => v === out[i]) ? prev : out));
      setHeadH(theadRef.current?.offsetHeight ?? 0);
    };
    measure();
    const ro = new ResizeObserver(measure);
    if (tableRef.current) ro.observe(tableRef.current);
    return () => ro.disconnect();
  }, [frozenCount, shownCols]);

  const [sort, setSort] = useState<{ id: ColId; dir: 1 | -1 } | null>(null);
  const rows = useMemo(() => {
    if (!sort) return nodes;
    const col = cols.find((c) => c.id === sort.id);
    if (!col) return nodes;
    return [...nodes].sort((a, b) => sort.dir * collator.compare(col.sortKey(a), col.sortKey(b)));
  }, [nodes, sort, cols]);

  // Per-source groups, in the sources' order; nodes without a known source go last.
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const groups = useMemo(() => {
    if (!groupBySource) return null;
    const by = new Map<string, Node[]>();
    for (const n of rows) {
      const k = n.source ?? "";
      if (!by.has(k)) by.set(k, []);
      by.get(k)!.push(n);
    }
    const known = (sources ?? []).filter((s) => by.has(s.id)).map((s) => ({ id: s.id, src: s as Source | undefined, rows: by.get(s.id)! }));
    const rest = [...by.keys()].filter((k) => !known.some((g) => g.id === k)).map((k) => ({ id: k, src: undefined, rows: by.get(k)! }));
    return [...known, ...rest];
  }, [groupBySource, rows, sources]);
  const toggleGroup = (id: string) => setCollapsed((c) => {
    const n = new Set(c);
    if (n.has(id)) n.delete(id); else n.add(id);
    return n;
  });

  const [dragging, setDragging] = useState<ColId | null>(null);
  const [over, setOver] = useState<ColId | null>(null);
  const drop = (target: ColId) => {
    if (!dragging || dragging === target) return;
    const ids = ordered.map((c) => c.id).filter((id) => id !== dragging);
    const before = ordered.findIndex((c) => c.id === dragging) < ordered.findIndex((c) => c.id === target);
    ids.splice(ids.indexOf(target) + (before ? 1 : 0), 0, dragging);
    setPrefs({ ...prefs, order: [...ids, ...prefs.order.filter((id) => !(ids as string[]).includes(id))] });
  };
  const clickSort = (id: ColId) =>
    setSort((s) => (!s || s.id !== id ? { id, dir: 1 } : s.dir === 1 ? { id, dir: -1 } : null));

  const pin = (i: number) => (i < frozenCount ? { left: lefts[i] ?? 0 } : undefined);
  const pinCls = (i: number, head: boolean) => cx(
    i < frozenCount && (head ? "sticky z-20" : "sticky z-[5]"),
    i === frozenCount - 1 && "shadow-[inset_-1px_0_0_var(--color-zinc-200)] dark:shadow-[inset_-1px_0_0_var(--color-zinc-700)]",
  );
  const cellBg = "bg-white dark:bg-zinc-900 group-hover:bg-zinc-50 dark:group-hover:bg-zinc-800";

  function renderRow(n: Node) {
    return (
      <tr key={`${n.source ?? ""}/${n.key}/${n.name}`} className="group">
        {shownCols.map((c, i) => (
          <td key={c.id} style={pin(i)} className={cx("border-b border-zinc-100 dark:border-zinc-800/60 px-3 py-1.5 whitespace-nowrap", cellBg, pinCls(i, false))}>
            {c.cell(n)}
          </td>
        ))}
        <td className={cx("sticky right-0 border-b border-zinc-100 dark:border-zinc-800/60 px-1 text-right", cellBg)}>
          {onTestNode && !n.info && (
            testing?.has(n.key)
              ? <Loader2 size={14} className="mx-1.5 inline animate-spin text-indigo-500" aria-label={tr("测试中")} />
              : (
                <button
                  type="button"
                  title={tr("立即测试此节点（延迟、出口 IP、服务）")}
                  aria-label={tr("测试此节点")}
                  onClick={() => onTestNode(n)}
                  className="rounded p-1.5 text-zinc-400 opacity-0 transition-opacity group-hover:opacity-100 focus:opacity-100 hover:bg-zinc-100 hover:text-indigo-600 dark:hover:bg-zinc-700"
                >
                  <Play size={13} />
                </button>
              )
          )}
        </td>
      </tr>
    );
  }

  if (nodes.length === 0) return <Empty>{tr("没有节点")}</Empty>;
  return (
    <div className={cx("overflow-auto", className)}>
      <table ref={tableRef} className="w-full min-w-max border-separate border-spacing-0 text-sm">
        <thead ref={theadRef} className="text-left text-xs text-zinc-500">
          <tr>
            {shownCols.map((c, i) => (
              <th
                key={c.id}
                ref={(el) => { thRefs.current[i] = el; }}
                style={pin(i)}
                title={c.title ?? tr("点击排序，拖动调整列顺序")}
                draggable
                onDragStart={(e) => { setDragging(c.id); e.dataTransfer.effectAllowed = "move"; }}
                onDragEnd={() => { setDragging(null); setOver(null); }}
                onDragOver={(e) => { e.preventDefault(); setOver(c.id); }}
                onDragLeave={() => setOver((o) => (o === c.id ? null : o))}
                onDrop={(e) => { e.preventDefault(); drop(c.id); setOver(null); }}
                onClick={() => clickSort(c.id)}
                className={cx(
                  "sticky top-0 z-10 cursor-pointer border-b border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-3 py-2 font-medium whitespace-nowrap select-none hover:text-zinc-800 dark:hover:text-zinc-200",
                  pinCls(i, true),
                  dragging === c.id && "opacity-40",
                  over === c.id && dragging && dragging !== c.id && "bg-indigo-50 dark:bg-indigo-950",
                )}
              >
                <span className="inline-flex items-center gap-1">
                  {c.label}
                  {prefs.frozen.includes(c.id) && <Pin size={11} className="text-indigo-500" />}
                  {sort?.id === c.id ? (sort.dir === 1 ? <ArrowUp size={12} /> : <ArrowDown size={12} />) : <ArrowUpDown size={12} className="opacity-30" />}
                </span>
              </th>
            ))}
            <th className="sticky top-0 right-0 z-20 w-10 border-b border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-1 text-right">
              <button
                type="button"
                title={tr("列设置")}
                aria-label={tr("列设置")}
                onClick={() => setSettingsOpen(true)}
                className="rounded p-1.5 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
              >
                <Settings size={15} />
              </button>
            </th>
          </tr>
        </thead>
        <tbody>
          {groups
            ? groups.map((g) => {
                const open = !collapsed.has(g.id);
                const info = g.src?.info;
                return (
                  <Fragment key={`g/${g.id}`}>
                    <tr className="cursor-pointer select-none" onClick={() => toggleGroup(g.id)}>
                      <td
                        colSpan={shownCols.length + 1}
                        style={{ top: headH }}
                        className="sticky z-[6] border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 px-3 py-2 hover:bg-zinc-100 dark:hover:bg-zinc-800/80"
                      >
                        <span className="sticky left-3 inline-flex items-center gap-2 text-sm">
                          {open ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
                          <span className="font-medium">{g.src?.name ?? (g.id || tr("未知来源"))}</span>
                          {g.src?.selfHosted && <Badge color="green">{tr("自建")}</Badge>}
                          <Badge>{g.rows.length} {tr("个节点")}</Badge>
                          {g.src && <Badge color={g.src.kind === "url" ? "indigo" : "zinc"}>{g.src.kind === "url" ? tr("订阅") : tr("手动")}</Badge>}
                          {info && info.total > 0 && <span className="text-xs text-zinc-500">{tr("流量")} {formatBytes(info.upload + info.download)} / {formatBytes(info.total)}</span>}
                          {info && info.expire > 0 && <span className="text-xs text-zinc-500">{tr("到期")} {new Date(info.expire * 1000).toLocaleDateString()}</span>}
                        </span>
                      </td>
                    </tr>
                    {open && g.rows.map(renderRow)}
                  </Fragment>
                );
              })
            : rows.map(renderRow)}
        </tbody>
      </table>
      <ColumnSettings
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        columns={ordered.map((c) => ({ id: c.id, label: c.label }))}
        defaultOrder={cols.map((c) => c.id)}
        prefs={prefs}
        onSave={setPrefs}
      />
    </div>
  );
}

// Test result columns; a click opens the detail.
function testColumns(results: Record<string, TestResult>, services: { id: string; name: string }[], show?: (r: TestResult) => void): Col[] {
  // Info entries often reuse a real node's endpoint (same key); never show that node's result on them.
  const R = (n: Node) => (n.info ? undefined : results[n.key]);
  const pad = (v: number) => String(Math.round(v * 10)).padStart(8, "0");
  const wrap = (n: Node, el: ReactNode) => {
    const r = R(n);
    return r && show ? <button type="button" className="text-left" onClick={() => show(r)}>{el}</button> : el;
  };
  return [
    { id: "latency", label: tr("延迟"), sortKey: (n) => { const r = R(n); return !r ? "z" : r.latency > 0 ? pad(r.latency) : "y"; }, cell: (n) => wrap(n, <LatencyCell r={R(n)} />) },
    { id: "exit", label: tr("出口"), sortKey: (n) => R(n)?.exit?.country ?? "\uffff", cell: (n) => {
      const e = R(n)?.exit;
      return wrap(n, e ? <span className="text-xs" title={[e.ip, e.isp].filter(Boolean).join(" · ")}>{e.country ? `${regionLabel(e.country)} ${e.country}` : "—"} <span className="font-mono text-zinc-500">{e.ip}</span></span> : <span className="text-zinc-400">—</span>);
    } },
    // One safety-score column per source: each site rates differently, so they are not merged.
    // One column per IP data source: its safety score (if it gives one), its labels,
    // or why it failed. Sources rate differently, so nothing is merged.
    ...reportSources(results).map((src): Col => ({
      id: `src:${src}`, label: src, title: tr("{0}：安全分（0–100，越大越安全）与标记", src),
      sortKey: (n) => {
        const rep = R(n)?.reports?.find((x) => x.source === src);
        if (!rep || rep.error) return "z";
        return rep.score === undefined ? "y" + (rep.flags ?? []).join(",") : pad(100 - rep.score);
      },
      cell: (n) => wrap(n, <SourceCell r={R(n)} src={src} />),
    })),
    { id: "unlock", label: tr("解锁"), title: tr("可用的流媒体 / AI 服务；½ 表示部分可用"), sortKey: (n) => String(100 - Object.values(R(n)?.services ?? {}).filter((s) => s.status === "yes").length).padStart(3, "0"), cell: (n) => wrap(n, <UnlockCell r={R(n)} services={services} />) },
    { id: "tested", label: tr("测试时间"), sortKey: (n) => String(1e12 - (R(n)?.testedAt ?? 0)), cell: (n) => <span className="text-xs text-zinc-500">{formatTime(R(n)?.testedAt)}</span> },
  ];
}

const UNKNOWN = "?";
const GROUP_KEY = "cusconf.nodeGroupBySource";

function countBy(nodes: Node[], key: (n: Node) => string) {
  const m = new Map<string, number>();
  for (const n of nodes) m.set(key(n), (m.get(key(n)) ?? 0) + 1);
  return m;
}

export default function NodesPage() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [q, setQ] = useState("");
  const [types, setTypes] = useState<string[]>([]);
  const [regions, setRegions] = useState<string[]>([]);
  const [srcs, setSrcs] = useState<string[]>([]);
  const [grouped, setGroupedState] = useState(() => { try { return localStorage.getItem(GROUP_KEY) !== "0"; } catch { return true; } });
  const setGrouped = (v: boolean) => { setGroupedState(v); try { localStorage.setItem(GROUP_KEY, v ? "1" : "0"); } catch { /* ignore */ } };
  const [err, setErr] = useState<string | null>(null);
  const [results, setResults] = useState<Record<string, TestResult>>({});
  const [detail, setDetail] = useState<TestResult | null>(null);
  const loadResults = useCallback(() => api.testResults().then(setResults).catch(() => {}), []);
  const [testing, setTesting] = useState<Set<string>>(new Set());
  const testNode = async (n: Node) => {
    setTesting((s) => new Set(s).add(n.key));
    try {
      const r = await api.testNode(n.key);
      setResults((prev) => ({ ...prev, [r.key]: r }));
    } catch (e) {
      setErr(tr("{0}：{1}", n.name, (e as Error).message));
    } finally {
      setTesting((s) => { const x = new Set(s); x.delete(n.key); return x; });
    }
  };

  useEffect(() => {
    Promise.all([api.nodes(), api.sources()]).then(([n, s]) => { setNodes(n); setSources(s); }).catch((e) => setErr(e.message));
    loadResults();
  }, [loadResults]);

  const typeOptions = useMemo(() => {
    const c = countBy(nodes, (n) => n.type);
    return [...c.keys()].sort().map((t) => ({ value: t, label: t, count: c.get(t) }));
  }, [nodes]);
  const regionOptions = useMemo(() => {
    const c = countBy(nodes, (n) => n.region || UNKNOWN);
    const opts = [...c.keys()].filter((r) => r !== UNKNOWN)
      .sort((a, b) => (c.get(b)! - c.get(a)!) || a.localeCompare(b))
      .map((r) => ({ value: r, label: `${regionLabel(r)} ${r}`, count: c.get(r) }));
    if (c.has(UNKNOWN)) opts.push({ value: UNKNOWN, label: tr("未识别"), count: c.get(UNKNOWN) });
    return opts;
  }, [nodes]);
  const sourceOptions = useMemo(() => {
    const c = countBy(nodes, (n) => n.source ?? "");
    return sources.filter((s) => c.has(s.id)).map((s) => ({ value: s.id, label: s.name, count: c.get(s.id) }));
  }, [nodes, sources]);

  const intercepted = Object.values(results).filter((r) => r.warning).length;
  const k = q.trim().toLowerCase();
  const shown = nodes.filter((n) =>
    (!k || n.name.toLowerCase().includes(k) || n.server.toLowerCase().includes(k)) &&
    (!types.length || types.includes(n.type)) &&
    (!regions.length || regions.includes(n.region || UNKNOWN)) &&
    (!srcs.length || srcs.includes(n.source ?? "")),
  );

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="shrink-0">
        <PageHeader
          title={tr("节点")}
          desc={tr("所有启用订阅源的节点。地区由节点名自动识别；测试结果可在策略组中按延迟、出口地区、风险和解锁情况筛选。")}
          actions={<TestToolbar nodes={shown} sources={sources} onFinished={loadResults} />}
        />
        <ErrorText error={err} />
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <Input className="w-full sm:w-64" placeholder={tr("搜索名称或地址")} value={q} onChange={(e) => setQ(e.target.value)} />
          <MultiSelect className="w-full sm:w-40" placeholder={tr("全部类型")} options={typeOptions} value={types} onChange={setTypes} />
          <MultiSelect className="w-full sm:w-44" placeholder={tr("全部地区")} options={regionOptions} value={regions} onChange={setRegions} />
          {sourceOptions.length > 1 && <MultiSelect className="w-full sm:w-44" placeholder={tr("全部来源")} options={sourceOptions} value={srcs} onChange={setSrcs} />}
          <span className="text-sm text-zinc-500">{shown.length} / {nodes.length}</span>
          {sources.length > 1 && <span className="ml-auto"><Toggle checked={grouped} onChange={setGrouped} label={tr("按来源分组")} /></span>}
        </div>
      </div>
      {intercepted > 0 && (
        <p className="mb-3 shrink-0 rounded-md bg-amber-50 dark:bg-amber-950/40 px-3 py-2 text-xs text-amber-900 dark:text-amber-200">
          ⚠ {intercepted} {tr("个节点的测试连接被本机代理软件接管（节点服务器被解析为 Fake IP 198.18.x.x），会按该软件的规则转发、可能被套进其它代理，结果不可信。 在 Surge 中添加规则")} <code>PROCESS-NAME,mihomo,DIRECT</code>{tr("（放在规则最前面；节点测试由 cusconf 启动的 mihomo 进程连接节点），或把 cusconf 部署在不经过本机代理的环境中，然后重新测试。")}
        </p>
      )}
      <Card className="min-h-0 flex-1 overflow-hidden">
        <NodeTable nodes={shown} sources={sources} className="h-full" groupBySource={grouped && sources.length > 1} results={results} onShowResult={setDetail} onTestNode={testNode} testing={testing} />
      </Card>
      <TestDetail r={detail} onClose={() => setDetail(null)} />
    </div>
  );
}
