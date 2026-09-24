import { useEffect, useRef, useState } from "react";
import { AlertTriangle, Archive, ArrowRight, ExternalLink, FileSearch, ImageOff, Link2, MoreHorizontal, Pencil, RefreshCw, RotateCcw, ShieldCheck, Trash2, X } from "lucide-react";
import type { EvidenceField, PhotoOverride, Run, RunAction } from "../types";
import { GooglePlacePanel } from "./GooglePlacePanel";
import { ManualEvidenceForm } from "./ManualEvidenceForm";
import { ConflictResolution } from "./ConflictResolution";
import { relativeTime, sourceLabel } from "./RunQueue";

type Props = { run: Run | null; deleting: boolean; savingEvidence: boolean; onAction: (action: RunAction) => void; onDelete: () => void; onAddEvidence: (input: { key: string; value: string }) => Promise<boolean>; onRemoveEvidence: (id: string) => void; onReplaceEvidence: (id: string, value: string) => Promise<boolean>; onEvidenceAction: (id: string, action: "exclude" | "restore" | "choose") => void; onSavePhoto: (input: PhotoOverride) => Promise<boolean>; onClearPhoto: () => void };

export function ReviewPanel({ run, deleting, savingEvidence, onAction, onDelete, onAddEvidence, onRemoveEvidence, onReplaceEvidence, onEvidenceAction, onSavePhoto, onClearPhoto }: Props) {
  if (!run) return <div className="grid min-h-[520px] place-items-center p-8 text-center"><div><FileSearch className="mx-auto h-8 w-8 text-ink/30" /><p className="mt-4 font-display text-3xl">Select a record to review.</p><p className="mt-2 text-sm text-ink/45">Evidence and conflicts stay visible before anything is published.</p></div></div>;
  const canPublish = run.state === "needs_review" && run.issues.length === 0;
  const warnings = run.warnings || [];
  return <div className="animate-soft-in">
    <div className="flex flex-col justify-between gap-4 border-b border-ink/15 px-5 py-5 sm:flex-row sm:items-center lg:px-7">
      <div><div className="flex flex-wrap items-center gap-2"><h2 className="font-display text-[32px] leading-none">{run.name}</h2><StateBadge state={run.state} /></div><p className="mt-2 flex items-center gap-1.5 text-xs text-ink/45"><Link2 className="h-3.5 w-3.5" /><a href={run.url} target="_blank" rel="noreferrer" className="max-w-md truncate hover:text-moss hover:underline">{run.url}</a><ExternalLink className="h-3 w-3" /></p></div>
      <div className="flex items-center gap-2"><button onClick={() => onAction("archive")} className="focus-ring h-9 rounded-lg border border-ink/15 px-3 text-xs font-medium hover:bg-cream"><Archive className="mr-1 inline h-3.5 w-3.5" /> Archive</button>{run.state === "failed" || run.state === "stale" ? <button onClick={() => onAction("refresh")} className="focus-ring inline-flex h-9 items-center gap-2 rounded-lg bg-moss px-3 text-xs font-semibold text-white"><RefreshCw className="h-3.5 w-3.5" /> Retry</button> : <button onClick={() => onAction("publish")} disabled={!canPublish} title={!canPublish ? "Resolve conflicts before publishing" : "Publish to search"} className="focus-ring inline-flex h-9 items-center gap-2 rounded-lg bg-moss px-3 text-xs font-semibold text-white disabled:cursor-not-allowed disabled:opacity-40"><ShieldCheck className="h-3.5 w-3.5" /> Publish</button>}<OverflowMenu run={run} deleting={deleting} onRefresh={() => onAction("refresh")} onDelete={onDelete} /></div>
    </div>
    <div className="grid gap-6 p-5 lg:grid-cols-[minmax(0,1fr)_260px] lg:p-7">
      <div>
        {run.state === "enriching" ? <PipelineState stage={run.stage} progress={run.progress} /> : null}
        {run.issues.length ? <div className="mb-5 rounded-xl border border-amber-900/20 bg-amber-50 p-4"><div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.12em] text-amber-900"><AlertTriangle className="h-4 w-4" /> {run.state === "failed" ? "Processing failed" : "Publication blocked"}</div>{run.issues.map((issue) => <p key={issue} className="mt-2 text-sm leading-6 text-amber-950/70">{issue}</p>)}{run.fields.some((field) => field.conflict && !field.excluded) ? <button type="button" onClick={() => document.getElementById("conflict-resolution")?.scrollIntoView({ behavior: "smooth", block: "start" })} className="mt-3 inline-flex items-center gap-1.5 text-xs font-semibold text-amber-900">Open conflict resolution <ArrowRight className="h-3.5 w-3.5" /></button> : null}</div> : null}
        {warnings.length ? <div className="mb-5 rounded-xl border border-sky/70 bg-sky/25 p-4"><div className="text-xs font-bold uppercase tracking-[0.12em] text-slate-700">Non-blocking enrichment note</div>{warnings.map((warning) => <p key={warning} className="mt-2 text-sm leading-6 text-slate-600">{warning}</p>)}</div> : null}
        <ConflictResolution fields={run.fields} disabled={savingEvidence} onChoose={(id) => onEvidenceAction(id, "choose")} />
        <GooglePlacePanel key={run.id} run={run} />
        {run.state === "enriching" || run.state === "archived" ? null : <CoverPhotoSection key={`cover-photo-${run.id}`} run={run} saving={savingEvidence} onSave={onSavePhoto} onClear={onClearPhoto} />}
        <ManualEvidenceForm disabled={savingEvidence || run.state === "enriching" || run.state === "archived"} onAdd={onAddEvidence} />
        <section><div className="mb-3 flex items-center justify-between"><h3 className="text-xs font-semibold uppercase tracking-[0.15em] text-ink/50">Evidence</h3><span className="text-xs text-ink/40">{run.fields.filter((field) => !field.excluded).length} active{run.fields.some((field) => field.excluded) ? ` · ${run.fields.filter((field) => field.excluded).length} excluded` : ""}</span></div><div className="overflow-hidden rounded-xl border border-ink/15 bg-[#fffdf8]">{run.fields.length ? [...run.fields].sort((a, b) => Number(Boolean(a.excluded)) - Number(Boolean(b.excluded))).map((field) => <EvidenceRow key={field.id || `${field.key}-${field.source}-${field.source_url || field.value}`} field={field} busy={savingEvidence} onRemove={onRemoveEvidence} onReplace={onReplaceEvidence} onAction={onEvidenceAction} />) : <div className="px-5 py-14 text-center text-sm text-ink/40">Evidence will appear as processing completes.</div>}</div></section>
      </div>
      <aside className="space-y-4"><div className="rounded-xl border border-ink/15 bg-[#eef1e5] p-4"><p className="text-[10px] font-bold uppercase tracking-[0.15em] text-ink/40">Record confidence</p><p className="mt-2 font-display text-4xl">{Math.round(run.confidence * 100)}<span className="text-xl">%</span></p><div className="mt-3 h-1.5 overflow-hidden rounded-full bg-ink/10"><div className="h-full rounded-full bg-olive" style={{ width: `${run.confidence * 100}%` }} /></div><p className="mt-3 text-[11px] leading-5 text-ink/45">Confidence is evidence coverage, not match quality.</p></div><div className="rounded-xl border border-ink/15 bg-white/65 p-4"><p className="text-[10px] font-bold uppercase tracking-[0.15em] text-ink/40">Provenance</p><dl className="mt-3 space-y-3 text-xs"><div><dt className="text-ink/40">Source type</dt><dd className="mt-0.5 font-medium">{sourceLabel(run.source)}</dd></div><div><dt className="text-ink/40">Added</dt><dd className="mt-0.5 font-medium">{new Date(run.created_at).toLocaleString()}</dd></div><div><dt className="text-ink/40">Last updated</dt><dd className="mt-0.5 font-medium">{relativeTime(run.updated_at)}</dd></div></dl></div></aside>
    </div>
  </div>;
}

