import { Archive, Clock3, Inbox, LayoutDashboard, LoaderCircle, ShieldCheck, TriangleAlert } from "lucide-react";
import type { FilterKey } from "./types";

export const FILTERS: { key: FilterKey; label: string; icon: typeof LayoutDashboard }[] = [
  { key: "all", label: "All records", icon: LayoutDashboard },
  { key: "needs_review", label: "Needs review", icon: Inbox },
  { key: "enriching", label: "Processing", icon: LoaderCircle },
  { key: "failed", label: "Failed", icon: TriangleAlert },
  { key: "published", label: "Published", icon: ShieldCheck },
  { key: "stale", label: "Stale", icon: Clock3 },
  { key: "archived", label: "Archived", icon: Archive },
];

// The queue stages are the API's state names (needs_review); a URL spells them the way a person
// would read them (needs-review). Every route and every link goes through these two, so a filter
// only ever has one address and the set of valid ones is still exactly the keys above.
export function filterSlug(key: FilterKey) {
  return key.replaceAll("_", "-");
}

export function filterFromSlug(slug: string | undefined): FilterKey | null {
  const key = (slug || "").replaceAll("-", "_");
  return FILTERS.some((item) => item.key === key) ? key as FilterKey : null;
}
