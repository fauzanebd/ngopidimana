import { Coffee, LoaderCircle } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, Navigate, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { AdminHeader } from "./components/AdminHeader";
import { AdminSidebar } from "./components/AdminSidebar";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { IngestionToolbar } from "./components/IngestionToolbar";
import { ReviewPanel } from "./components/ReviewPanel";
import { RunQueue } from "./components/RunQueue";
import { SignInPanel } from "./components/SignInPanel";
import { FILTERS, filterFromSlug, filterSlug } from "./constants";
import { useApiHealth } from "./hooks/useApiHealth";
import { useIngestionRuns } from "./hooks/useIngestionRuns";
import { useSession } from "./hooks/useSession";
import type { Contributor, FilterKey, ManualEvidenceInput, PhotoOverride, Run, RunAction } from "./types";

// A delete waiting on confirmation. The records are captured when the dialog opens and the
// handler deletes exactly those — it never re-derives "the selected record" afterwards.
type PendingDelete = { kind: "single"; run: Run } | { kind: "bulk"; runs: Run[] };

function App({ callbackToken = null }: { callbackToken?: string | null }) {
  const session = useSession(callbackToken);

  if (session.status === "checking") return <main className="grid min-h-screen place-items-center bg-paper text-ink">
    <div className="flex flex-col items-center gap-4"><div className="grid h-11 w-11 place-items-center rounded-xl bg-moss text-white"><Coffee className="h-6 w-6" /></div><p className="flex items-center gap-2 text-xs text-ink/55"><LoaderCircle className="h-3.5 w-3.5 animate-spin" /> {callbackToken === null ? "Checking your session…" : "Signing you in…"}</p></div>
  </main>;

  if (session.status === "signed-out" || !session.contributor) return <SignInPanel signingIn={session.signingIn} error={session.signInError} sentTo={session.sentTo} onSignIn={session.signIn} onUseAnotherAddress={session.clearSentTo} />;

  return <Routes>
    <Route path="/" element={<Navigate to="/needs-review" replace />} />
    <Route path="/:filter" element={<FilterRoute contributor={session.contributor} onSignOut={session.signOut} />} />
    <Route path="/:filter/:runId" element={<FilterRoute contributor={session.contributor} onSignOut={session.signOut} />} />
    <Route path="*" element={<Navigate to="/needs-review" replace />} />
  </Routes>;
}

// A filter that is not one of the queue's stages is a URL nobody should have opened, so it
// lands on the default queue instead of rendering an empty shell.
function FilterRoute({ contributor, onSignOut }: { contributor: Contributor; onSignOut: () => Promise<void> }) {
  const { filter, runId } = useParams();
  const stage = filterFromSlug(filter);
  if (!stage) return <Navigate to="/needs-review" replace />;
  return <Dashboard filter={stage} runId={runId || null} contributor={contributor} onSignOut={onSignOut} />;
}

