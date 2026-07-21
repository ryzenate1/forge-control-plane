"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ExternalLink, GitPullRequest, Globe, Play, Trash2,
} from "lucide-react";
import { fetchJSON, postJSON } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";
import { formatDate } from "@/lib/utils";

type PreviewDeployment = {
  id: string;
  serverId: string;
  serviceId?: string;
  prNumber: number;
  prTitle?: string;
  prUrl?: string;
  branch?: string;
  repoOwner?: string;
  repoName?: string;
  commitSha?: string;
  status: "deploying" | "running" | "stopped" | "failed" | "cleaned_up";
  previewUrl?: string;
  deploymentUrl?: string;
  source: "github" | "gitlab";
  uniqueSuffix?: string;
  isIsolated: boolean;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
  cleanedAt?: string;
};

const statusConfig: Record<string, { tone: "green" | "yellow" | "red" | "blue" | "neutral" }> = {
  deploying: { tone: "blue" },
  running: { tone: "green" },
  stopped: { tone: "yellow" },
  failed: { tone: "red" },
  cleaned_up: { tone: "neutral" },
};

export default function AdminPreviewDeploymentsPage() {
  const queryClient = useQueryClient();
  const [statusFilter, setStatusFilter] = useState<string>("");

  const previewsQuery = useQuery({
    queryKey: ["admin", "preview-deployments"],
    queryFn: () => fetchJSON<PreviewDeployment[]>("/admin/preview-deployments"),
    refetchInterval: 15_000,
  });

  const deployMutation = useMutation({
    mutationFn: (id: string) => postJSON(`/admin/preview-deployments/${encodeURIComponent(id)}/deploy`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "preview-deployments"] }),
  });

  const cleanupMutation = useMutation({
    mutationFn: (id: string) => postJSON(`/admin/preview-deployments/${encodeURIComponent(id)}/cleanup`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "preview-deployments"] }),
  });

  const previews = useMemo(() => previewsQuery.data ?? [], [previewsQuery.data]);

  const filtered = useMemo(() => {
    if (!statusFilter) return previews;
    return previews.filter((p) => p.status === statusFilter);
  }, [previews, statusFilter]);

  const statuses = ["deploying", "running", "stopped", "failed", "cleaned_up"];

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Preview Deployments"
        sub="PR-based preview environments for your applications."
      />

      <Card>
        <CardHeader title={`${filtered.length.toLocaleString()} preview${filtered.length === 1 ? "" : "s"}`} icon={GitPullRequest} />
        <div className="flex flex-wrap items-center gap-3 p-4">
          <select
            className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-300 outline-none"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
          >
            <option value="">All Statuses</option>
            {statuses.map((s) => (
              <option key={s} value={s}>{s.replace(/_/g, " ")}</option>
            ))}
          </select>
        </div>

        {previewsQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading preview deployments...</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={GitPullRequest} message="No preview deployments found." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">PR</th>
                  <th className="px-4 py-3">Title</th>
                  <th className="px-4 py-3">Branch</th>
                  <th className="px-4 py-3">Source</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Preview URL</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filtered.map((p) => {
                  const cfg = statusConfig[p.status] ?? statusConfig.deploying;
                  return (
                    <tr key={p.id} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs font-medium text-slate-200">#{p.prNumber}</td>
                      <td className="px-4 py-3 text-xs text-slate-300 max-w-[200px] truncate">{p.prTitle || "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{p.branch || "—"}</td>
                      <td className="px-4 py-3">
                        <Pill tone={p.source === "github" ? "neutral" : "blue"}>{p.source}</Pill>
                      </td>
                      <td className="px-4 py-3">
                        <Pill tone={cfg.tone}>{p.status.replace(/_/g, " ")}</Pill>
                      </td>
                      <td className="px-4 py-3">
                        {p.previewUrl ? (
                          <a
                            href={p.previewUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center gap-1 text-xs text-blue-400 hover:text-blue-300"
                          >
                            <ExternalLink size={12} /> View
                          </a>
                        ) : "—"}
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-500">{formatDate(p.createdAt)}</td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1">
                          {p.status === "deploying" && (
                            <Btn size="sm" tone="primary" onClick={() => deployMutation.mutate(p.id)} disabled={deployMutation.isPending}>
                              <Play size={12} /> Deploy
                            </Btn>
                          )}
                          {p.status !== "cleaned_up" && (
                            <Btn size="sm" tone="danger" onClick={() => cleanupMutation.mutate(p.id)} disabled={cleanupMutation.isPending}>
                              <Trash2 size={12} /> Cleanup
                            </Btn>
                          )}
                          {p.prUrl && (
                            <a
                              href={p.prUrl}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="inline-flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-medium text-slate-400 hover:text-slate-200"
                            >
                              <ExternalLink size={12} /> PR
                            </a>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