function OverflowMenu({ run, deleting, onRefresh, onDelete }: { run: Run; deleting: boolean; onRefresh: () => void; onDelete: () => void }) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);

  useEffect(() => { setOpen(false); }, [run.id]);
  useEffect(() => {
    if (!open) return;
    function dismiss(event: PointerEvent) {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    }
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") setOpen(false);
    }
    document.addEventListener("pointerdown", dismiss);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", dismiss);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  function requestDelete() {
    setOpen(false);
    if (window.confirm(`Delete “${run.name}”?\n\nThis permanently removes the ingestion record and its extracted evidence.`)) onDelete();
  }

  function requestRefresh() {
    setOpen(false);
    onRefresh();
  }

  return <div ref={container} className="relative">
    <button type="button" aria-label="More record actions" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((current) => !current)} className="focus-ring grid h-9 w-9 place-items-center rounded-lg border border-ink/15 bg-white/40 hover:bg-cream"><MoreHorizontal className="h-4 w-4" /></button>
    {open ? <div role="menu" className="absolute right-0 top-11 z-30 w-48 rounded-lg border border-ink/15 bg-[#fffdf8] p-1.5 shadow-xl shadow-ink/10">
      <button type="button" role="menuitem" disabled={run.state === "enriching" || deleting} onClick={requestRefresh} className="focus-ring flex w-full items-center gap-2 rounded-md px-3 py-2.5 text-left text-xs font-semibold text-ink/75 hover:bg-cream disabled:opacity-40"><RefreshCw className="h-4 w-4" /> Re-run extraction</button>
      <div className="my-1 border-t border-ink/10" />
      <button type="button" role="menuitem" disabled={deleting} onClick={requestDelete} className="focus-ring flex w-full items-center gap-2 rounded-md px-3 py-2.5 text-left text-xs font-semibold text-red-700 hover:bg-red-50 disabled:opacity-50"><Trash2 className="h-4 w-4" /> {deleting ? "Deleting…" : "Delete record"}</button>
    </div> : null}
  </div>;
}

