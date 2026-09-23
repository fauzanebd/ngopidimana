import { useState } from "react";
import { Plus, X } from "lucide-react";
import type { ManualEvidenceInput } from "../types";

const CATEGORIES = [
  "Other note", "Seating comfort", "Table size", "Wi-Fi quality", "Power outlets",
  "Quietness", "Crowd level", "Laptop policy", "Long-stay friendliness", "Opening hours",
  "Minimum price", "Maximum price", "Accessibility", "Parking notes", "Ambience",
];

type Props = {
  disabled: boolean;
  onAdd: (input: ManualEvidenceInput) => Promise<boolean>;
};

export function ManualEvidenceForm({ disabled, onAdd }: Props) {
  const [open, setOpen] = useState(false);
  const [key, setKey] = useState("Other note");
  const [value, setValue] = useState("");

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!value.trim()) return;
    if (await onAdd({ key, value: value.trim() })) {
      setValue("");
      setOpen(false);
    }
  }

  if (!open) return <button type="button" disabled={disabled} onClick={() => setOpen(true)} className="focus-ring mb-5 inline-flex items-center gap-2 rounded-lg border border-moss/30 bg-[#eef1e5] px-3 py-2 text-xs font-semibold text-moss hover:bg-[#e4ead7] disabled:opacity-40"><Plus className="h-3.5 w-3.5" /> Add manual evidence</button>;

  return <form onSubmit={submit} className="mb-5 rounded-xl border border-moss/25 bg-[#eef1e5] p-4">
    <div className="flex items-start justify-between gap-4">
      <div><h3 className="text-xs font-bold uppercase tracking-[0.12em] text-moss">Add manual evidence</h3><p className="mt-1 text-[11px] leading-5 text-ink/50">Add something you verified yourself. Choose “Other note” for a useful detail that does not fit another category. Its source will be shown as “Manually added.”</p></div>
      <button type="button" aria-label="Close manual evidence form" onClick={() => setOpen(false)} className="focus-ring grid h-7 w-7 shrink-0 place-items-center rounded-md hover:bg-white/60"><X className="h-4 w-4" /></button>
    </div>
    <div className="mt-3 grid gap-3 sm:grid-cols-[180px_minmax(0,1fr)]">
      <label className="text-[11px] font-semibold text-ink/60">Category<select value={key} onChange={(event) => setKey(event.target.value)} className="focus-ring mt-1.5 h-10 w-full rounded-lg border border-ink/15 bg-[#fffdf8] px-3 text-xs text-ink">{CATEGORIES.map((category) => <option key={category}>{category}</option>)}</select></label>
      <label className="text-[11px] font-semibold text-ink/60">Observation<textarea autoFocus value={value} onChange={(event) => setValue(event.target.value)} maxLength={800} rows={3} placeholder="e.g. Upstairs seats have back support, but the small round tables barely fit a laptop." className="focus-ring mt-1.5 w-full resize-y rounded-lg border border-ink/15 bg-[#fffdf8] px-3 py-2.5 text-sm leading-5 text-ink placeholder:text-ink/30" /></label>
    </div>
    <div className="mt-3 flex items-center justify-between gap-3"><span className="text-[10px] text-ink/35">{value.length}/800</span><button type="submit" disabled={disabled || !value.trim()} className="focus-ring rounded-lg bg-moss px-4 py-2 text-xs font-semibold text-white disabled:opacity-40">{disabled ? "Saving…" : "Add evidence"}</button></div>
  </form>;
}
