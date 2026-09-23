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
