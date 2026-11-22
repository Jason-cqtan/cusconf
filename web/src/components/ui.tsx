import { useEffect, useRef, useState, type ComponentProps, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type Ref, type SelectHTMLAttributes } from "react";
import { X, Check, Copy } from "lucide-react";

export const cx = (...c: (string | false | null | undefined)[]) => c.filter(Boolean).join(" ");

type Variant = "primary" | "secondary" | "ghost" | "danger";
const variants: Record<Variant, string> = {
  primary: "bg-indigo-600 text-white hover:bg-indigo-500 disabled:bg-indigo-600/50",
  secondary: "bg-white dark:bg-zinc-900 border border-zinc-300 dark:border-zinc-700 hover:bg-zinc-100 dark:hover:bg-zinc-800",
  ghost: "hover:bg-zinc-200/70 dark:hover:bg-zinc-800",
  danger: "text-red-600 hover:bg-red-50 dark:hover:bg-red-950/50",
};

export function Button({ variant = "secondary", size = "md", className, ...p }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: "sm" | "md" }) {
  return (
    <button
      {...p}
      className={cx(
        "inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60",
        size === "sm" ? "h-7 px-2 text-xs" : "h-9 px-3 text-sm",
        variants[variant], className,
      )}
    />
  );
}

const fieldBase = "rounded-md border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-2.5 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20";
// Width defaults to full unless the caller sets one.
const fieldCls = (c?: string) => cx(fieldBase, !/(^|\s)w-/.test(c ?? "") && "w-full", c);

export function Input({ className, ...p }: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...p} className={cx(fieldCls(className), !/(^|\s)h-/.test(className ?? "") && "h-9")} />;
}

export function Textarea({ className, ...p }: ComponentProps<"textarea">) {
  return <textarea {...p} className={cx(fieldCls(className), "py-2 font-mono text-xs")} />;
}

export function Select({ className, children, ...p }: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...p} className={cx(fieldCls(className), "pr-7", !/(^|\s)h-/.test(className ?? "") && "h-9")}>{children}</select>;
}

export function Field({ label, hint, children, className }: { label: string; hint?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <label
      className={cx("block space-y-1", className)}
      onClick={(e) => {
        // A <label> forwards clicks on its blank area / text to its first control. That is
        // useful for an input, but when the first control is a button (a chip's ×, an icon
        // picker, a toggle chip) it would fire that button. Block the forwarded click then.
        const target = e.target as Element;
        if (e.currentTarget.control instanceof HTMLButtonElement && !target.closest("button, input, select, textarea, a, [role=switch]")) {
          e.preventDefault();
        }
      }}
    >
      <span className="text-xs font-medium text-zinc-600 dark:text-zinc-400">{label}</span>
      {children}
      {hint && <span className="block text-xs text-zinc-500">{hint}</span>}
    </label>
  );
}

export function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label?: string }) {
  return (
    <label className="inline-flex cursor-pointer items-center gap-2 text-sm select-none">
      <span
        role="switch"
        aria-checked={checked}
        tabIndex={0}
        onKeyDown={(e) => (e.key === " " || e.key === "Enter") && (e.preventDefault(), onChange(!checked))}
        onClick={() => onChange(!checked)}
        className={cx("relative inline-block h-5 w-9 rounded-full transition-colors", checked ? "bg-indigo-600" : "bg-zinc-300 dark:bg-zinc-700")}
      >
        <span className={cx("absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-all", checked ? "left-4.5" : "left-0.5")} />
      </span>
      {label}
    </label>
  );
}

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cx("rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900", className)}>{children}</div>;
}

const badgeColors = {
  zinc: "bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300",
  indigo: "bg-indigo-50 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300",
  green: "bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300",
  amber: "bg-amber-50 text-amber-800 dark:bg-amber-950 dark:text-amber-300",
  red: "bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300",
};

export function Badge({ children, color = "zinc", className }: { children: ReactNode; color?: keyof typeof badgeColors; className?: string }) {
  return <span className={cx("inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs font-medium whitespace-nowrap", badgeColors[color], className)}>{children}</span>;
}

