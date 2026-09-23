import { useCallback, useEffect, useState } from "react";
import * as ingestionAPI from "../api/ingestion";
import type { ManualEvidenceInput, Run, RunAction } from "../types";

export function useIngestionRuns() {
  const [runs, setRuns] = useState<Run[]>([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [deletingID, setDeletingID] = useState<string | null>(null);
  const [savingEvidence, setSavingEvidence] = useState(false);
  const [bulkBusy, setBulkBusy] = useState<"publish" | "delete" | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      setRuns(await ingestionAPI.listRuns());
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not load the review queue");
    } finally {
      if (!silent) setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    if (!runs.some((run) => run.state === "enriching")) return;
    const timer = window.setInterval(() => void load(true), 1500);
    return () => window.clearInterval(timer);
  }, [runs, load]);

  async function create(url: string) {
    setSubmitting(true); setError(""); setNotice("");
    try {
      const run = await ingestionAPI.createRun(url);
      setRuns((current) => [run, ...current]);
      setNotice("Ingestion queued. The worker will attach evidence before review.");
      return run;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not start ingestion");
      return null;
    } finally { setSubmitting(false); }
  }

  async function update(run: Run, action: RunAction) {
    setError(""); setNotice("");
    try {
      const next = await ingestionAPI.updateRun(run.id, action);
      setRuns((current) => current.map((item) => item.id === next.id ? next : item));
      setNotice(action === "publish" ? `${next.name} is published.` : `Record moved to ${next.state.replace("_", " ")}.`);
      return next;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not update record");
      return null;
    }
  }

  async function remove(run: Run) {
    setDeletingID(run.id); setError(""); setNotice("");
    try {
      await ingestionAPI.deleteRun(run.id);
      setRuns((current) => current.filter((item) => item.id !== run.id));
      setNotice(`${run.name} was permanently deleted.`);
      return true;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not delete record");
      return false;
    } finally { setDeletingID(null); }
  }

  async function publishMany(selected: Run[]) {
    const targets = selected.filter((run) => run.state !== "published");
    const skipped = selected.length - targets.length;
    setBulkBusy("publish"); setError(""); setNotice("");
    try {
      const results = await Promise.all(targets.map(async (run) => {
        try {
          return { run: await ingestionAPI.updateRun(run.id, "publish"), failed: false as const, id: run.id, name: run.name, message: "" };
        } catch (cause) {
          return { run: null, failed: true as const, id: run.id, name: run.name, message: cause instanceof Error ? cause.message : "Could not publish record" };
        }
      }));
      const published = results.flatMap((result) => result.run ? [result.run] : []);
      const failed = results.filter((result) => result.failed);
      if (published.length) {
        const byID = new Map(published.map((run) => [run.id, run]));
        setRuns((current) => current.map((run) => byID.get(run.id) || run));
      }
      const summary = [`Published ${published.length} ${published.length === 1 ? "record" : "records"}.`];
      if (skipped) summary.push(`Skipped ${skipped} already published.`);
      if (!targets.length) summary[0] = "All selected records were already published; nothing changed.";
      setNotice(summary.join(" "));
      if (failed.length) setError(`${failed.length} could not be published: ${failed.map((item) => `${item.name} — ${item.message}`).join("; ")}`);
      return { failedIDs: failed.map((item) => item.id) };
    } finally {
      setBulkBusy(null);
    }
  }

  async function removeMany(selected: Run[]) {
    setBulkBusy("delete"); setError(""); setNotice("");
    try {
      const results = await Promise.all(selected.map(async (run) => {
        try {
          await ingestionAPI.deleteRun(run.id);
          return { failed: false as const, id: run.id, name: run.name, message: "" };
        } catch (cause) {
          return { failed: true as const, id: run.id, name: run.name, message: cause instanceof Error ? cause.message : "Could not delete record" };
        }
      }));
      const deletedIDs = new Set(results.filter((result) => !result.failed).map((result) => result.id));
      const failed = results.filter((result) => result.failed);
      setRuns((current) => current.filter((run) => !deletedIDs.has(run.id)));
      setNotice(`Permanently deleted ${deletedIDs.size} ${deletedIDs.size === 1 ? "record" : "records"}.`);
      if (failed.length) setError(`${failed.length} could not be deleted: ${failed.map((item) => `${item.name} — ${item.message}`).join("; ")}`);
      return { failedIDs: failed.map((item) => item.id) };
    } finally {
      setBulkBusy(null);
    }
  }

  async function addEvidence(run: Run, input: ManualEvidenceInput) {
    setSavingEvidence(true); setError(""); setNotice("");
    try {
      const next = await ingestionAPI.addManualEvidence(run.id, input);
      setRuns((current) => current.map((item) => item.id === next.id ? next : item));
      setNotice(`${input.key} was added as manually sourced evidence.`);
      return next;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not add manual evidence");
      return null;
    } finally { setSavingEvidence(false); }
  }

  async function removeEvidence(run: Run, evidenceID: string) {
    setSavingEvidence(true); setError(""); setNotice("");
    try {
      const next = await ingestionAPI.removeManualEvidence(run.id, evidenceID);
      setRuns((current) => current.map((item) => item.id === next.id ? next : item));
      setNotice("Manual evidence was removed.");
      return next;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not remove manual evidence");
      return null;
    } finally { setSavingEvidence(false); }
  }

  async function mutateEvidence(run: Run, request: Promise<Run>, success: string) {
    setSavingEvidence(true); setError(""); setNotice("");
    try {
      const next = await request;
      setRuns((current) => current.map((item) => item.id === next.id ? next : item));
      setNotice(success);
      return next;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not update evidence");
      return null;
    } finally { setSavingEvidence(false); }
  }

  function replaceEvidence(run: Run, evidenceID: string, value: string) {
    return mutateEvidence(run, ingestionAPI.replaceEvidence(run.id, evidenceID, value), "A manual correction replaced the sourced value.");
  }

  function excludeEvidence(run: Run, evidenceID: string) {
    return mutateEvidence(run, ingestionAPI.excludeEvidence(run.id, evidenceID), "Evidence was excluded from catalogue facts and matching.");
  }

  function restoreEvidence(run: Run, evidenceID: string) {
    return mutateEvidence(run, ingestionAPI.restoreEvidence(run.id, evidenceID), "Evidence was restored.");
  }

  function chooseEvidence(run: Run, evidenceID: string) {
    return mutateEvidence(run, ingestionAPI.chooseEvidence(run.id, evidenceID), "Conflict resolved using the selected value.");
  }

  return { runs, loading, submitting, deletingID, savingEvidence, bulkBusy, notice, error, load, create, update, remove, publishMany, removeMany, addEvidence, removeEvidence, replaceEvidence, excludeEvidence, restoreEvidence, chooseEvidence };
}
