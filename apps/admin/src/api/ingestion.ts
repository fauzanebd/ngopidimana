import { request, type ApiRequestInit } from "./client";
import type { GooglePlace, ManualEvidenceInput, Run, RunAction } from "../types";

const INGESTION_ERROR = "The ingestion API request failed";

function ingest<T>(path: string, init: ApiRequestInit = {}): Promise<T> {
  return request<T>(path, { errorMessage: INGESTION_ERROR, ...init });
}

export async function listRuns(): Promise<Run[]> {
  const body = await ingest<{ runs: Run[] }>("/v1/admin/ingestion-runs");
  return body.runs || [];
}

export async function createRun(url: string): Promise<Run> {
  return ingest<Run>("/v1/admin/ingestion-runs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
}

export async function updateRun(id: string, action: RunAction): Promise<Run> {
  return ingest<Run>(`/v1/admin/ingestion-runs/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action }),
  });
}

export async function deleteRun(id: string): Promise<void> {
  await ingest<void>(`/v1/admin/ingestion-runs/${id}`, { method: "DELETE", errorMessage: "Could not delete the ingestion record" });
}

export async function getGooglePlace(id: string): Promise<GooglePlace> {
  return ingest<GooglePlace>(`/v1/admin/ingestion-runs/${id}/google-place`, {
    cache: "no-store",
  });
}

export async function addManualEvidence(id: string, input: ManualEvidenceInput): Promise<Run> {
  return ingest<Run>(`/v1/admin/ingestion-runs/${id}/manual-evidence`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function removeManualEvidence(id: string, evidenceID: string): Promise<Run> {
  return ingest<Run>(`/v1/admin/ingestion-runs/${id}/manual-evidence/${encodeURIComponent(evidenceID)}`, {
    method: "DELETE",
  });
}

export async function replaceEvidence(id: string, evidenceID: string, value: string): Promise<Run> {
  return ingest<Run>(`/v1/admin/ingestion-runs/${id}/evidence/${encodeURIComponent(evidenceID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ value }),
  });
}

export async function excludeEvidence(id: string, evidenceID: string): Promise<Run> {
  return ingest<Run>(`/v1/admin/ingestion-runs/${id}/evidence/${encodeURIComponent(evidenceID)}`, { method: "DELETE" });
}

export async function restoreEvidence(id: string, evidenceID: string): Promise<Run> {
  return ingest<Run>(`/v1/admin/ingestion-runs/${id}/evidence/${encodeURIComponent(evidenceID)}/restore`, { method: "POST" });
}

export async function chooseEvidence(id: string, evidenceID: string): Promise<Run> {
  return ingest<Run>(`/v1/admin/ingestion-runs/${id}/evidence/${encodeURIComponent(evidenceID)}/choose`, { method: "POST" });
}
