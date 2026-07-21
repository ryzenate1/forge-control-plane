"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft, ExternalLink, GitPullRequest, Globe, Play, Trash2,
} from "lucide-react";
import { fetchJSON, postJSON } from "@/lib/api";
import { Btn, Card, CardHeader, Pill, SectionHeader } from "@/components/admin/admin-ui";
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

export default function AdminPreviewDeploymentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const id = params.id as string;

  const query = useQuery({
    queryKey: ["admin", "preview-deployments", id],
    queryFn: () => fetchJSON<PreviewDeployment>(`/admin/preview-deployments/${encodeURIComponent(id)}`),
  });

  const deployMutation = useMutation({
    mutationFn: () => postJSON(`/admin/preview-deployments/${encodeURIComponent(id)}/deploy`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "preview-deployments", id] }),
  });

  const cleanupMutation = useMutation({
    mutationFn: () => postJSON(`/admin/preview-deployments/${encodeURIComponent(id)}/cleanup`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "preview-deployments", id] }),
  });

  const p = query.data;

  if (query.isLoading) {
    return <div className="p-8 text-center text-sm text-slate-500">Loading preview deployment...</div>;
  }

  if (!p) {
    return <div className="p-8 text-center text-sm text-red-300">Preview deployment not found.</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-4">
        <Btn tone="ghost" onClick={() => router.push("/admin/preview-deployments")}>
          <ArrowLeft size={14} /> Back
        </Btn>
        <div className="flex-1">
          <SectionHeader
            title={`Preview: PR #${p.prNumber}`}
            sub={p.prTitle || `${p.repoOwner}/${p.repoName}`}
            action={
              <div className="flex gap-2">
                {p.prUrl && (
                  <a
                    href={p.prUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1.5 rounded-lg bg-white/[0.06] px-3 py-2 text-sm font-medium text-slate-200 hover:bg-white/[0.1]"
                  >
                    <ExternalLink size={14} /> Open PR
                  </a>
                )}
                {p.status === "deploying" && (
                  <Btn tone="primary" onClick={() => deployMutation.mutate()} disabled={deployMutation.isPending}>
                    <Play size={14} /> Deploy
                  </Btn>
                )}
                {p.status !== "cleaned_up" && (
                  <Btn tone="danger" onClick={() => cleanupMutation.mutate()} disabled={cleanupMutation.isPending}>
                    <Trash2 size={14} /> Cleanup
                  </Btn>
                )}
              </div>
            }
          />
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Card className="p-4">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1">
            <GitPullRequest size={12} /> PR Number
          </div>
          <p className="text-sm font-medium text-slate-200">#{p.prNumber}</p>
        </Card>
        <Card className="p-4">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1">
            <Globe size={12} /> Status
          </div>
          <Pill tone={
            p.status === "running" ? "green" :
            p.status === "failed" ? "red" :
            p.status === "deploying" ? "blue" :
            p.status === "cleaned_up" ? "neutral" : "yellow"
          }>
            {p.status.replace(/_/g, " ")}
          </Pill>
        </Card>
        <Card className="p-4">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1">
            <ExternalLink size={12} /> Preview URL
          </div>
          {p.previewUrl ? (
            <a href={p.previewUrl} target="_blank" rel="noopener noreferrer" className="text-sm text-blue-400 hover:text-blue-300">
              {p.previewUrl}
            </a>
          ) : (
            <p className="text-sm text-slate-400">—</p>
          )}
        </Card>
      </div>

      <Card>
        <CardHeader title="Details" icon={GitPullRequest} />
        <div className="grid grid-cols-2 gap-4 p-4 sm:grid-cols-4">
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Branch</p>
            <p className="mt-1 text-sm text-slate-200">{p.branch || "—"}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Commit</p>
            <p className="mt-1 font-mono text-sm text-slate-200">{p.commitSha ? p.commitSha.slice(0, 7) : "—"}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Repository</p>
            <p className="mt-1 text-sm text-slate-200">{p.repoOwner ? `${p.repoOwner}/${p.repoName}` : "—"}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Source</p>
            <Pill tone={p.source === "github" ? "neutral" : "blue"}>{p.source}</Pill>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Suffix</p>
            <p className="mt-1 font-mono text-sm text-slate-200">{p.uniqueSuffix || "—"}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Isolated</p>
            <p className="mt-1 text-sm text-slate-200">{p.isIsolated ? "Yes" : "No"}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Created</p>
            <p className="mt-1 text-sm text-slate-200">{formatDate(p.createdAt)}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Cleaned</p>
            <p className="mt-1 text-sm text-slate-200">{p.cleanedAt ? formatDate(p.cleanedAt) : "—"}</p>
          </div>
        </div>
      </Card>
    </div>
  );
}
