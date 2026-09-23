import { useI18n, type Locale } from "../i18n";

const LOCALES: { value: Locale; label: string }[] = [
  { value: "en", label: "English" },
  { value: "id", label: "Bahasa Indonesia" },
];

export function LanguageToggle() {
  const { locale, setLocale, messages } = useI18n();
  return <div role="group" aria-label={messages.language} className="flex shrink-0 items-center gap-0.5 rounded-full border border-ink/15 bg-white/70 p-0.5">
    {LOCALES.map((option) => <button key={option.value} type="button" onClick={() => setLocale(option.value)} aria-label={option.label} aria-pressed={locale === option.value} className={`focus-ring rounded-full px-2.5 py-1 text-[11px] font-semibold transition ${locale === option.value ? "bg-moss text-white" : "text-ink/55 hover:text-ink"}`}>{option.value.toUpperCase()}</button>)}
  </div>;
}
