import { Link } from "react-router-dom";
import { AlertTriangle, Check, ChevronRight, Inbox, LoaderCircle, RefreshCw, ShieldCheck, Trash2, X } from "lucide-react";
import { FILTERS, filterSlug } from "../constants";
import type { FilterKey, Run } from "../types";

type Props = {
  runs: Run[];
  loading: boolean;
  activeFilter: FilterKey;
  selectedID: string | null;
  selectionMode: boolean;
  selectedIDs: Set<string>;
  bulkBusy: "publish" | "delete" | null;
  onToggleSelection: (id: string) => void;
  onToggleSelectionMode: () => void;
  onToggleAll: () => void;
  onBulkPublish: () => void;
  onBulkDelete: () => void;
  onRefresh: () => void;
};

export function RunQueue({ runs, loading, activeFilter, selectedID, selectionMode, selectedIDs, bulkBusy, onToggleSelection, onToggleSelectionMode, onToggleAll, onBulkPublish, onBulkDelete, onRefresh }: Props) {
  const selectedCount = runs.filter((run) => selectedIDs.has(run.id)).length;
  const publishableCount = runs.filter((run) => selectedIDs.has(run.id) && run.state !== "published").length;
  const allSelected = runs.length > 0 && runs.every((run) => selectedIDs.has(run.id));
  return <div className="border-b border-ink/15 bg-[#eeece4] xl:border-b-0 xl:border-r">
    <div className="flex h-14 items-center justify-between gap-3 border-b border-ink/15 px-5"><p className="truncate text-xs font-semibold uppercase tracking-[0.15em] text-ink/50">{selectionMode ? `${selectedCount} selected` : FILTERS.find((item) => item.key === activeFilter)?.label}</p><div className="flex shrink-0 items-center gap-1">{selectionMode ? <><button type="button" disabled={!runs.length || Boolean(bulkBusy)} onClick={onToggleAll} className="focus-ring rounded-md px-2 py-1.5 text-[11px] font-semibold text-moss hover:bg-white disabled:opacity-40">{allSelected ? "Clear all" : "Select all"}</button><button type="button" disabled={Boolean(bulkBusy)} onClick={onToggleSelectionMode} aria-label="Exit selection mode" className="focus-ring rounded-md p-1.5 text-ink/45 hover:bg-white disabled:opacity-40"><X className="h-4 w-4" /></button></> : <><button type="button" disabled={!runs.length} onClick={onToggleSelectionMode} className="focus-ring rounded-md px-2 py-1.5 text-[11px] font-semibold text-moss hover:bg-white disabled:opacity-40">Select</button><button type="button" onClick={onRefresh} aria-label="Refresh records" className="focus-ring rounded-md p-1.5 text-ink/45 hover:bg-white"><RefreshCw className="h-4 w-4" /></button></>}</div></div>
    <div className="max-h-[520px] overflow-y-auto xl:max-h-[calc(100vh-255px)]">
      {loading ? <div className="grid place-items-center py-24 text-ink/40"><LoaderCircle className="h-5 w-5 animate-spin" /></div> : null}
      {!loading && runs.length === 0 ? <div className="px-8 py-20 text-center"><Inbox className="mx-auto h-6 w-6 text-ink/35" /><p className="mt-3 font-display text-2xl">Nothing here.</p><p className="mt-1 text-xs leading-5 text-ink/45">Records in this stage will appear automatically.</p></div> : null}
      {runs.map((run) => <QueueItem key={run.id} run={run} href={`/${filterSlug(activeFilter)}/${run.id}`} active={!selectionMode && selectedID === run.id} selectionMode={selectionMode} checked={selectedIDs.has(run.id)} onToggle={() => onToggleSelection(run.id)} />)}
    </div>
    {selectionMode ? <div className="border-t border-ink/15 bg-[#f7f5ee] p-3"><p className="mb-2 text-[10px] leading-4 text-ink/45">Published records are skipped by Publish.</p><div className="grid grid-cols-2 gap-2"><button type="button" disabled={!publishableCount || Boolean(bulkBusy)} onClick={onBulkPublish} className="focus-ring inline-flex h-9 items-center justify-center gap-1.5 rounded-lg bg-moss px-3 text-xs font-semibold text-white disabled:opacity-40">{bulkBusy === "publish" ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <ShieldCheck className="h-3.5 w-3.5" />} Publish{publishableCount ? ` ${publishableCount}` : ""}</button><button type="button" disabled={!selectedCount || Boolean(bulkBusy)} onClick={onBulkDelete} className="focus-ring inline-flex h-9 items-center justify-center gap-1.5 rounded-lg border border-red-900/20 bg-white px-3 text-xs font-semibold text-red-700 disabled:opacity-40">{bulkBusy === "delete" ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />} Delete{selectedCount ? ` ${selectedCount}` : ""}</button></div></div> : null}
  </div>;
}

