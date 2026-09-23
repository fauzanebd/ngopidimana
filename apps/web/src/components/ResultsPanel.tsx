import { ArrowUpRight } from "lucide-react";
import { useI18n, type TitlePart } from "../i18n";
import { formatRupiah } from "../lib/format";
import type { RecommendationStatus } from "../hooks/useRecommendations";
import type { RecommendationResponse, Requirement } from "../types";
import { RecommendationCard } from "./RecommendationCard";
import { Title } from "./Title";

type Props = {
  data: RecommendationResponse | null;
  status: RecommendationStatus;
  expanded: string | null;
  onExpand: (id: string) => void;
  onExample: (query: string) => void;
  requirements: Requirement[];
};

export function ResultsPanel({ data, status, expanded, onExpand, onExample, requirements }: Props) {
  const { messages } = useI18n();
  const idle = status === "idle";
  const budget = data?.interpretation.budget;
  return <section className="results-panel order-1 min-w-0 bg-[rgba(247,245,237,.72)] pb-[190px] lg:order-2 lg:overflow-y-auto lg:pb-0" aria-busy={status === "loading"}>
    {idle ? null : <div className="sticky top-0 z-10 bg-paper/90 px-5 py-3 backdrop-blur-md lg:px-7 lg:py-4">
      <p className="text-sm font-medium text-ink/60" aria-live="polite">{data ? messages.foundPlaces(data.results.length) : messages.findingPlaces}</p>
      {requirements.length > 0 || budget || data?.interpretation.location ? <div className="mt-2 flex gap-1.5 overflow-x-auto pb-0.5 [scrollbar-width:none] lg:hidden">
        {requirements.map((item) => <span key={item.key} className="whitespace-nowrap rounded-md border border-ink/15 bg-white/85 px-2 py-1 text-[9px] font-semibold text-ink/65">{item.label}</span>)}
        {budget ? <span className="whitespace-nowrap rounded-md border border-ink/15 bg-white/85 px-2 py-1 text-[9px] font-semibold text-ink/65">{messages.aroundBudget(formatRupiah(budget))}</span> : null}
        {data?.interpretation.location ? <span className="whitespace-nowrap rounded-md border border-ink/15 bg-white/85 px-2 py-1 text-[9px] font-semibold text-ink/65">{data.interpretation.location}</span> : null}
      </div> : null}
    </div>}
    <div className={`grid gap-3 p-4 transition-opacity lg:p-6 ${status === "loading" && data ? "opacity-70" : "opacity-100"}`}>
      {idle ? <MoodStart title={messages.moodTitle} body={messages.moodBody} examples={messages.examples} onExample={onExample} /> : null}
      {!idle && !data && status === "loading" ? <LoadingCards /> : null}
      {idle ? null : data?.results.map((place, index) => <RecommendationCard key={place.place_id} place={place} rank={index + 1} showDistance={data.interpretation.location_mode !== "default"} expanded={expanded === place.place_id} onExpand={() => onExpand(place.place_id)} />)}
      {!idle && data && data.results.length === 0 ? <div className="mx-auto max-w-lg py-24 text-center"><p className="font-display text-5xl"><Title parts={messages.emptyTitle} /></p><p className="mt-4 text-sm leading-6 text-ink/55">{messages.emptyBody}</p></div> : null}
    </div>
  </section>;
}

function MoodStart({ title, body, examples, onExample }: { title: TitlePart[]; body: string; examples: string[]; onExample: (query: string) => void }) {
  return <div className="mx-auto max-w-lg py-16">
    <p className="font-display text-center text-5xl"><Title parts={title} /></p>
    <p className="mt-4 text-center text-sm leading-6 text-ink/55">{body}</p>
    <div className="mt-8 space-y-2">
      {examples.map((example) => <button key={example} onClick={() => onExample(example)} type="button" className="focus-ring group flex w-full items-center justify-between gap-4 rounded-xl border border-ink/15 bg-white/60 px-4 py-3 text-left text-sm leading-5 text-ink/70 transition hover:border-ink/30 hover:bg-white/90 hover:text-ink">
        <span>{example}</span>
        <ArrowUpRight className="h-4 w-4 shrink-0 text-ink/30 transition group-hover:text-ink" />
      </button>)}
    </div>
  </div>;
}

function LoadingCards() {
  return <>{[0, 1, 2].map((item) => <div key={item} className="h-48 animate-soft-pulse rounded-xl border border-ink/10 bg-white/50" />)}</>;
}