function PipelineState({ stage, progress }: { stage: string; progress: number }) {
  const labels: Record<string, string> = { queued: "Waiting for worker", fetch_source: "Fetching source", extract_evidence: "Extracting deterministic evidence", resolve_google_place: "Resolving the stable Google Place ID", fetch_google_reviews: "Fetching official Google reviews", discover_sources: "Discovering corroborating sources with Exa", merge_evidence: "Attaching source context", structured_extraction: "Extracting cited facts with OpenRouter", normalize_evidence: "Normalizing evidence and detecting conflicts", ready_for_review: "Ready for review" };
  return <div className="mb-5 rounded-xl border border-sky/70 bg-sky/25 p-4"><div className="flex items-center justify-between text-xs font-semibold"><span className="flex items-center gap-2"><RefreshCw className="h-4 w-4 animate-spin" /> Enrichment in progress</span><span>{progress}%</span></div><div className="mt-3 h-1.5 overflow-hidden rounded-full bg-ink/10"><div className="h-full rounded-full bg-moss transition-all" style={{ width: `${progress}%` }} /></div><p className="mt-2 text-[11px] text-ink/50">{labels[stage] || stage.replaceAll("_", " ")}</p></div>;
}

function CoverPhotoSection({ run, saving, onSave, onClear }: { run: Run; saving: boolean; onSave: (input: PhotoOverride) => Promise<boolean>; onClear: () => void }) {
  const savedURL = run.photo_override?.url?.trim() || "";
  const savedAttribution = run.photo_override?.attribution || "";
  const [url, setURL] = useState(savedURL);
  const [attribution, setAttribution] = useState(savedAttribution);
  const [broken, setBroken] = useState(false);
  useEffect(() => { setURL(savedURL); setAttribution(savedAttribution); setBroken(false); }, [savedURL, savedAttribution]);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!url.trim()) return;
    await onSave({ url: url.trim(), attribution: attribution.trim() });
  }

  return <form onSubmit={submit} className="mb-5 rounded-xl border border-ink/15 bg-white/65 p-4">
    <p className="text-xs font-bold uppercase tracking-[0.12em] text-ink/55">Cover photo</p>
    <p className="mt-1 text-[11px] leading-5 text-ink/50">This image is served instead of Google’s photo, so it costs nothing and never expires. Use one the project has the rights to use — supplied by the venue or licensed.</p>
    <div className="mt-3 grid gap-3 sm:grid-cols-[180px_minmax(0,1fr)]">
      <div>
        <p className="text-[11px] font-semibold text-ink/60">Current photo</p>
        {savedURL ? (broken
          ? <div className="mt-1.5 grid h-28 w-full place-items-center rounded-lg border border-dashed border-ink/20 bg-[#f8f8f6] px-3 text-center"><span><ImageOff className="mx-auto h-5 w-5 text-ink/30" /><span className="mt-1 block text-[10px] leading-4 text-ink/45">This image could not be loaded. Check the URL.</span></span></div>
          : <img src={savedURL} alt="Cover photo override preview" onError={() => setBroken(true)} className="mt-1.5 h-28 w-full rounded-lg border border-ink/15 bg-cream object-cover" />)
          : <div className="mt-1.5 grid h-28 w-full place-items-center rounded-lg border border-dashed border-ink/20 bg-[#f8f8f6] px-3 text-center text-[10px] leading-4 text-ink/45">No override — this record falls back to the Google photo.</div>}
        {savedURL && savedAttribution ? <p className="mt-1.5 truncate text-[10px] text-ink/45">{savedAttribution}</p> : null}
      </div>
      <div className="grid content-start gap-3">
        <label className="text-[11px] font-semibold text-ink/60">Image URL<input value={url} onChange={(event) => setURL(event.target.value)} maxLength={800} placeholder="https://…" spellCheck={false} className="focus-ring mt-1.5 h-10 w-full rounded-lg border border-ink/15 bg-[#fffdf8] px-3 text-xs text-ink placeholder:text-ink/30" /></label>
        <label className="text-[11px] font-semibold text-ink/60">Credit<textarea value={attribution} onChange={(event) => setAttribution(event.target.value)} maxLength={200} rows={2} placeholder="e.g. Venue photo" className="focus-ring mt-1.5 w-full resize-y rounded-lg border border-ink/15 bg-[#fffdf8] px-3 py-2.5 text-xs leading-5 text-ink placeholder:text-ink/30" /></label>
      </div>
    </div>
    <div className="mt-3 flex items-center justify-between gap-3">
      <span className="text-[10px] text-ink/35">{attribution.length}/200</span>
      <div className="flex items-center gap-2">
        {savedURL ? <button type="button" disabled={saving} onClick={onClear} className="focus-ring inline-flex h-9 items-center gap-2 rounded-lg border border-ink/15 px-3 text-xs font-medium hover:bg-cream disabled:opacity-40"><Trash2 className="h-3.5 w-3.5" /> Clear</button> : null}
        <button type="submit" disabled={saving || !url.trim()} className="focus-ring rounded-lg bg-moss px-4 py-2 text-xs font-semibold text-white disabled:opacity-40">{saving ? "Saving…" : "Save photo"}</button>
      </div>
    </div>
  </form>;
}

