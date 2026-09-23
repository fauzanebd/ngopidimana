import { Leaf } from "lucide-react";

export function Hero() {
  return <section id="top" className="relative z-10 border-b border-ink/15 px-5 py-7 lg:px-8 lg:py-8">
    <div className="flex max-w-[1500px] flex-col justify-between gap-5 xl:flex-row xl:items-end">
      <div>
        <p className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-moss/75"><Leaf className="h-3.5 w-3.5" /> Find your right kind of room</p>
        <h1 className="font-display max-w-3xl text-[42px] leading-[0.96] tracking-[-0.035em] sm:text-[56px] lg:text-[64px]">A coffee place that fits <i>how you work.</i></h1>
      </div>
      <p className="max-w-lg text-sm leading-6 text-ink/60 xl:pb-1">Describe the day in your own words. Your constraints become visible, and the list reshapes as you edit.</p>
    </div>
  </section>;
}
