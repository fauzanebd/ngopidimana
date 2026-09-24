import { ArrowUpRight, Check, ChevronDown, CircleAlert, Clock3, ExternalLink, Gauge, MapPin } from "lucide-react";
import { useI18n } from "../i18n";
import { formatRupiah } from "../lib/format";
import { usePlacePhoto } from "../hooks/usePlacePhoto";
import type { Recommendation } from "../types";

type Props = { place: Recommendation; rank: number; showDistance: boolean; expanded: boolean; onExpand: () => void };

export function RecommendationCard({ place, rank, showDistance, expanded, onExpand }: Props) {
  const { messages } = useI18n();
  const { photoURL, attribution, reportBroken } = usePlacePhoto(place.google_place_id);
  const credit = photoURL && attribution?.name ? attribution : null;
  const creditClass = "absolute bottom-2 left-2 max-w-[calc(100%-1rem)] truncate rounded-md bg-ink/60 px-1.5 py-1 text-[10px] font-medium text-white backdrop-blur-sm";
  return <article className="result-card animate-soft-in overflow-hidden rounded-xl border border-ink/15 bg-[#fffdf7]/95 transition duration-200 hover:border-ink/30" style={{ animationDelay: `${Math.min(rank * 25, 250)}ms` }}>
    <div className="grid sm:grid-cols-[122px_1fr]">
      {/* The gradient and its leaves stay underneath the photo, so an absent, pending or broken
          image simply shows today's art — never an empty box, a broken icon or a layout shift. */}
      <div className={`place-art art-${place.accent} relative min-h-[96px] overflow-hidden border-b border-ink/10 sm:min-h-full sm:border-b-0 sm:border-r`} aria-hidden={photoURL ? undefined : true}>
        <span aria-hidden="true" className="absolute left-3 top-3 grid h-7 min-w-7 place-items-center rounded-md border border-white/40 bg-white/70 px-1.5 text-xs font-semibold text-ink backdrop-blur">{String(rank).padStart(2, "0")}</span>
        <div className="leaf-shape leaf-a" /><div className="leaf-shape leaf-b" />
        {photoURL ? <img src={photoURL} alt="" onError={reportBroken} className="absolute inset-0 h-full w-full object-cover" /> : null}
        {credit ? (credit.uri
          ? <a className={`focus-ring ${creditClass} transition hover:bg-ink/75`} href={credit.uri} target="_blank" rel="noreferrer">{messages.photoCredit(credit.name)}</a>
          : <span className={creditClass}>{messages.photoCredit(credit.name)}</span>) : null}
      </div>
      <div className="p-4 sm:p-5">
        <div className="flex items-start gap-3">
          <div><div className="mb-1 flex flex-wrap items-center gap-2"><h2 className="font-display text-[27px] leading-none">{place.name}</h2><span className="rounded-md bg-[#e8eddc] px-2 py-1 text-[11px] font-semibold text-moss">{messages.matchBadge(Math.round(place.match_score * 100))}</span></div><p className="flex items-center gap-1.5 text-xs text-ink/50"><MapPin className="h-3 w-3" /> {place.area}{showDistance ? ` · ${place.distance_km} km` : ""}</p></div>
        </div>
        <p className="mt-3 text-sm leading-6 text-ink/65">{place.description}</p>
        <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1.5 text-xs font-medium text-moss">{place.matched_on.map((item) => <span key={item} className="flex items-center gap-1.5"><Check className="h-3 w-3" /> {item}</span>)}</div>
        <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-ink/10 pt-3 text-xs text-ink/55">
          <span>{formatRupiah(place.price_min)}–{formatRupiah(place.price_max)}</span>
          <span className="flex items-center gap-1"><Clock3 className="h-3.5 w-3.5" /> {place.open_24_hours ? messages.open24Hours : messages.openUntil(place.open_until)}</span>
          <span className="flex items-center gap-1"><Gauge className="h-3.5 w-3.5" /> {messages.evidenceBadge(Math.round(place.result_confidence * 100))}</span>
        </div>
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
          <button onClick={onExpand} type="button" className="focus-ring flex items-center gap-1.5 rounded-sm text-xs font-medium text-ink/55 hover:text-ink" aria-expanded={expanded}>{messages.whyThisFits} <ChevronDown className={`h-3.5 w-3.5 transition ${expanded ? "rotate-180" : ""}`} /></button>
          <div className="flex items-center gap-2">
            {place.review_links[0] ? <a className="focus-ring inline-flex h-9 items-center gap-1.5 rounded-lg border border-ink/15 px-3 text-xs font-medium transition hover:bg-cream" href={place.review_links[0].url} target="_blank" rel="noreferrer">{messages.review} <ExternalLink className="h-3.5 w-3.5" /></a> : null}
            <a className="focus-ring inline-flex h-9 items-center gap-1.5 rounded-lg bg-moss px-3 text-xs font-semibold text-white transition hover:bg-[#1f3b2a]" href={place.maps_url} target="_blank" rel="noreferrer">{messages.openMaps} <ArrowUpRight className="h-3.5 w-3.5" /></a>
          </div>
        </div>
      </div>
    </div>
    {expanded ? <div className="animate-soft-in border-t border-ink/10 bg-[#f1f3e9] px-4 py-4 sm:pl-[142px] sm:pr-5"><div className="grid gap-4 sm:grid-cols-2">
      <div><p className="text-[11px] font-semibold text-ink/50">{messages.scoreBreakdown}</p><div className="mt-2 space-y-2">{Object.entries(place.score_components).map(([key, value]) => <div key={key} className="grid grid-cols-[92px_1fr_30px] items-center gap-2 text-[11px] text-ink/55"><span>{messages.scoreComponents[key] || key.replace("_", " ")}</span><span className="h-1.5 overflow-hidden rounded-full bg-ink/10"><span className="block h-full rounded-full bg-olive" style={{ width: `${value * 100}%` }} /></span><span className="text-right">{Math.round(value * 100)}</span></div>)}</div></div>
      <div className="text-xs leading-5 text-ink/55"><p className="text-[11px] font-semibold text-ink/50">{messages.evidenceNote}</p><p className="mt-2">{place.evidence_summary}</p>{place.caveats.map((caveat) => <p key={caveat} className="mt-1.5 flex gap-1.5 text-amber-900"><CircleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" /> {caveat}</p>)}</div>
    </div></div> : null}
  </article>;
}
