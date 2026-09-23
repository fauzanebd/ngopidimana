import type { Requirement } from "../types";

export function IntentChip({ requirement }: { requirement: Requirement }) {
  return <span className="intent-chip" title={`${Math.round(requirement.confidence * 100)}% interpretation confidence`}>
    <span className="h-1.5 w-1.5 rounded-full bg-olive" />
    {requirement.label}
  </span>;
}
