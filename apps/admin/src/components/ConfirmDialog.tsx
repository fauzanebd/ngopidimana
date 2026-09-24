import { useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";
import { LoaderCircle, Trash2 } from "lucide-react";

type Props = {
  title: string;
  description?: string;
  confirmLabel: string;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

// A destructive decision cannot go through window.confirm: a browser that suppresses dialogs
// (Chrome's "prevent this page from creating additional dialogs", any dialog policy, a headless
// run) returns false without ever showing anything, so the click looks dead. This is the one
// in-app confirmation every destructive action shares instead.
//
// It portals to the document body because the panel it is opened from runs an animation that
// leaves a transform on an ancestor, which would otherwise capture position: fixed and pin the
// overlay inside the panel instead of over the viewport.
export function ConfirmDialog({ title, description, confirmLabel, busy = false, onConfirm, onCancel }: Props) {
  const titleID = useId();
  const cancelButton = useRef<HTMLButtonElement>(null);

  // Cancel takes focus, never the destructive action: Enter should not be able to delete.
  useEffect(() => { cancelButton.current?.focus(); }, []);

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape" && !busy) onCancel();
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [busy, onCancel]);

  return createPortal(
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-ink/45 p-4"
      onPointerDown={(event) => { if (event.target === event.currentTarget && !busy) onCancel(); }}
    >
      <div role="dialog" aria-modal="true" aria-labelledby={titleID} className="w-full max-w-md animate-soft-in rounded-xl border border-ink/15 bg-[#fffdf8] p-5 shadow-lift">
        <h2 id={titleID} className="font-display text-2xl leading-tight">{title}</h2>
        {description ? <p className="mt-3 whitespace-pre-line text-sm leading-6 text-ink/60">{description}</p> : null}
        <div className="mt-5 flex items-center justify-end gap-2">
          <button ref={cancelButton} type="button" disabled={busy} onClick={onCancel} className="focus-ring h-9 rounded-lg border border-ink/15 bg-white/60 px-3 text-xs font-medium transition hover:bg-cream disabled:opacity-40">Cancel</button>
          <button type="button" disabled={busy} onClick={onConfirm} className="focus-ring inline-flex h-9 items-center gap-1.5 rounded-lg bg-red-700 px-3 text-xs font-semibold text-white transition hover:bg-red-800 disabled:opacity-40">{busy ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />} {confirmLabel}</button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
