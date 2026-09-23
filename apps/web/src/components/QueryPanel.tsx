import { MapPin } from "lucide-react";
import { useI18n } from "../i18n";
import { formatRupiah } from "../lib/format";
import type { RecommendationResponse, Requirement } from "../types";
import { IntentChip } from "./IntentChip";

type Props = {
  query: string;
  setQuery: (query: string) => void;
  data: RecommendationResponse | null;
  error: string;
  warning: string;
  requirements: Requirement[];
};

export function QueryPanel({ query, setQuery, data, error, warning, requirements }: Props) {
  const { messages } = useI18n();
  // On mobile the criteria live in the results header, so this panel only has a
  // reason to exist below lg when it carries a message the list cannot show.
  return <aside className={`query-panel order-2 border-t border-ink/15 bg-[#f4f1e7]/88 lg:order-1 lg:overflow-y-auto lg:border-r lg:border-t-0 ${error || warning ? "" : "hidden lg:block"}`}>
    <div className="p-5 pb-32 lg:p-7 lg:pb-8">
      <div className="composer relative hidden rounded-2xl border border-ink/20 bg-[#fffdf7] p-4 shadow-lift lg:block">
        <textarea value={query} onChange={(event) => setQuery(event.target.value)} aria-label={messages.briefAriaLabel} className="min-h-[150px] w-full resize-none bg-transparent text-[18px] leading-7 outline-none placeholder:text-ink/35" placeholder={messages.briefPlaceholder} />
      </div>
      {error && <p className="mt-3 rounded-lg border border-red-900/15 bg-red-50 px-3 py-2 text-xs text-red-800">{messages.staleList(error)}</p>}
      {warning && !error ? <p className="mt-3 rounded-lg border border-amber-900/15 bg-amber-50 px-3 py-2 text-xs text-amber-900">{warning}</p> : null}

      {data ? <div className="mt-8 hidden lg:block">
        <p className="text-sm text-ink/60">{data.interpretation.summary}</p>
        <div className="mt-4 flex flex-wrap gap-2">
          {requirements.map((item) => <IntentChip key={item.key} requirement={item} />)}
          {data.interpretation.budget ? <span className="intent-chip"><span className="h-1.5 w-1.5 rounded-full bg-olive" /> {messages.aroundBudget(formatRupiah(data.interpretation.budget))}</span> : null}
          {data.interpretation.location ? <span className="intent-chip"><MapPin className="h-3 w-3" /> {data.interpretation.location}</span> : null}
        </div>
      </div> : null}
    </div>
  </aside>;
}
