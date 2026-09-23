import type { GooglePlace, ManualEvidenceInput, Run, RunAction } from "../types";

async function parse<T>(response: Response): Promise<T> {
  const body = await response.json();
  if (!response.ok) throw new Error(body.error || "The ingestion API request failed");
  return body;
}

export async function listRuns(): Promise<Run[]> {
  const body = await parse<{ runs: Run[] }>(await fetch("/v1/admin/ingestion-runs"));
  return body.runs || [];
}

export async function createRun(url: string): Promise<Run> {
  return parse(await fetch("/v1/admin/ingestion-runs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  }));
}

export async function updateRun(id: string, action: RunAction): Promise<Run> {
  return parse(await fetch(`/v1/admin/ingestion-runs/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action }),
  }));
}

export async function deleteRun(id: string): Promise<void> {
  const response = await fetch(`/v1/admin/ingestion-runs/${id}`, { method: "DELETE" });
  if (response.ok) return;
  const body = await response.json().catch(() => ({ error: "The ingestion API request failed" })) as { error?: string };
  throw new Error(body.error || "Could not delete the ingestion record");
}

export async function getGooglePlace(id: string): Promise<GooglePlace> {
  return parse(await fetch(`/v1/admin/ingestion-runs/${id}/google-place`, {
    cache: "no-store",
  }));
}

export async function addManualEvidence(id: string, input: ManualEvidenceInput): Promise<Run> {
  return parse(await fetch(`/v1/admin/ingestion-runs/${id}/manual-evidence`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  }));
}

export async function removeManualEvidence(id: string, evidenceID: string): Promise<Run> {
  return parse(await fetch(`/v1/admin/ingestion-runs/${id}/manual-evidence/${encodeURIComponent(evidenceID)}`, {
    method: "DELETE",
  }));
}

export async function replaceEvidence(id: string, evidenceID: string, value: string): Promise<Run> {
  return parse(await fetch(`/v1/admin/ingestion-runs/${id}/evidence/${encodeURIComponent(evidenceID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ value }),
  }));
}

export async function excludeEvidence(id: string, evidenceID: string): Promise<Run> {
  return parse(await fetch(`/v1/admin/ingestion-runs/${id}/evidence/${encodeURIComponent(evidenceID)}`, { method: "DELETE" }));
}

export async function restoreEvidence(id: string, evidenceID: string): Promise<Run> {
  return parse(await fetch(`/v1/admin/ingestion-runs/${id}/evidence/${encodeURIComponent(evidenceID)}/restore`, { method: "POST" }));
}

export async function chooseEvidence(id: string, evidenceID: string): Promise<Run> {
  return parse(await fetch(`/v1/admin/ingestion-runs/${id}/evidence/${encodeURIComponent(evidenceID)}/choose`, { method: "POST" }));
}
