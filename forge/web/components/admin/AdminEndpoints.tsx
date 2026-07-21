"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Box, Plus, Trash2, ExternalLink } from "lucide-react";
import { ApiError, createEndpoint, deleteEndpoint, fetchEndpoints } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import {
  Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, PermissionDeniedState, Pill, SectionHeader,
} from "./admin-ui";
import Link from "next/link";

const EP_TYPE_COLORS: Record<string, string> = {
  docker: "bg-blue-500/10 text-blue-400",
  swarm: "bg-orange-500/10 text-orange-400",
  kubernetes: "bg-purple-500/10 text-purple-400",
  edge: "bg-green-500/10 text-green-400",
};

const STATUS_COLORS: Record<string, string> = {
  online: "bg-green-500/10 text-green-400",
  degraded: "bg-yellow-500/10 text-yellow-400",
  offline: "bg-red-500/10 text-red-400",
  unknown: "bg-slate-500/10 text-slate-400",
  provisioning: "bg-cyan-500/10 text-cyan-400",
};

export function AdminEndpoints() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const endpointsQuery = useQuery({ queryKey: ["infra-endpoints"], queryFn: fetchEndpoints });
  const endpoints = endpointsQuery.data ?? [];

  const [modal, setModal] = useState<null | "create" | { id: string }>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [epType, setEpType] = useState("docker");
  const [connMode, setConnMode] = useState("direct");
  const [url, setUrl] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const createMut = useMutation({
    mutationFn: () =>
      createEndpoint({
        name: name.trim(),
        description: description.trim(),
        endpointType: epType,
        connectionMode: connMode,
        url: url.trim() || undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["infra-endpoints"] });
      setModal(null);
      setName("");
      setDescription("");
      setEpType("docker");
      setConnMode("direct");
      setUrl("");
      setFormError(null);
    },
    onError: (e: Error) => {
      setFormError(e.message || "Failed to create endpoint");
    },
  });

  const deleteMut = useMutation({
    mutationFn: deleteEndpoint,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["infra-endpoints"] }),
    onError: (e: Error) => toast({ tone: "error", title: "Failed to delete endpoint", message: e.message }),
  });

  return (
    <div>
      <SectionHeader
        title="Infrastructure Endpoints"
        sub="Logical groupings over nodes (Portainer-style environment abstraction)."
        action={<Btn onClick={() => { setName(""); setDescription(""); setEpType("docker"); setConnMode("direct"); setUrl(""); setFormError(null); setModal("create"); }}><Plus size={14} /> New Endpoint</Btn>}
      />

      <Card>
        <CardHeader title="All endpoints" icon={Box} />
        {endpointsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading</div>
        ) : endpointsQuery.isError ? (
          endpointsQuery.error instanceof ApiError && endpointsQuery.error.status === 403 ? (
            <div className="p-4">
              <PermissionDeniedState />
            </div>
          ) : (
          <div className="p-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load endpoints: {endpointsQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void endpointsQuery.refetch()}>Retry</Btn>
            </div>
          </div>
          )
        ) : endpoints.length === 0 ? (
          <EmptyState icon={Box} title="No endpoints yet" message="Create your first infrastructure endpoint to group nodes." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-700/50 text-left text-xs uppercase tracking-wider text-slate-500">
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Type</th>
                <th className="px-4 py-3 font-medium">Connection</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Reachable</th>
                <th className="px-4 py-3 font-medium">Version</th>
                <th className="px-4 py-3 font-medium w-20"></th>
              </tr>
            </thead>
            <tbody>
              {endpoints.map((ep) => (
                <tr key={ep.id} className="border-b border-slate-800/50 transition-colors hover:bg-slate-800/30">
                  <td className="px-4 py-3">
                    <Link href={`/admin/endpoints/${ep.id}`} className="font-medium text-white hover:text-blue-400 transition-colors">
                      {ep.name}
                      <ExternalLink size={12} className="inline ml-1 opacity-40" />
                    </Link>
                    {ep.description && <div className="text-xs text-slate-500 mt-0.5">{ep.description}</div>}
                  </td>
                  <td className="px-4 py-3">
                    <Pill className={EP_TYPE_COLORS[ep.endpointType] ?? ""}>{ep.endpointType}</Pill>
                  </td>
                  <td className="px-4 py-3 text-slate-300">{ep.connectionMode}</td>
                  <td className="px-4 py-3">
                    <Pill className={STATUS_COLORS[ep.status] ?? ""}>{ep.status}</Pill>
                  </td>
                  <td className="px-4 py-3">
                    {ep.reachable ? (
                      <span className="text-green-400">Yes</span>
                    ) : (
                      <span className="text-red-400">No</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-slate-400">{ep.version || "-"}</td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => {
                        if (window.confirm(`Delete endpoint "${ep.name}"?`)) {
                          deleteMut.mutate(ep.id);
                        }
                      }}
                      className="rounded p-1 text-slate-500 hover:bg-red-500/10 hover:text-red-400 transition-colors"
                      title="Delete"
                    >
                      <Trash2 size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {modal === "create" && (
        <Modal title="New Endpoint" onClose={() => setModal(null)}>
          {formError && (
            <div className="mb-3 flex items-center gap-2 rounded-md border border-red-500/20 bg-red-500/5 p-2.5 text-sm text-red-400">
              <AlertCircle size={14} /> {formError}
            </div>
          )}
          <div className="space-y-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-400">Name *</label>
              <Input value={name} onChange={setName} placeholder="Production Cluster" />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-400">Description</label>
              <Input value={description} onChange={setDescription} placeholder="Primary production environment" />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="mb-1 block text-xs font-medium text-slate-400">Type</label>
                <select value={epType} onChange={(e) => setEpType(e.target.value)} className="h-9 w-full rounded border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100">
                  <option value="docker">Docker</option>
                  <option value="swarm">Swarm</option>
                  <option value="kubernetes">Kubernetes</option>
                  <option value="edge">Edge</option>
                </select>
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-slate-400">Connection Mode</label>
                <select value={connMode} onChange={(e) => setConnMode(e.target.value)} className="h-9 w-full rounded border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100">
                  <option value="direct">Direct</option>
                  <option value="tunnel">Tunnel</option>
                  <option value="edge">Edge</option>
                </select>
              </div>
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-400">URL</label>
              <Input value={url} onChange={setUrl} placeholder="https://docker.example.com:2375" />
            </div>
          </div>
          <ModalFooter
            onCancel={() => setModal(null)}
            onConfirm={() => createMut.mutate()}
            confirmLabel="Create"
            disabled={createMut.isPending}
          />
        </Modal>
      )}
    </div>
  );
}
