import { useState } from "react";
import { Hero } from "./components/Hero";
import { MobileComposer } from "./components/MobileComposer";
import { QueryPanel } from "./components/QueryPanel";
import { ResultsPanel } from "./components/ResultsPanel";
import { useRecommendations } from "./hooks/useRecommendations";

function App() {
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);
  const { data, status, error, warning, requirements } = useRecommendations(query);

  return <main className="flex min-h-[100dvh] flex-col overflow-hidden bg-paper text-ink">
    <div className="ambient ambient-one" aria-hidden="true" />
    <div className="ambient ambient-two" aria-hidden="true" />
    <Hero />
    <div className="relative z-10 mx-auto grid w-full max-w-[1600px] flex-1 grid-cols-[minmax(0,1fr)] grid-rows-[1fr_auto] lg:min-h-[620px] lg:grid-cols-[minmax(360px,40%)_minmax(0,60%)]">
      <QueryPanel query={query} setQuery={setQuery} data={data} error={error} warning={warning} requirements={requirements} />
      <ResultsPanel data={data} status={status} expanded={expanded} onExpand={(id) => setExpanded((current) => current === id ? null : id)} onExample={setQuery} requirements={requirements} />
    </div>
    <MobileComposer query={query} setQuery={setQuery} />
  </main>;
}

export default App;
