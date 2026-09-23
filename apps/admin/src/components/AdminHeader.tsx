import { Coffee } from "lucide-react";

export function AdminHeader({ connected }: { connected: boolean }) {
  return <header className="flex h-16 items-center justify-between border-b border-ink/15 bg-[#f7f5ee] px-5 lg:px-7">
    <div className="flex items-center gap-3"><div className="grid h-9 w-9 place-items-center rounded-lg bg-moss text-white"><Coffee className="h-5 w-5" /></div><div className="leading-none"><p className="font-display text-[23px]">where to <i>WFC</i></p><p className="mt-1 text-[9px] font-bold uppercase tracking-[0.22em] text-ink/45">Catalogue admin</p></div></div>
    <div className="flex items-center gap-3"><span className="hidden items-center gap-2 text-xs text-ink/55 sm:flex"><span className={`h-2 w-2 rounded-full ${connected ? "bg-emerald-600" : "bg-red-600"}`} /> {connected ? "API connected" : "API unavailable"}</span><button className="focus-ring grid h-9 w-9 place-items-center rounded-full border border-ink/15 bg-butter/60 text-xs font-bold">FA</button></div>
  </header>;
}
