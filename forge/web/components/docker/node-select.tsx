"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchAllNodes } from "@/lib/api";

/**
 * Explicit node picker for Docker create/pull operations. Mutating Docker
 * endpoints require `?node=` (or `nodeId`) since the API no longer defaults
 * to the first listed node — an ambiguous target is rejected with 400.
 */
export function NodeSelect({
  value,
  onChange,
  label = "Node *",
}: {
  value: string;
  onChange: (nodeId: string) => void;
  label?: string;
}) {
  const nodesQuery = useQuery({
    queryKey: ["nodes", "all"],
    queryFn: fetchAllNodes,
    staleTime: 30_000,
    retry: false,
  });
  const nodes = nodesQuery.data ?? [];

  return (
    <div>
      <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">{label}</label>
      <select
        className="ui-input"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={nodesQuery.isPending}
      >
        <option value="">{nodesQuery.isPending ? "Loading nodes…" : "Select a node…"}</option>
        {nodes.map((n) => (
          <option key={n.id} value={n.id}>
            {n.name || n.id}
          </option>
        ))}
      </select>
      {nodesQuery.isError && (
        <p className="mt-1 text-xs text-red-400">Failed to load nodes.</p>
      )}
    </div>
  );
}
