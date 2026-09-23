import { useState } from "react";
import { Hero } from "./components/Hero";
import { MobileComposer } from "./components/MobileComposer";
import { QueryPanel } from "./components/QueryPanel";
import { ResultsPanel } from "./components/ResultsPanel";
import { STARTER_QUERY } from "./constants";
import { useRecommendations } from "./hooks/useRecommendations";

function App() {
  const [query, setQuery] = useState(STARTER_QUERY);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [showIntent, setShowIntent] = useState(true);
  const { data, status, error, warning, requirements } = useRecommendations(query);

  return <main className="min-h-[100dvh] overflow-hidden bg-paper text-ink">
    <div className="ambient ambient-one" aria-hidden="true" />
    <div className="ambient ambient-two" aria-hidden="true" />
    <Hero />
    <div className="relative z-10 mx-auto grid max-w-[1600px] lg:h-[calc(100dvh-163px)] lg:min-h-[620px] lg:grid-cols-[minmax(360px,40%)_minmax(0,60%)]">
      <QueryPanel query={query} setQuery={setQuery} data={data} status={status} error={error} warning={warning} requirements={requirements} showIntent={showIntent} setShowIntent={setShowIntent} />
      <ResultsPanel data={data} status={status} expanded={expanded} onExpand={(id) => setExpanded((current) => current === id ? null : id)} />
    </div>
    <MobileComposer query={query} setQuery={setQuery} status={status} requirements={requirements} budget={data?.interpretation.budget} />
  </main>;
}

export default App;
