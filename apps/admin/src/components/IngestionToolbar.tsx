import { AlertTriangle, Check, Link2, LoaderCircle, Sparkles } from "lucide-react";
import { FormEvent, useState } from "react";

type Props = { submitting: boolean; notice: string; error: string; onSubmit: (url: string) => Promise<unknown> };

export function IngestionToolbar({ submitting, notice, error, onSubmit }: Props) {
  const [url, setURL] = useState("");
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!url.trim()) return;
    const created = await onSubmit(url.trim());
    if (created) setURL("");
  }
  return <div className="border-b border-ink/15 bg-[#f7f5ee] px-5 py-6 lg:px-8">
    <div className="flex flex-col justify-between gap-5 xl:flex-row xl:items-end">
      <div><p className="text-[10px] font-bold uppercase tracking-[0.18em] text-moss/65">Data operations</p><h1 className="mt-1 font-display text-[40px] leading-none">A trustworthy catalogue, <i>one source at a time.</i></h1></div>
      <form onSubmit={submit} className="flex w-full max-w-xl gap-2"><label className="relative flex-1"><span className="sr-only">Café URL</span><Link2 className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink/35" /><input type="url" value={url} onChange={(event) => setURL(event.target.value)} required placeholder="Paste a café URL" className="focus-ring h-11 w-full rounded-lg border border-ink/20 bg-white pl-10 pr-3 text-sm outline-none placeholder:text-ink/35" /></label><button disabled={submitting} className="focus-ring inline-flex h-11 items-center gap-2 rounded-lg bg-moss px-4 text-sm font-semibold text-white transition hover:bg-[#203d2b] disabled:opacity-60">{submitting ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />} Ingest</button></form>
    </div>
    {(notice || error) ? <div className={`mt-4 flex items-center gap-2 text-xs ${error ? "text-red-800" : "text-moss"}`}>{error ? <AlertTriangle className="h-4 w-4" /> : <Check className="h-4 w-4" />}{error || notice}</div> : null}
  </div>;
}
