import { Coffee, LogOut } from "lucide-react";
import type { Contributor } from "../types";

function initials(email: string) {
  const local = email.split("@")[0] || email;
  const parts = local.split(/[._+-]+/).filter(Boolean);
  if (parts.length > 1) return (parts[0][0] + parts[1][0]).toUpperCase();
  return local.slice(0, 2).toUpperCase();
}

export function AdminHeader({ connected, contributor, onSignOut }: { connected: boolean; contributor: Contributor; onSignOut: () => void }) {
  return <header className="flex h-16 items-center justify-between border-b border-ink/15 bg-[#f7f5ee] px-5 lg:px-7">
    <div className="flex items-center gap-3"><div className="grid h-9 w-9 place-items-center rounded-lg bg-moss text-white"><Coffee className="h-5 w-5" /></div><div className="leading-none"><p className="font-display text-[23px]">where to <i>WFC</i></p><p className="mt-1 text-[9px] font-bold uppercase tracking-[0.22em] text-ink/45">Catalogue admin</p></div></div>
    <div className="flex items-center gap-3"><span className="hidden items-center gap-2 text-xs text-ink/55 sm:flex"><span className={`h-2 w-2 rounded-full ${connected ? "bg-emerald-600" : "bg-red-600"}`} /> {connected ? "API connected" : "API unavailable"}</span><span className="hidden max-w-[200px] truncate text-xs text-ink/60 sm:inline" title={contributor.display_name || contributor.email}>{contributor.email}</span><span aria-hidden="true" title={contributor.display_name || contributor.email} className="grid h-9 w-9 place-items-center rounded-full border border-ink/15 bg-butter/60 text-xs font-bold">{initials(contributor.email)}</span><button type="button" onClick={onSignOut} className="focus-ring inline-flex h-9 items-center gap-1.5 rounded-lg border border-ink/15 px-3 text-xs font-medium transition hover:bg-cream"><LogOut className="h-3.5 w-3.5" /> Sign out</button></div>
  </header>;
}
