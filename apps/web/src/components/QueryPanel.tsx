import { ArrowUpRight, Check, ChevronDown, CircleAlert, MapPin, RefreshCw, SlidersHorizontal, Sparkles } from "lucide-react";
import { EXAMPLE_QUERIES } from "../constants";
import type { RecommendationStatus } from "../hooks/useRecommendations";
import { formatRupiah } from "../lib/format";
import type { RecommendationResponse, Requirement } from "../types";
import { IntentChip } from "./IntentChip";

type Props = {
  query: string;
  setQuery: (query: string) => void;
  data: RecommendationResponse | null;
  status: RecommendationStatus;
  error: string;
  warning: string;
  requirements: Requirement[];
  showIntent: boolean;
  setShowIntent: (value: boolean) => void;
};

export function QueryPanel({ query, setQuery, data, status, error, warning, requirements, showIntent, setShowIntent }: Props) {
  return <aside className="query-panel order-2 border-t border-ink/15 bg-[#f4f1e7]/88 lg:order-1 lg:overflow-y-auto lg:border-r lg:border-t-0">
    <div className="p-5 pb-32 lg:p-7 lg:pb-8">
      <div className="mb-4 flex items-center justify-between"><p className="text-xs font-semibold uppercase tracking-[0.17em] text-ink/55">Your brief</p><span className="text-xs text-ink/45">updates after 350ms</span></div>
      <div className="composer relative hidden rounded-2xl border border-ink/20 bg-[#fffdf7] p-4 shadow-lift lg:block">
        <textarea value={query} onChange={(event) => setQuery(event.target.value)} aria-label="Describe your ideal coffee place" className="focus-ring min-h-[150px] w-full resize-none bg-transparent text-[18px] leading-7 outline-none placeholder:text-ink/35" placeholder="e.g. quiet enough for four hours of coding, under 50k, with a musholla…" />
        <div className="mt-4 flex items-center justify-between border-t border-ink/10 pt-3">
          <div className="flex items-center gap-2 text-xs text-ink/45"><Sparkles className="h-4 w-4 text-moss" />{data?.interpretation.provider === "jev" ? `Jev · ${data.interpretation.latency_ms}ms` : "Indonesian + English"}</div>
          <div className={`flex items-center gap-2 text-xs font-medium ${status === "error" ? "text-red-700" : "text-moss"}`} aria-live="polite">
            {status === "loading" ? <><RefreshCw className="h-3.5 w-3.5 animate-spin" /> Updating</> : status === "error" ? <><CircleAlert className="h-3.5 w-3.5" /> Retry on edit</> : <><Check className="h-3.5 w-3.5" /> Live</>}
          </div>
        </div>
      </div>
      {error && <p className="mt-3 rounded-lg border border-red-900/15 bg-red-50 px-3 py-2 text-xs text-red-800">{error}. Your last good list stays visible.</p>}
      {warning && !error ? <p className="mt-3 rounded-lg border border-amber-900/15 bg-amber-50 px-3 py-2 text-xs text-amber-900">{warning}</p> : null}

      <div className="mt-8">
        <button onClick={() => setShowIntent(!showIntent)} className="focus-ring flex w-full items-center justify-between rounded-sm" type="button" aria-expanded={showIntent}>
          <span className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.17em] text-ink/55"><SlidersHorizontal className="h-3.5 w-3.5" /> We understood</span>
          <ChevronDown className={`h-4 w-4 transition ${showIntent ? "rotate-180" : ""}`} />
        </button>
        {showIntent ? <div className="animate-soft-in">
          <p className="mt-3 text-sm text-ink/60">{data?.interpretation.summary || "Start typing to make your requirements visible."}</p>
          <div className="mt-4 flex flex-wrap gap-2">
            {requirements.map((item) => <IntentChip key={item.key} requirement={item} />)}
            {data?.interpretation.budget ? <span className="intent-chip"><span className="h-1.5 w-1.5 rounded-full bg-olive" /> Around {formatRupiah(data.interpretation.budget)}</span> : null}
            {data?.interpretation.location ? <span className="intent-chip"><MapPin className="h-3 w-3" /> {data.interpretation.location}</span> : null}
          </div>
        </div> : null}
      </div>

      <div className="mt-8 border-t border-ink/15 pt-5">
        <p className="mb-3 text-xs font-semibold uppercase tracking-[0.17em] text-ink/55">Try another mood</p>
        <div className="space-y-2">{EXAMPLE_QUERIES.map((example) => <button key={example} onClick={() => setQuery(example)} type="button" className="focus-ring group flex w-full items-center justify-between gap-4 rounded-lg py-2 text-left text-sm leading-5 text-ink/60 transition hover:text-ink"><span>{example}</span><ArrowUpRight className="h-4 w-4 shrink-0 opacity-0 transition group-hover:opacity-100" /></button>)}</div>
      </div>
    </div>
  </aside>;
}
