import { CircleAlert, RefreshCw, Search } from "lucide-react";
import type { RecommendationStatus } from "../hooks/useRecommendations";
import { formatRupiah } from "../lib/format";
import type { Requirement } from "../types";

type Props = { query: string; setQuery: (query: string) => void; status: RecommendationStatus; requirements: Requirement[]; budget?: number };

export function MobileComposer({ query, setQuery, status, requirements, budget }: Props) {
  return <div className="mobile-composer lg:hidden">
    <div className="mb-2 flex max-w-full gap-1.5 overflow-x-auto pb-0.5 [scrollbar-width:none]">
      {requirements.slice(0, 4).map((item) => <span key={item.key} className={`whitespace-nowrap rounded-md border px-2 py-1 text-[9px] font-semibold ${item.kind === "hard" ? "border-moss bg-moss text-white" : "border-ink/15 bg-white/85 text-ink/65"}`}>{item.label}{item.kind === "hard" ? " · must" : ""}</span>)}
      {budget ? <span className="whitespace-nowrap rounded-md border border-ink/15 bg-white/85 px-2 py-1 text-[9px] font-semibold text-ink/65">{formatRupiah(budget)}</span> : null}
    </div>
    <div className="flex items-end gap-2 rounded-xl border border-ink/20 bg-[#fffdf7] p-2 shadow-lift">
      <textarea value={query} onChange={(event) => setQuery(event.target.value)} aria-label="Describe your ideal coffee place" rows={2} className="focus-ring max-h-24 min-h-[46px] flex-1 resize-none bg-transparent px-1.5 py-1 text-[13px] leading-5 outline-none" placeholder="Describe your ideal WFC place…" />
      <span className={`mb-1 grid h-9 w-9 shrink-0 place-items-center rounded-lg ${status === "error" ? "bg-red-100 text-red-700" : "bg-moss text-white"}`} aria-live="polite">{status === "loading" ? <RefreshCw className="h-4 w-4 animate-spin" /> : status === "error" ? <CircleAlert className="h-4 w-4" /> : <Search className="h-4 w-4" />}</span>
    </div>
  </div>;
}
