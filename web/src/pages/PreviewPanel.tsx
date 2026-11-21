import { useEffect, useState } from "react";
import { AlertTriangle } from "lucide-react";
import { api, type Capabilities, type Client, type Profile, type RenderResult, type Target } from "../api";
import { Button, CopyButton, ErrorText, Field, Input, Select } from "../components/ui";

export function TargetPicker({ value, onChange, caps }: { value: Target; onChange: (t: Target) => void; caps: Capabilities | null }) {
  const platforms = caps?.clients[value.client]?.platforms ?? {};
  const latest = platforms[value.platform]?.latest ?? "";
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <Field label="客户端">
        <Select value={value.client} onChange={(e) => onChange({ client: e.target.value as Client, platform: "ios", version: "" })}>
          <option value="surge">Surge</option>
          <option value="stash">Stash</option>
        </Select>
      </Field>
      <Field label="平台">
        <Select value={value.platform} onChange={(e) => onChange({ ...value, platform: e.target.value, version: "" })}>
          {Object.entries(platforms).map(([k, p]) => <option key={k} value={k}>{p.label}</option>)}
        </Select>
      </Field>
      <Field label="版本" hint={`留空 = 最新（${latest}），低版本会自动降级或剔除不支持的节点和参数`}>
        <Input value={value.version} placeholder={latest} onChange={(e) => onChange({ ...value, version: e.target.value.trim() })} />
      </Field>
    </div>
  );
}

export default function PreviewPanel({ profile, caps }: { profile: Profile; caps: Capabilities | null }) {
  const [target, setTarget] = useState<Target>({ client: "surge", platform: "ios", version: "" });
  const [res, setRes] = useState<RenderResult | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const run = async () => {
    setBusy(true);
    setErr(null);
    try { setRes(await api.preview(profile, target)); } catch (e) { setErr((e as Error).message); setRes(null); }
    setBusy(false);
  };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { run(); }, [target]);

  const warnings = res?.warnings ?? [];
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-3">
        <div className="min-w-0 flex-1"><TargetPicker value={target} onChange={setTarget} caps={caps} /></div>
        <Button onClick={run} disabled={busy}>{busy ? "生成中…" : "重新生成"}</Button>
      </div>
      <ErrorText error={err} />
      {warnings.length > 0 && (
        <div className="rounded-md bg-amber-50 dark:bg-amber-950/40 p-3 text-sm text-amber-900 dark:text-amber-200">
          <div className="mb-1 flex items-center gap-1 font-medium"><AlertTriangle size={14} /> {warnings.length} 条兼容性提示</div>
          <ul className="space-y-0.5 text-xs">
            {warnings.map((w, i) => <li key={i}><span className="opacity-60">[{w.kind}]</span> <b>{w.name}</b>：{w.message}</li>)}
          </ul>
        </div>
      )}
      {res && (
        <div className="relative">
          <div className="absolute top-2 right-2"><CopyButton text={res.content} /></div>
          <pre className="max-h-[60vh] overflow-auto rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-4 font-mono text-xs leading-relaxed">{res.content}</pre>
        </div>
      )}
    </div>
  );
}
