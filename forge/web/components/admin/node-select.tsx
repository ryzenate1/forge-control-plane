"use client";

import { useEffect, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Server } from "lucide-react";
import { fetchNodes, type ApiNode } from "@/lib/api";
import { cn } from "@/lib/utils";

// Node picker for host-level tools (terminal, file manager). Mirrors the
// backend fallback: default to the first active node, else the first node.
export function pickDefaultNode(nodes: ApiNode[]): string {
  const active = nodes.find((n) => n.status === "active");
  return (active ?? nodes[0])?.id ?? "";
}

export function NodeSelect({
  value,
  onChange,
  className,
}: {
  value: string;
  onChange: (nodeId: string) => void;
  className?: string;
}) {
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);

  useEffect(() => {
    if (!value && nodes.length > 0) {
      onChange(pickDefaultNode(nodes));
    }
  }, [value, nodes, onChange]);

  if (nodesQuery.isLoading) {
    return <span className="text-xs text-slate-500">Loading nodes...</span>;
  }
  if (nodes.length === 0) {
    return <span className="text-xs text-amber-400">No nodes registered</span>;
  }

  return (
    <label className={cn("inline-flex items-center gap-2", className)}>
      <Server className="shrink-0 text-slate-500" size={14} />
      <span className="sr-only">Target node</span>
      <select
        className="h-9 rounded-lg border border-white/10 bg-[#0d131d] px-2 text-xs text-white outline-none focus:border-red-400/70 transition-all"
        onChange={(event) => onChange(event.target.value)}
        value={value}
      >
        {nodes.map((node) => (
          <option key={node.id} value={node.id}>
            {node.name}{node.status !== "active" ? ` (${node.status})` : ""}
          </option>
        ))}
      </select>
    </label>
  );
}
