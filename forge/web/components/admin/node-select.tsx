"use client";
import { useNodesQuery } from "@/lib/admin/telemetry";

import { useMemo } from "react";
import { Server } from "lucide-react";
import { type ApiNode } from "@/lib/api";
import { cn } from "@/lib/utils";

/**
 * @deprecated Host tools must not choose a node on the operator's behalf. The
 * control plane rejects an omitted node with 400 precisely so that a shell or a
 * prune can never land on a machine nobody named. Kept only while callers are
 * migrated; new code should render {@link NodeSelect} with an empty value.
 */
export function pickDefaultNode(nodes: ApiNode[]): string {
  const active = nodes.find((n) => n.status === "active");
  return (active ?? nodes[0])?.id ?? "";
}

/**
 * Explicit node picker for host-level tools (terminal, file manager, inspector).
 *
 * There is deliberately no default selection: the value stays empty until the
 * operator picks, so a privileged action can never resolve to "whatever node
 * happened to be first". `components/docker/node-select.tsx` enforces the same
 * rule for Docker create/pull.
 */
export function NodeSelect({
  value,
  onChange,
  className,
  label = "Target node",
}: {
  value: string;
  onChange: (nodeId: string) => void;
  className?: string;
  label?: string;
}) {
  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);

  if (nodesQuery.isLoading) {
    return <span className="text-xs text-text-subtle">Loading nodes…</span>;
  }

  if (nodesQuery.isError) {
    return (
      <span className="text-xs text-text-subtle" role="status">
        Node list could not be loaded. Retry from the refresh control before choosing a host.
      </span>
    );
  }

  if (nodes.length === 0) {
    return (
      <span className="text-xs text-text-subtle" role="status">
        No nodes are registered, so there is no host to target.
      </span>
    );
  }

  return (
    <label className={cn("inline-flex items-center gap-2", className)}>
      <Server aria-hidden="true" className="shrink-0 text-text-subtle" size={14} />
      <span className="text-xs font-semibold uppercase tracking-wider text-text-subtle">{label}</span>
      <select
        className="h-9 min-w-0 rounded-lg border border-line bg-[var(--surface-input)] px-2 text-xs text-text outline-none transition-all focus-visible:border-brand focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
        onChange={(event) => onChange(event.target.value)}
        value={value}
      >
        <option value="">Select a node…</option>
        {nodes.map((node) => (
          <option key={node.id} value={node.id}>
            {node.name}{node.status !== "active" ? ` (${node.status})` : ""}
          </option>
        ))}
      </select>
    </label>
  );
}