export function Modal({ open, onClose, title, children, footer, wide }: { open: boolean; onClose: () => void; title: string; children: ReactNode; footer?: ReactNode; wide?: boolean }) {
  useEffect(() => {
    if (!open) return;
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/40 p-4 pt-[8vh]" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={cx("w-full rounded-lg bg-white dark:bg-zinc-900 shadow-xl", wide ? "max-w-4xl" : "max-w-lg")}>
        <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-3">
          <h2 className="font-semibold">{title}</h2>
          <Button variant="ghost" size="sm" onClick={onClose} aria-label="关闭"><X size={16} /></Button>
        </div>
        <div className="max-h-[70vh] overflow-y-auto p-4">{children}</div>
        {footer && <div className="flex justify-end gap-2 border-t border-zinc-200 dark:border-zinc-800 px-4 py-3">{footer}</div>}
      </div>
    </div>
  );
}

/** Multi-select rendered as toggleable chips. */
export function Chips({ options, value, onChange, render }: { options: { value: string; label: string }[]; value: string[]; onChange: (v: string[]) => void; render?: (o: { value: string; label: string }) => ReactNode }) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {options.map((o) => {
        const on = value.includes(o.value);
        return (
          <button
            type="button"
            key={o.value}
            onClick={() => onChange(on ? value.filter((v) => v !== o.value) : [...value, o.value])}
            className={cx(
              "rounded-full border px-2.5 py-0.5 text-xs transition-colors",
              on ? "border-indigo-500 bg-indigo-50 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300" : "border-zinc-300 dark:border-zinc-700 hover:bg-zinc-100 dark:hover:bg-zinc-800",
            )}
          >
            {render ? render(o) : o.label}
          </button>
        );
      })}
    </div>
  );
}

export function CopyButton({ text, label = "复制" }: { text: string; label?: string }) {
  const [done, setDone] = useState(false);
  const t = useRef<number>(0);
  return (
    <Button
      size="sm"
      onClick={async () => {
        try { await navigator.clipboard.writeText(text); } catch { return; }
        setDone(true);
        clearTimeout(t.current);
        t.current = window.setTimeout(() => setDone(false), 1500);
      }}
    >
      {done ? <Check size={14} /> : <Copy size={14} />} {done ? "已复制" : label}
    </Button>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="rounded-lg border border-dashed border-zinc-300 dark:border-zinc-700 p-8 text-center text-sm text-zinc-500">{children}</div>;
}

/**
 * Pinned to the top of <main> while the page scrolls. Sticky offsets count from <main>'s
 * content edge, so the negative top / margins cancel its padding: the bar covers the full
 * width and nothing shows above it.
 */
export function StickyBar({ children, className, barRef }: { children: ReactNode; className?: string; barRef?: Ref<HTMLDivElement> }) {
  return (
    <div ref={barRef} className={cx("sticky -top-4 z-30 -mx-4 -mt-4 md:-top-8 bg-zinc-50 px-4 pt-4 md:-mx-8 md:-mt-8 md:px-8 md:pt-8 dark:bg-zinc-950", className)}>
      {children}
    </div>
  );
}

export function PageHeader({ title, desc, actions, sticky }: { title: string; desc?: string; actions?: ReactNode; sticky?: boolean }) {
  const head = (
    <div className={cx("flex flex-wrap items-end justify-between gap-3", sticky ? "pb-4" : "mb-5")}>
      <div>
        <h1 className="text-xl font-semibold">{title}</h1>
        {desc && <p className="mt-1 text-sm text-zinc-500">{desc}</p>}
      </div>
      <div className="flex gap-2">{actions}</div>
    </div>
  );
  return sticky ? <StickyBar className="mb-4 border-b border-zinc-200 dark:border-zinc-800">{head}</StickyBar> : head;
}

/**
 * Briefly highlights a newly added row. `flash(id)` marks it; the element carrying
 * `data-flash={id}` is scrolled into view and blinks (see .flash-row in index.css).
 */
export function useFlash(): [string | null, (id: string) => void] {
  const [id, setId] = useState<string | null>(null);
  const t = useRef<number>(0);
  useEffect(() => {
    if (!id) return;
    const raf = requestAnimationFrame(() => document.querySelector(`[data-flash="${CSS.escape(id)}"]`)?.scrollIntoView({ block: "center", behavior: "smooth" }));
    clearTimeout(t.current);
    t.current = window.setTimeout(() => setId(null), 2400);
    return () => cancelAnimationFrame(raf);
  }, [id]);
  return [id, setId];
}

export function ErrorText({ error }: { error?: string | null }) {
  if (!error) return null;
  return <p className="rounded-md bg-red-50 dark:bg-red-950/50 px-3 py-2 text-sm text-red-700 dark:text-red-300">{error}</p>;
}
