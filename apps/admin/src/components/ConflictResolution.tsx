import { Check, ExternalLink } from "lucide-react";
import type { EvidenceField } from "../types";

type Props = {
  fields: EvidenceField[];
  disabled: boolean;
  onChoose: (id: string) => void;
};

export function ConflictResolution({ fields, disabled, onChoose }: Props) {
  const groups = new Map<string, EvidenceField[]>();
  fields.filter((field) => field.conflict && !field.excluded).forEach((field) => groups.set(field.key, [...(groups.get(field.key) || []), field]));
  if (groups.size === 0) return null;

  return <section id="conflict-resolution" className="mb-5 scroll-mt-5 rounded-xl border border-amber-900/20 bg-[#fffaf0] p-4">
    <h3 className="text-xs font-bold uppercase tracking-[0.12em] text-amber-900">Resolve conflicting evidence</h3>
    <p className="mt-1 text-[11px] leading-5 text-ink/50">Choose the value the catalogue should use. Other values stay in the audit trail but are excluded from publication and matching.</p>
    <div className="mt-4 space-y-4">{[...groups].map(([key, candidates]) => <div key={key}>
      <p className="mb-2 text-xs font-semibold">{key}</p>
      <div className="grid gap-2">{candidates.map((field) => <div key={field.id || `${field.value}-${field.source_url}`} className="flex flex-col justify-between gap-3 rounded-lg border border-ink/10 bg-white/70 p-3 sm:flex-row sm:items-center">
        <div className="min-w-0"><p className="text-sm font-medium">{field.value}</p><p className="mt-1 flex flex-wrap items-center gap-2 text-[10px] text-ink/45"><span>{field.source}</span><span>{Math.round(field.confidence * 100)}% confidence</span>{field.source_url ? <a href={field.source_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-moss hover:underline">Open source <ExternalLink className="h-2.5 w-2.5" /></a> : null}</p></div>
        <button type="button" disabled={disabled || !field.id} onClick={() => field.id && onChoose(field.id)} className="focus-ring inline-flex h-8 shrink-0 items-center justify-center gap-1.5 rounded-md bg-moss px-3 text-[11px] font-semibold text-white disabled:opacity-40"><Check className="h-3.5 w-3.5" /> Use this value</button>
      </div>)}</div>
    </div>)}</div>
  </section>;
}