function QueueItem({ run, href, active, selectionMode, checked, onToggle }: { run: Run; href: string; active: boolean; selectionMode: boolean; checked: boolean; onToggle: () => void }) {
  const hasProblem = run.issues.length > 0 || run.state === "failed";
  const rowClass = `focus-ring flex w-full items-start gap-3 border-b border-ink/10 px-5 py-4 text-left transition ${checked || active ? "bg-[#fffdf7]" : "hover:bg-white/55"}`;
  const content = <>
    {selectionMode ? <span className={`mt-1 grid h-6 w-6 shrink-0 place-items-center rounded-md border transition ${checked ? "border-moss bg-moss text-white" : "border-ink/25 bg-white/60 text-transparent"}`}><Check className="h-3.5 w-3.5" /></span> : <span className={`mt-1 grid h-8 w-8 shrink-0 place-items-center rounded-lg ${hasProblem ? "bg-amber-100 text-amber-800" : run.state === "enriching" ? "bg-sky/70 text-slate-700" : "bg-emerald-100 text-emerald-800"}`}>{run.state === "enriching" ? <LoaderCircle className="h-4 w-4 animate-spin" /> : hasProblem ? <AlertTriangle className="h-4 w-4" /> : <Check className="h-4 w-4" />}</span>}
    <span className="min-w-0 flex-1"><span className="flex items-center justify-between gap-2"><strong className="truncate text-sm">{run.name}</strong><span className="text-[10px] text-ink/35">{relativeTime(run.updated_at)}</span></span><span className="mt-1 block text-xs text-ink/45">{run.area || sourceLabel(run.source)} · {Math.round(run.confidence * 100)}% confidence</span><span className="mt-2 block truncate text-[10px] text-ink/35">{run.state === "published" && selectionMode ? "Published · will be skipped by bulk publish" : run.issues[0] || run.url}</span></span>{selectionMode ? null : <ChevronRight className="mt-2 h-4 w-4 shrink-0 text-ink/25" />}
  </>;
  // A row is a real link to its own record, so it can be right-clicked, copied and opened in a
  // new tab. Selection mode takes that away: there the row toggles its checkbox instead.
  return selectionMode
    ? <button type="button" onClick={onToggle} aria-pressed={checked} className={rowClass}>{content}</button>
    : <Link to={href} aria-current={active ? "true" : undefined} className={rowClass}>{content}</Link>;
}

export function sourceLabel(source: string) { return source.split("_").map((part) => part[0]?.toUpperCase() + part.slice(1)).join(" "); }
export function relativeTime(value: string) { const mins = Math.max(0, Math.round((Date.now() - new Date(value).getTime()) / 60000)); if (mins < 1) return "now"; if (mins < 60) return `${mins}m`; const hours = Math.round(mins / 60); if (hours < 48) return `${hours}h`; return `${Math.round(hours / 24)}d`; }
