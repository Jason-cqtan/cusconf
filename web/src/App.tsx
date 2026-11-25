import { useEffect, useState } from "react";
import { Cloud, Server, SlidersHorizontal, Link2, LogOut } from "lucide-react";
import { api, session, setUnauthorizedHandler } from "./api";
import { Button, ErrorText, Input, cx } from "./components/ui";
import SourcesPage from "./pages/Sources";
import NodesPage from "./pages/Nodes";
import ProfilesPage from "./pages/Profiles";
import TokensPage from "./pages/Tokens";
import { PrefsSwitch } from "./components/PrefsSwitch";
import { tr } from "./i18n";

const pages = [
  { id: "sources", label: tr("订阅源"), icon: Cloud },
  { id: "nodes", label: tr("节点"), icon: Server },
  { id: "profiles", label: tr("配置"), icon: SlidersHorizontal },
  { id: "tokens", label: tr("订阅链接"), icon: Link2 },
] as const;
type PageId = (typeof pages)[number]["id"];

function useHashRoute(): [string, (p: string) => void] {
  const read = () => location.hash.replace(/^#\/?/, "") || "sources";
  const [route, setRoute] = useState(read);
  useEffect(() => {
    const h = () => setRoute(read());
    window.addEventListener("hashchange", h);
    return () => window.removeEventListener("hashchange", h);
  }, []);
  return [route, (p) => { location.hash = "/" + p; }];
}

export default function App() {
  const [authed, setAuthed] = useState(!!session.get());
  const [route, go] = useHashRoute();
  useEffect(() => setUnauthorizedHandler(() => setAuthed(false)), []);

  if (!authed) return <Login onDone={() => setAuthed(true)} />;

  const [page, ...rest] = route.split("/");
  const current = (pages.find((p) => p.id === page)?.id ?? "sources") as PageId;

  // Phones: a slim top bar and a bottom tab bar; md and up: a sidebar.
  return (
    <div className="flex h-dvh flex-col overflow-hidden md:flex-row">
      <aside className="flex shrink-0 items-center gap-1 border-b border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-3 py-1.5 pt-[max(0.375rem,env(safe-area-inset-top))] md:w-52 md:flex-col md:items-stretch md:border-b-0 md:border-r md:py-4">
        <div className="mr-auto px-2 font-semibold tracking-tight md:mr-0 md:mb-4">cusconf</div>
        <nav className="hidden flex-1 gap-1 md:flex md:flex-col">
          {pages.map((p) => (
            <button
              key={p.id}
              onClick={() => go(p.id)}
              className={cx(
                "flex items-center gap-2 rounded-md px-2.5 py-1.5 text-sm whitespace-nowrap",
                current === p.id ? "bg-indigo-50 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300 font-medium" : "text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-800",
              )}
            >
              <p.icon size={16} /> {p.label}
            </button>
          ))}
        </nav>
        <PrefsSwitch className="shrink-0 md:mb-2 md:px-1" />
        <Button variant="ghost" size="sm" className="md:justify-start" aria-label={tr("退出")} onClick={() => { session.clear(); setAuthed(false); }}>
          <LogOut size={14} /> <span className="hidden md:inline">{tr("退出")}</span>
        </Button>
      </aside>
      <main className="min-h-0 min-w-0 flex-1 overflow-y-auto p-4 md:p-8">
        {current === "sources" && <SourcesPage />}
        {current === "nodes" && <NodesPage />}
        {current === "profiles" && <ProfilesPage id={rest[0]} go={go} />}
        {current === "tokens" && <TokensPage />}
      </main>
      <nav className="grid shrink-0 grid-cols-4 border-t border-zinc-200 bg-white pb-[env(safe-area-inset-bottom)] md:hidden dark:border-zinc-800 dark:bg-zinc-900">
        {pages.map((p) => (
          <button
            key={p.id}
            onClick={() => go(p.id)}
            className={cx("flex flex-col items-center gap-0.5 py-1.5 text-[11px]", current === p.id ? "font-medium text-indigo-600 dark:text-indigo-400" : "text-zinc-500 dark:text-zinc-400")}
          >
            <p.icon size={20} /> {p.label}
          </button>
        ))}
      </nav>
    </div>
  );
}

function Login({ onDone }: { onDone: () => void }) {
  const [pw, setPw] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  return (
    <div className="relative flex min-h-screen items-center justify-center p-4">
      <PrefsSwitch className="absolute top-4 right-4" />
      <form
        className="w-full max-w-sm space-y-4 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setErr(null);
          try {
            const r = await api.login(pw);
            session.set(r.token);
            onDone();
          } catch (e) {
            setErr((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <div>
          <h1 className="text-lg font-semibold">cusconf</h1>
          <p className="text-sm text-zinc-500">{tr("订阅转换与配置生成")}</p>
        </div>
        <Input type="password" placeholder={tr("管理员密码")} value={pw} onChange={(e) => setPw(e.target.value)} autoFocus />
        <ErrorText error={err} />
        <Button variant="primary" className="w-full" disabled={busy || !pw}>{tr("登录")}</Button>
      </form>
    </div>
  );
}