function EvidenceRow({ field, busy, onRemove, onReplace, onAction }: { field: EvidenceField; busy: boolean; onRemove: (id: string) => void; onReplace: (id: string, value: string) => Promise<boolean>; onAction: (id: string, action: "exclude" | "restore" | "choose") => void }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(field.value);
  useEffect(() => { setDraft(field.value); setEditing(false); }, [field.id, field.value]);
  async function saveEdit() { if (field.id && draft.trim() && await onReplace(field.id, draft.trim())) setEditing(false); }
  function exclude() { if (field.id && window.confirm(`Exclude this ${field.key} evidence from catalogue facts and matching?\n\nThe original source remains visible in the audit trail.`)) onAction(field.id, "exclude"); }
  return <div className={`grid grid-cols-[100px_minmax(0,1fr)] gap-3 border-b border-ink/10 px-4 py-4 last:border-b-0 sm:grid-cols-[112px_minmax(0,1fr)] ${field.excluded ? "bg-ink/[0.025] opacity-55" : ""}`}><div><p className="text-xs font-semibold">{field.key}</p>{field.conflict ? <span className="mt-1 inline-flex items-center gap-1 text-[10px] font-semibold text-amber-800"><AlertTriangle className="h-3 w-3" /> Conflict</span> : null}{field.excluded ? <span className="mt-1 block text-[10px] font-semibold uppercase tracking-wide text-ink/45">Excluded</span> : null}</div><div className="min-w-0">{editing ? <div><textarea autoFocus value={draft} onChange={(event) => setDraft(event.target.value)} maxLength={800} rows={3} className="focus-ring w-full resize-y rounded-lg border border-ink/20 bg-white px-3 py-2 text-sm" /><p className="mt-1 text-[10px] text-ink/45">{field.method === "manual" ? "Updates this manual entry." : "Creates a manual correction and excludes the original sourced value."}</p><div className="mt-2 flex gap-2"><button type="button" disabled={busy || !draft.trim()} onClick={() => void saveEdit()} className="focus-ring rounded-md bg-moss px-3 py-1.5 text-[11px] font-semibold text-white disabled:opacity-40">Save correction</button><button type="button" onClick={() => { setDraft(field.value); setEditing(false); }} className="focus-ring inline-flex items-center gap-1 rounded-md px-2 py-1.5 text-[11px] font-semibold"><X className="h-3 w-3" /> Cancel</button></div></div> : <><div className="flex items-start justify-between gap-3"><p className="min-w-0 text-sm leading-5">{field.value}</p><span className="flex shrink-0 items-center gap-1.5 pt-0.5"><span className={`h-2 w-2 rounded-full ${field.confidence > .9 ? "bg-emerald-600" : field.confidence > .7 ? "bg-amber-500" : "bg-red-500"}`} /><span className="text-xs tabular-nums text-ink/55">{Math.round(field.confidence * 100)}%</span></span></div>{field.author_name ? <div className="mt-2 flex items-center gap-2">{field.author_photo ? <img src={field.author_photo} alt="" referrerPolicy="no-referrer" className="h-6 w-6 rounded-full bg-cream object-cover" /> : null}{field.author_url ? <a href={field.author_url} target="_blank" rel="noreferrer" className="text-[11px] font-medium hover:underline">{field.author_name}</a> : <span className="text-[11px] font-medium">{field.author_name}</span>}</div> : null}{field.excerpt ? <p className="mt-2 border-l-2 border-olive/35 pl-3 text-xs leading-5 text-ink/60">{field.excerpt}</p> : null}<p className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-[10px] text-ink/40"><span>{field.source}</span>{field.method ? <span>{field.method.replaceAll("_", " ")}</span> : null}{field.extractor ? <span>{field.extractor}</span> : null}{field.source_url ? <a href={field.source_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 font-medium text-moss hover:underline">Open source <ExternalLink className="h-2.5 w-2.5" /></a> : null}{field.published_at ? <span>Published {new Date(field.published_at).toLocaleDateString()}</span> : null}</p>{field.id ? <div className="mt-2 flex flex-wrap gap-2">{field.excluded ? <button type="button" disabled={busy} onClick={() => onAction(field.id!, "restore")} className="focus-ring inline-flex items-center gap-1 text-[10px] font-semibold text-moss disabled:opacity-40"><RotateCcw className="h-3 w-3" /> Restore</button> : <><button type="button" disabled={busy} onClick={() => setEditing(true)} className="focus-ring inline-flex items-center gap-1 text-[10px] font-semibold text-moss disabled:opacity-40"><Pencil className="h-3 w-3" /> Edit as correction</button>{field.method === "manual" ? <button type="button" disabled={busy} onClick={() => onRemove(field.id!)} className="focus-ring inline-flex items-center gap-1 text-[10px] font-semibold text-red-700 disabled:opacity-40"><Trash2 className="h-3 w-3" /> Delete</button> : <button type="button" disabled={busy} onClick={exclude} className="focus-ring inline-flex items-center gap-1 text-[10px] font-semibold text-red-700 disabled:opacity-40"><Trash2 className="h-3 w-3" /> Exclude</button>}</>}</div> : null}</>}</div></div>;
}

function StateBadge({ state }: { state: Run["state"] }) {
  const styles = state === "needs_review" ? "bg-amber-100 text-amber-900" : state === "published" ? "bg-emerald-100 text-emerald-900" : state === "enriching" ? "bg-sky/60 text-slate-800" : state === "failed" ? "bg-red-100 text-red-900" : "bg-cream text-ink/60";
  return <span className={`rounded-md px-2 py-1 text-[9px] font-bold uppercase tracking-[0.1em] ${styles}`}>{state.replace("_", " ")}</span>;
}