function Dashboard({ filter, runId, contributor, onSignOut }: { filter: FilterKey; runId: string | null; contributor: Contributor; onSignOut: () => Promise<void> }) {
  const navigate = useNavigate();
  const { runs, loading, reconnecting, submitting, deletingID, savingEvidence, bulkBusy, notice, error, load, create, update, remove, publishMany, removeMany, addEvidence, removeEvidence, replaceEvidence, excludeEvidence, restoreEvidence, chooseEvidence, savePhotoOverride, clearPhotoOverride } = useIngestionRuns(() => void onSignOut());
  const apiHealthy = useApiHealth();
  const [selectionMode, setSelectionMode] = useState(false);
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(() => new Set());
  const [pendingDelete, setPendingDelete] = useState<PendingDelete | null>(null);
  const visibleRuns = useMemo(() => filter === "all" ? runs : runs.filter((run) => run.state === filter), [filter, runs]);
  // The record on screen is exactly the one the URL names. There is deliberately no fallback to
  // the first visible record: that fallback is how a click meant for one record used to land on
  // another after a poll re-sorted the queue or the selected record left the filter.
  const selected = runId ? runs.find((run) => run.id === runId) ?? null : null;
  const bulkSelection = useMemo(() => runs.filter((run) => selectedIDs.has(run.id)), [runs, selectedIDs]);
  const firstVisibleID = visibleRuns[0]?.id ?? null;

  useEffect(() => {
    const currentIDs = new Set(runs.map((run) => run.id));
    setSelectedIDs((current) => {
      const next = new Set([...current].filter((id) => currentIDs.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [runs]);

  // Switching filters drops the record and leaves selection mode, whether the change came from a
  // link, the back button or a pasted URL.
  useEffect(() => {
    setSelectionMode(false);
    setSelectedIDs(new Set());
  }, [filter]);

  // A bare filter URL fills in the record it is already showing first. Replacing the entry keeps
  // Back pointing at the previous view rather than walking back through each auto-selection.
  useEffect(() => {
    if (runId || loading || !firstVisibleID) return;
    navigate(`/${filterSlug(filter)}/${firstVisibleID}`, { replace: true });
  }, [runId, loading, firstVisibleID, filter, navigate]);

  function leaveSelection() {
    setSelectionMode(false);
    setSelectedIDs(new Set());
  }

  function toggleSelectionMode() {
    setSelectionMode((current) => !current);
    setSelectedIDs(new Set());
  }

  function toggleRecordSelection(id: string) {
    setSelectedIDs((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }

  function toggleAllVisible() {
    const allSelected = visibleRuns.length > 0 && visibleRuns.every((run) => selectedIDs.has(run.id));
    setSelectedIDs((current) => {
      const next = new Set(current);
      visibleRuns.forEach((run) => allSelected ? next.delete(run.id) : next.add(run.id));
      return next;
    });
  }

  async function publishSelection() {
    if (!bulkSelection.length) return;
    const result = await publishMany(bulkSelection);
    setSelectedIDs(new Set(result.failedIDs));
    if (!result.failedIDs.length) setSelectionMode(false);
  }

  function requestDelete(run: Run) {
    setPendingDelete({ kind: "single", run });
  }

  function requestBulkDelete() {
    if (!bulkSelection.length) return;
    setPendingDelete({ kind: "bulk", runs: bulkSelection });
  }

  async function confirmDelete() {
    const pending = pendingDelete;
    if (!pending) return;
    if (pending.kind === "single") {
      const { run } = pending;
      const deleted = await remove(run);
      setPendingDelete(null);
      // A deleted record cannot stay in the URL; the bare filter then selects whatever is left.
      if (deleted) navigate(`/${filterSlug(filter)}`, { replace: true });
      return;
    }
    const targets = pending.runs;
    const result = await removeMany(targets);
    setPendingDelete(null);
    setSelectedIDs(new Set(result.failedIDs));
    if (!result.failedIDs.length) setSelectionMode(false);
    if (runId && targets.some((run) => run.id === runId) && !result.failedIDs.includes(runId)) navigate(`/${filterSlug(filter)}`, { replace: true });
  }

  async function submitURL(url: string) {
    const run = await create(url);
    if (run) navigate(`/enriching/${run.id}`);
    return run;
  }

  async function act(action: RunAction) {
    if (!selected) return;
    const next = await update(selected, action);
    if (next) navigate(`/${filterSlug(next.state)}/${next.id}`);
  }

  async function addManualEvidence(input: ManualEvidenceInput) {
    if (!selected) return false;
    return Boolean(await addEvidence(selected, input));
  }

  async function deleteManualEvidence(evidenceID: string) {
    if (!selected) return;
    await removeEvidence(selected, evidenceID);
  }

  async function editEvidence(evidenceID: string, value: string) {
    if (!selected) return false;
    return Boolean(await replaceEvidence(selected, evidenceID, value));
  }

  function changeEvidence(evidenceID: string, action: "exclude" | "restore" | "choose") {
    if (!selected) return;
    if (action === "exclude") void excludeEvidence(selected, evidenceID);
    if (action === "restore") void restoreEvidence(selected, evidenceID);
    if (action === "choose") void chooseEvidence(selected, evidenceID);
  }

  async function savePhoto(input: PhotoOverride) {
    if (!selected) return false;
    return Boolean(await savePhotoOverride(selected, input));
  }

  async function clearPhoto() {
    if (!selected) return;
    await clearPhotoOverride(selected);
  }

  const deleteRequest = pendingDelete ? deleteCopy(pendingDelete) : null;
  const deletingPending = pendingDelete?.kind === "single" ? deletingID === pendingDelete.run.id : bulkBusy === "delete";

  return <main className="min-h-screen bg-paper text-ink">
    <AdminHeader connected={apiHealthy} contributor={contributor} onSignOut={onSignOut} />
    <div className="grid min-h-[calc(100vh-64px)] lg:grid-cols-[228px_minmax(0,1fr)]">
      <AdminSidebar runs={runs} active={filter} onNavigate={leaveSelection} />
      <section className="min-w-0">
        <IngestionToolbar submitting={submitting} notice={notice} error={reconnecting && error ? `${error} Retrying automatically…` : error} onSubmit={submitURL} />
        <div className="flex items-center gap-2 overflow-x-auto border-b border-ink/15 px-5 py-3 lg:hidden">{FILTERS.map(({ key, label }) => <Link key={key} to={`/${filterSlug(key)}`} onClick={leaveSelection} className={`focus-ring whitespace-nowrap rounded-md px-3 py-2 text-xs font-medium ${filter === key ? "bg-moss text-white" : "border border-ink/15 bg-white/60"}`}>{label}</Link>)}</div>
        <div className="grid min-h-[650px] xl:grid-cols-[390px_minmax(0,1fr)]">
          <RunQueue runs={visibleRuns} loading={loading} activeFilter={filter} selectedID={selected?.id || null} selectionMode={selectionMode} selectedIDs={selectedIDs} bulkBusy={bulkBusy} onToggleSelection={toggleRecordSelection} onToggleSelectionMode={toggleSelectionMode} onToggleAll={toggleAllVisible} onBulkPublish={() => void publishSelection()} onBulkDelete={requestBulkDelete} onRefresh={() => void load()} />
          <div className="min-w-0 bg-[#f7f5ee]"><ReviewPanel run={selected} deleting={deletingID === selected?.id} savingEvidence={savingEvidence} onAction={(action) => void act(action)} onDelete={() => { if (selected) requestDelete(selected); }} onAddEvidence={addManualEvidence} onRemoveEvidence={(id) => void deleteManualEvidence(id)} onReplaceEvidence={editEvidence} onEvidenceAction={changeEvidence} onSavePhoto={savePhoto} onClearPhoto={() => void clearPhoto()} /></div>
        </div>
      </section>
    </div>
    {deleteRequest ? <ConfirmDialog title={deleteRequest.title} description={deleteRequest.description} confirmLabel={deleteRequest.confirmLabel} busy={deletingPending} onConfirm={() => void confirmDelete()} onCancel={() => setPendingDelete(null)} /> : null}
  </main>;
}

function deleteCopy(pending: PendingDelete) {
  if (pending.kind === "single") return {
    title: `Delete “${pending.run.name}”?`,
    description: "This permanently removes the ingestion record and its extracted evidence.",
    confirmLabel: "Delete record",
  };
  const count = pending.runs.length;
  const published = pending.runs.filter((run) => run.state === "published").length;
  const publishedNote = published ? `\n\n${published} published ${published === 1 ? "record" : "records"} will also be removed from the public catalogue.` : "";
  return {
    title: `Permanently delete ${count} selected ${count === 1 ? "record" : "records"}?`,
    description: `This permanently removes their extracted evidence too.${publishedNote}`,
    confirmLabel: count === 1 ? "Delete record" : `Delete ${count} records`,
  };
}

export default App;
