import { useI18n } from "../i18n";

type Props = { query: string; setQuery: (query: string) => void };

export function MobileComposer({ query, setQuery }: Props) {
  const { messages } = useI18n();
  return <div className="mobile-composer lg:hidden">
    <div className="brief-card flex items-end gap-2 rounded-xl border border-ink/20 bg-[#fffdf7] p-2 shadow-lift">
      <textarea value={query} onChange={(event) => setQuery(event.target.value)} aria-label={messages.briefAriaLabel} rows={2} className="max-h-24 min-h-[46px] flex-1 resize-none bg-transparent px-1.5 py-1 text-[13px] leading-5 outline-none" placeholder={messages.mobileBriefPlaceholder} />
    </div>
  </div>;
}
