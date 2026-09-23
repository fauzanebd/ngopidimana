import type { Requirement } from "../types";

export function IntentChip({ requirement }: { requirement: Requirement }) {
  return <span className={`intent-chip ${requirement.kind === "hard" ? "intent-chip-hard" : ""}`} title={`${Math.round(requirement.confidence * 100)}% interpretation confidence`}>
    <span className={`h-1.5 w-1.5 rounded-full ${requirement.kind === "hard" ? "bg-butter" : "bg-olive"}`} />
    {requirement.label}
    {requirement.kind === "hard" ? <span className="intent-chip-qualifier ml-0.5 text-[9px] font-bold uppercase tracking-wider">must</span> : null}
  </span>;
}
