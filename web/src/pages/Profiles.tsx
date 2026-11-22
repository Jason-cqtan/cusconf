import { useEffect, useState } from "react";
import { Plus, Trash2, Pencil, Copy } from "lucide-react";
import { api, type Profile } from "../api";
import { Badge, Button, Card, Empty, ErrorText, PageHeader } from "../components/ui";
import ProfileEditor from "./ProfileEditor";
import { tr } from "../i18n";

export default function ProfilesPage({ id, go }: { id?: string; go: (p: string) => void }) {
  if (id) return <ProfileEditor id={id} onBack={() => go("profiles")} onSaved={(p) => go(`profiles/${p.id}`)} />;
  return <ProfileList go={go} />;
}

function ProfileList({ go }: { go: (p: string) => void }) {
  const [items, setItems] = useState<Profile[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const load = () => api.profiles().then(setItems).catch((e) => setErr(e.message));
  useEffect(() => { load(); }, []);

  const duplicate = async (p: Profile) => {
    await api.saveProfile({ ...structuredClone(p), id: "", name: p.name + tr(" 副本") });
    load();
  };

  return (
    <>
      <PageHeader
        sticky
        title={tr("配置")}
        desc={tr("策略组、规则与规则集。同一份配置可以为不同客户端、不同版本生成订阅链接。")}
        actions={<Button variant="primary" onClick={() => go("profiles/new")}><Plus size={16} /> {tr("新建配置")}</Button>}
      />
      <ErrorText error={err} />
      {items.length === 0 ? (
        <Empty>{tr("还没有配置。新建时会预置常用的策略组与规则集，可以在此基础上修改。")}</Empty>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {items.map((p) => (
            <Card key={p.id} className="flex flex-col gap-3 p-4">
              <div className="font-medium">{p.name}</div>
              <div className="flex flex-wrap gap-1.5">
                <Badge>{p.groups.length} {tr("个策略组")}</Badge>
                <Badge>{p.rules.length} {tr("条规则")}</Badge>
                <Badge>{p.ruleSets.length} {tr("个规则集")}</Badge>
                <Badge>{p.sources?.length ? tr("{0} 个订阅源", p.sources.length) : tr("全部订阅源")}</Badge>
              </div>
              <div className="mt-auto flex gap-1">
                <Button size="sm" onClick={() => go(`profiles/${p.id}`)}><Pencil size={14} /> {tr("编辑")}</Button>
                <Button size="sm" variant="ghost" onClick={() => duplicate(p)}><Copy size={14} /> {tr("复制")}</Button>
                <Button size="sm" variant="danger" className="ml-auto" onClick={async () => {
                  if (!confirm(tr("删除配置「{0}」？使用它的订阅链接将失效。", p.name))) return;
                  await api.deleteProfile(p.id);
                  load();
                }}><Trash2 size={14} /></Button>
              </div>
            </Card>
          ))}
        </div>
      )}
    </>
  );
}
