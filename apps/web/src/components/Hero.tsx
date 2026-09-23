import { Leaf } from "lucide-react";
import { useI18n } from "../i18n";
import { LanguageToggle } from "./LanguageToggle";
import { Title } from "./Title";

export function Hero() {
  const { messages } = useI18n();
  return <section id="top" className="relative z-10 border-b border-ink/15 px-5 py-7 lg:px-7 lg:py-8">
    <div className="mx-auto flex max-w-[1600px] items-start justify-between gap-4">
      <div className="min-w-0">
        <p className="mb-2 flex items-center gap-2 text-[13px] font-medium text-moss/80"><Leaf className="h-3.5 w-3.5" /> {messages.heroEyebrow}</p>
        <h1 className="font-display max-w-3xl text-balance text-[42px] leading-[0.96] tracking-[-0.035em] sm:text-[56px] lg:text-[64px]"><Title parts={messages.heroTitle} /></h1>
      </div>
      <LanguageToggle />
    </div>
  </section>;
}
