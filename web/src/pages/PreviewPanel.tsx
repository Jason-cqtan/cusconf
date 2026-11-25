import { useEffect, useState } from "react";
import { AlertTriangle, Download } from "lucide-react";
import { api, clientList, defaultPlatform, platformList, type Capabilities, type Client, type Profile, type RenderResult, type Target } from "../api";
import { Button, CopyButton, ErrorText, Field, Input, Select } from "../components/ui";
import { tr } from "../i18n";

export function TargetPicker({ value, onChange, caps }: { value: Target; onChange: (t: Target) => void; caps: Capabilities | null }) {
  const ci = caps?.clients[value.client];
  const latest = ci?.platforms[value.platform]?.latest ?? "";
  const single = platformList(ci).length === 1;
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
      <Field label={tr("客户端")}>
        <Select value={value.client} onChange={(e) => {
          const client = e.target.value as Client;
          onChange({ client, platform: defaultPlatform(caps?.clients[client]), version: "" });
        }}>
          {clientList(caps).map(([k, c]) => <option key={k} value={k}>{c.label}</option>)}
        </Select>
      </Field>
      <Field label={tr("平台")}>
        <Select value={value.platform} disabled={single} onChange={(e) => onChange({ ...value, platform: e.target.value, version: "" })}>
          {platformList(ci).map(([k, p]) => <option key={k} value={k}>{p.label}</option>)}
        </Select>
      </Field>
      <Field label={tr("版本")} hint={tr("留空 = 最新（{0}），低版本会自动降级或剔除不支持的节点和参数", latest)}>
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
  const ci = caps?.clients[target.client];
  const ext = (ci?.ext ?? ".conf").replace(/^\./, "");
  const platform = caps?.clients[target.client]?.platforms[target.platform]?.label ?? `${target.client} ${target.platform}`;
  const fileName = `${profile.name || "cusconf"} - ${platform}${target.version ? ` ${target.version}` : ""}.${ext}`;
  // Saves the generated text as a file, for importing into the app by hand.
  const download = () => {
    if (!res) return;
    const url = URL.createObjectURL(new Blob([res.content], { type: res.contentType }));
    const a = Object.assign(document.createElement("a"), { href: url, download: fileName });
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-3">
        <div className="min-w-0 flex-1"><TargetPicker value={target} onChange={setTarget} caps={caps} /></div>
        <Button onClick={run} disabled={busy}>{busy ? tr("生成中…") : tr("重新生成")}</Button>
        <Button variant="primary" onClick={download} disabled={!res || busy} title={fileName}><Download size={16} /> {tr("下载配置文件")}</Button>
      </div>
      {res && (
        <p className="text-xs text-zinc-500">
          {tr("下载的文件按当前编辑内容（含未保存的修改）生成，可在")} {ci?.label ?? target.client} {tr("中手动导入；它不带订阅地址，不会自动更新。 需要自动更新请在「订阅链接」页创建链接后导入或下载。")}
        </p>
      )}
      <ErrorText error={err} />
      {warnings.length > 0 && (
        <div className="rounded-md bg-amber-50 dark:bg-amber-950/40 p-3 text-sm text-amber-900 dark:text-amber-200">
          <div className="mb-1 flex items-center gap-1 font-medium"><AlertTriangle size={14} /> {warnings.length} {tr("条兼容性提示")}</div>
          <ul className="space-y-0.5 text-xs">
            {warnings.map((w, i) => <li key={i}><span className="opacity-60">[{w.kind}]</span> <b>{w.name}</b>{tr("：")}{w.message}</li>)}
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
