import type { RecommendationStatus } from "../hooks/useRecommendations";
import type { RecommendationResponse } from "../types";
import { RecommendationCard } from "./RecommendationCard";

type Props = {
  data: RecommendationResponse | null;
  status: RecommendationStatus;
  expanded: string | null;
  onExpand: (id: string) => void;
};

export function ResultsPanel({ data, status, expanded, onExpand }: Props) {
  return <section className="results-panel order-1 bg-[rgba(247,245,237,.72)] pb-[190px] lg:order-2 lg:overflow-y-auto lg:pb-0" aria-busy={status === "loading"}>
    <div className="sticky top-0 z-10 border-b border-ink/15 bg-paper/90 px-5 py-4 backdrop-blur-md lg:px-7"><p className="text-sm font-medium text-ink/60" aria-live="polite">{data ? `Found: ${data.results.length} ${data.results.length === 1 ? "place" : "places"}` : "Finding places…"}</p></div>
    <div className={`grid gap-3 p-4 transition-opacity lg:p-6 ${status === "loading" && data ? "opacity-70" : "opacity-100"}`}>
      {!data && status === "loading" ? <LoadingCards /> : null}
      {data?.results.map((place, index) => <RecommendationCard key={place.place_id} place={place} rank={index + 1} showDistance={data.interpretation.location_mode !== "default"} expanded={expanded === place.place_id} onExpand={() => onExpand(place.place_id)} />)}
      {data && data.results.length === 0 ? <div className="mx-auto max-w-lg py-24 text-center"><p className="font-display text-5xl">No exact room, <i>yet.</i></p><p className="mt-4 text-sm leading-6 text-ink/55">One of your must-haves rules out every verified place. Soften a constraint and the list will return.</p></div> : null}
    </div>
  </section>;
}

function LoadingCards() {
  return <>{[0, 1, 2].map((item) => <div key={item} className="h-48 animate-soft-pulse rounded-xl border border-ink/10 bg-white/50" />)}</>;
}
