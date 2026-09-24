import { ExternalLink, LoaderCircle, MapPinned, Star } from "lucide-react";
import { useGooglePlace } from "../hooks/useGooglePlace";
import type { GoogleReview, Run } from "../types";

export function GooglePlacePanel({ run }: { run: Run }) {
  const { place, loading, error, load } = useGooglePlace(run.id);

  if (!run.google_place_id) {
    if (run.source !== "google_maps" || run.state === "enriching") return null;
    return <section className="mb-5 rounded-xl border border-ink/15 bg-white/55 p-4">
      <p className="text-xs font-bold uppercase tracking-[0.12em] text-ink/55">Official Google review reference</p>
      <p className="mt-2 text-xs leading-5 text-ink/50">No Google Place ID is linked yet, so this record cannot be published. Re-run extraction to resolve it; if it keeps failing, the enrichment note above records the reason — a venue name that does not match a Google listing, or Google Places not being configured on the API.</p>
    </section>;
  }

  if (!place) return <section className="mb-5 rounded-xl border border-ink/15 bg-white/65 p-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div><p className="text-xs font-bold uppercase tracking-[0.12em] text-ink/55">Official Google review reference</p><p className="mt-2 text-xs leading-5 text-ink/50">Load up to five current Google reviews on demand. Reviews fetched by the worker are also stored as attributed ingestion evidence and supplied to OpenRouter.</p></div>
      <button type="button" disabled={loading} onClick={() => void load()} className="focus-ring inline-flex h-9 items-center gap-2 rounded-lg bg-moss px-3 text-xs font-semibold text-white disabled:opacity-50">{loading ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <MapPinned className="h-3.5 w-3.5" />} {loading ? "Loading…" : "Load official reviews"}</button>
    </div>
    {error ? <p className="mt-3 text-xs text-red-700">{error}</p> : null}
  </section>;

  return <section className="mb-5 overflow-hidden rounded-xl border border-ink/15 bg-white/65">
    <div className="border-b border-ink/10 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><p className="text-xs font-bold uppercase tracking-[0.12em] text-ink/55">Official review reference</p><h3 className="mt-2 text-lg font-semibold">{place.displayName.text}</h3>{place.formattedAddress ? <p className="mt-1 text-xs text-ink/50">{place.formattedAddress}</p> : null}</div>
        <div className="text-right">{place.rating ? <p className="inline-flex items-center gap-1 text-sm font-semibold"><Star className="h-4 w-4 fill-amber-400 text-amber-500" /> {place.rating.toFixed(1)} <span className="font-normal text-ink/40">({place.userRatingCount || 0})</span></p> : null}<a href={place.googleMapsUri} target="_blank" rel="noreferrer" className="mt-2 flex items-center justify-end gap-1 text-xs font-medium text-moss hover:underline">Open place <ExternalLink className="h-3 w-3" /></a></div>
      </div>
      <p className="mt-3 text-[11px] leading-5 text-ink/45">Current API response for comparison with the stored ingestion evidence. Google returns at most five reviews, sorted by relevance.</p>
    </div>
    <div>{place.reviews.length ? place.reviews.map((review) => <ReviewCard key={review.name || review.googleMapsUri} review={review} />) : <p className="p-5 text-sm text-ink/45">Google returned no review text for this place.</p>}</div>
    <div className="flex flex-wrap items-center justify-between gap-2 border-t border-ink/10 bg-[#f8f8f6] px-4 py-3">
      <span translate="no" className="whitespace-nowrap text-xs font-normal text-[#5e5e5e]">Google Maps</span>
      {place.attributions?.map((attribution) => attribution.providerUri ? <a key={attribution.provider} href={attribution.providerUri} target="_blank" rel="noreferrer" className="text-xs text-ink/50 hover:underline">{attribution.provider}</a> : <span key={attribution.provider} className="text-xs text-ink/50">{attribution.provider}</span>)}
    </div>
  </section>;
}

function ReviewCard({ review }: { review: GoogleReview }) {
  const author = review.authorAttribution;
  const translated = Boolean(review.originalText?.text && review.text?.text && review.originalText.text !== review.text.text);
  return <article className="border-b border-ink/10 p-4 last:border-b-0">
    <div className="flex items-center gap-3">
      {author.photoUri ? <img src={author.photoUri} alt="" referrerPolicy="no-referrer" className="h-8 w-8 rounded-full bg-cream object-cover" /> : <span className="grid h-8 w-8 place-items-center rounded-full bg-cream text-xs font-semibold">{author.displayName?.slice(0, 1) || "?"}</span>}
      <div className="min-w-0">{author.uri ? <a href={author.uri} target="_blank" rel="noreferrer" className="block truncate text-xs font-semibold hover:underline">{author.displayName}</a> : <p className="truncate text-xs font-semibold">{author.displayName}</p>}<p className="mt-0.5 text-[10px] text-ink/40">{"★".repeat(Math.max(0, Math.min(5, Math.round(review.rating))))}{review.relativePublishTimeDescription ? ` · ${review.relativePublishTimeDescription}` : ""}</p></div>
    </div>
    {review.text?.text ? <p className="mt-3 text-sm leading-6 text-ink/70">{review.text.text}</p> : <p className="mt-3 text-xs text-ink/40">Rating only; no review text.</p>}
    <div className="mt-2 flex flex-wrap items-center gap-3 text-[10px] text-ink/45">{translated ? <span>Translated; original is available on Google Maps</span> : null}<a href={review.googleMapsUri} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 font-medium text-moss hover:underline">View review on Google Maps <ExternalLink className="h-2.5 w-2.5" /></a>{review.flagContentUri ? <a href={review.flagContentUri} target="_blank" rel="noreferrer" className="hover:underline">Report</a> : null}</div>
  </article>;
}
