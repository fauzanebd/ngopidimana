import { FILTERS } from "../constants";
import type { FilterKey, Run } from "../types";

type Props = { runs: Run[]; active: FilterKey; onChange: (filter: FilterKey) => void };

export function AdminSidebar({ runs, active, onChange }: Props) {
  return <aside className="hidden border-r border-ink/15 bg-[#ece9df] p-4 lg:block">
    <nav className="space-y-1" aria-label="Admin sections">{FILTERS.map(({ key, label, icon: Icon }) => {
      const count = key === "all" ? runs.length : runs.filter((run) => run.state === key).length;
      return <button key={key} onClick={() => onChange(key)} type="button" className={`focus-ring flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition ${active === key ? "bg-moss text-white" : "text-ink/60 hover:bg-white/65 hover:text-ink"}`}><Icon className={`h-4 w-4 ${key === "enriching" && count ? "animate-spin" : ""}`} /><span className="flex-1 text-left">{label}</span><span className={`text-[11px] ${active === key ? "text-white/65" : "text-ink/40"}`}>{count}</span></button>;
    })}</nav>
    <div className="mt-8 border-t border-ink/15 pt-5"><p className="px-3 text-[10px] font-bold uppercase tracking-[0.17em] text-ink/40">Worker model</p><div className="mt-3 rounded-xl border border-ink/10 bg-white/55 p-3"><p className="text-xs font-semibold">Redis + Asynq</p><p className="mt-2 text-[10px] leading-4 text-ink/45">Submitted sources are processed outside the API request lifecycle.</p></div></div>
  </aside>;
}
