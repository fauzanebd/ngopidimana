import { Coffee, LoaderCircle } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { AdminHeader } from "./components/AdminHeader";
import { AdminSidebar } from "./components/AdminSidebar";
import { IngestionToolbar } from "./components/IngestionToolbar";
import { ReviewPanel } from "./components/ReviewPanel";
import { RunQueue } from "./components/RunQueue";
import { SignInPanel } from "./components/SignInPanel";
import { FILTERS } from "./constants";
import { useApiHealth } from "./hooks/useApiHealth";
import { useIngestionRuns } from "./hooks/useIngestionRuns";
import { useSession } from "./hooks/useSession";
import type { Contributor, FilterKey, ManualEvidenceInput, RunAction } from "./types";

function App({ callbackToken = null }: { callbackToken?: string | null }) {
  const session = useSession(callbackToken);

  if (session.status === "checking") return <main className="grid min-h-screen place-items-center bg-paper text-ink">
    <div className="flex flex-col items-center gap-4"><div className="grid h-11 w-11 place-items-center rounded-xl bg-moss text-white"><Coffee className="h-6 w-6" /></div><p className="flex items-center gap-2 text-xs text-ink/55"><LoaderCircle className="h-3.5 w-3.5 animate-spin" /> {callbackToken === null ? "Checking your session…" : "Signing you in…"}</p></div>
  </main>;

  if (session.status === "signed-out" || !session.contributor) return <SignInPanel signingIn={session.signingIn} error={session.signInError} sentTo={session.sentTo} onSignIn={session.signIn} onUseAnotherAddress={session.clearSentTo} />;

  return <Dashboard contributor={session.contributor} onSignOut={session.signOut} />;
}

function Dashboard({ contributor, onSignOut }: { contributor: Contributor; onSignOut: () => Promise<void> }) {
  const { runs, loading, reconnecting, submitting, deletingID, savingEvidence, bulkBusy, notice, error, load, create, update, remove, publishMany, removeMany, addEvidence, removeEvidence, replaceEvidence, excludeEvidence, restoreEvidence, chooseEvidence } = useIngestionRuns(() => void onSignOut());
  const apiHealthy = useApiHealth();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("needs_review");
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [selectionMode, setSelectionMode] = useState(false);
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(() => new Set());
  const visibleRuns = useMemo(() => activeFilter === "all" ? runs : runs.filter((run) => run.state === activeFilter), [activeFilter, runs]);
  const selected = runs.find((run) => run.id === selectedID) || visibleRuns[0] || null;
  const bulkSelection = useMemo(() => runs.filter((run) => selectedIDs.has(run.id)), [runs, selectedIDs]);

  useEffect(() => {
    const currentIDs = new Set(runs.map((run) => run.id));
    setSelectedIDs((current) => {
      const next = new Set([...current].filter((id) => currentIDs.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [runs]);

  function changeFilter(filter: FilterKey) {
    setActiveFilter(filter);
    setSelectedID(null);
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

  async function deleteSelection() {
    if (!bulkSelection.length) return;
    const published = bulkSelection.filter((run) => run.state === "published").length;
    const publishedNote = published ? `\n\n${published} published ${published === 1 ? "record" : "records"} will also be removed from the public catalogue.` : "";
    if (!window.confirm(`Permanently delete ${bulkSelection.length} selected ${bulkSelection.length === 1 ? "record" : "records"}?${publishedNote}`)) return;
    const deletedSelectedRecord = selected ? selectedIDs.has(selected.id) : false;
    const result = await removeMany(bulkSelection);
    setSelectedIDs(new Set(result.failedIDs));
    if (deletedSelectedRecord && !result.failedIDs.includes(selected!.id)) setSelectedID(null);
    if (!result.failedIDs.length) setSelectionMode(false);
  }

  async function submitURL(url: string) {
    const run = await create(url);
    if (run) {
      setSelectedID(run.id);
      setActiveFilter("enriching");
    }
    return run;
  }

  async function act(action: RunAction) {
    if (!selected) return;
    const next = await update(selected, action);
    if (next) setActiveFilter(next.state);
  }

  async function deleteSelected() {
    if (!selected) return;
    if (await remove(selected)) setSelectedID(null);
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

  return <main className="min-h-screen bg-paper text-ink">
    <AdminHeader connected={apiHealthy} contributor={contributor} onSignOut={onSignOut} />
    <div className="grid min-h-[calc(100vh-64px)] lg:grid-cols-[228px_minmax(0,1fr)]">
      <AdminSidebar runs={runs} active={activeFilter} onChange={changeFilter} />
      <section className="min-w-0">
        <IngestionToolbar submitting={submitting} notice={notice} error={reconnecting && error ? `${error} Retrying automatically…` : error} onSubmit={submitURL} />
        <div className="flex items-center gap-2 overflow-x-auto border-b border-ink/15 px-5 py-3 lg:hidden">{FILTERS.map(({ key, label }) => <button key={key} onClick={() => changeFilter(key)} className={`whitespace-nowrap rounded-md px-3 py-2 text-xs font-medium ${activeFilter === key ? "bg-moss text-white" : "border border-ink/15 bg-white/60"}`}>{label}</button>)}</div>
        <div className="grid min-h-[650px] xl:grid-cols-[390px_minmax(0,1fr)]">
          <RunQueue runs={visibleRuns} loading={loading} activeFilter={activeFilter} selectedID={selected?.id || null} selectionMode={selectionMode} selectedIDs={selectedIDs} bulkBusy={bulkBusy} onSelect={setSelectedID} onToggleSelection={toggleRecordSelection} onToggleSelectionMode={toggleSelectionMode} onToggleAll={toggleAllVisible} onBulkPublish={() => void publishSelection()} onBulkDelete={() => void deleteSelection()} onRefresh={() => void load()} />
          <div className="min-w-0 bg-[#f7f5ee]"><ReviewPanel run={selected} deleting={deletingID === selected?.id} savingEvidence={savingEvidence} onAction={(action) => void act(action)} onDelete={() => void deleteSelected()} onAddEvidence={addManualEvidence} onRemoveEvidence={(id) => void deleteManualEvidence(id)} onReplaceEvidence={editEvidence} onEvidenceAction={changeEvidence} /></div>
        </div>
      </section>
    </div>
  </main>;
}

export default App;
