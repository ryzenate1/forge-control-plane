"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertCircle, ArrowRightLeft, CheckCircle2, Loader2, Network, XCircle,
} from "lucide-react";
import { useToast } from "@/components/ui/toast";
import {
  type ApiAllocation, type ApiNode, type ApiServer,
  fetchNodeAllocations, fetchNodes,
} from "@/lib/api";
import { cancelServerTransfer, fetchServerTransferStatus, transferServer } from "@/lib/api/servers";
import { errorMessage as message } from "@/lib/utils";
import { hasServerPermission, useOptionalServerContext } from "./server-context";

const field = "mt-1 w-full rounded-lg border border-white/10 bg-[#111722] px-3 py-2 text-sm text-white outline-none focus:border-red-500 disabled:opacity-60";

export function TransferView({ server }: { server: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: [], isAdmin: true, isOwner: true };
  const canTransfer = hasServerPermission(access, "settings.reinstall");
  const qc = useQueryClient();
  const { toast } = useToast();

  const [targetNodeId, setTargetNodeId] = useState(server.transferTargetNodeId ?? "");
  const [primaryAllocationId, setPrimaryAllocationId] = useState("");

  const transferStatusQuery = useQuery({
    queryKey: ["server-transfer", server.id],
    queryFn: () => fetchServerTransferStatus(server.id),
    retry: false,
    refetchInterval: server.transferring ? 5000 : false,
  });
  const transfer = transferStatusQuery.data;

  const { data: nodes } = useQuery({
    queryKey: ["nodes"],
    queryFn: fetchNodes,
  });

  const { data: nodeAllocations } = useQuery({
    queryKey: ["node-allocations", targetNodeId],
    queryFn: () => fetchNodeAllocations(targetNodeId),
    enabled: !!targetNodeId && canTransfer,
  });

  const targetAllocations = (nodeAllocations ?? []).filter(
    (a: ApiAllocation) => !a.server,
  );

  const startMut = useMutation({
    mutationFn: () => transferServer(server.id, targetNodeId),
    onSuccess: () => {
      void transferStatusQuery.refetch();
      void qc.invalidateQueries({ queryKey: ["server", server.id] });
    },
    onError: (error) =>
      toast({ tone: "error", title: "Transfer failed", message: error instanceof Error ? error.message : "Could not transfer server" }),
  });

  const cancelMut = useMutation({
    mutationFn: () => cancelServerTransfer(server.id),
    onSuccess: () => {
      void transferStatusQuery.refetch();
      void qc.invalidateQueries({ queryKey: ["server", server.id] });
    },
    onError: (error) =>
      toast({ tone: "error", title: "Cancel failed", message: error instanceof Error ? error.message : "Could not cancel transfer" }),
  });

  const isTransferring = server.transferring;
  const phase = server.transferState ?? null;
  const errorMessage = server.transferError ?? null;
  const progress = transfer?.progress ?? null;

  const phaseLabel = phase
    ? phase.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())
    : null;

  return (
    <div className="grid gap-5">
      {/* Transfer status card */}
      <section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-5">
        <h2 className="flex items-center gap-2 font-bold text-white">
          <ArrowRightLeft size={18} />
          Server transfer
        </h2>

        {isTransferring ? (
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-3 rounded-lg border border-sky-500/20 bg-sky-500/10 p-4">
              <Loader2 className="h-5 w-5 shrink-0 animate-spin text-sky-300" />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold text-sky-200">Transfer in progress</p>
                {phaseLabel ? (
                  <p className="mt-0.5 text-xs text-sky-300">{phaseLabel}</p>
                ) : null}
              </div>
            </div>

            {progress !== null && (
              <div>
                <div className="flex items-center justify-between text-xs text-slate-400">
                  <span>Progress</span>
                  <span>{Math.round(progress)}%</span>
                </div>
                <div className="mt-1 h-2 overflow-hidden rounded-full bg-[#0a0e16]">
                  <div
                    className="h-full rounded-full bg-sky-500 transition-all"
                    style={{ width: `${Math.min(100, Math.max(0, progress))}%` }}
                  />
                </div>
              </div>
            )}

            {errorMessage ? (
              <p className="flex items-center gap-2 text-sm text-red-300" role="alert">
                <AlertCircle size={14} />{errorMessage}
              </p>
            ) : null}

            {canTransfer ? (
              <button
                className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-bold text-white disabled:opacity-40"
                disabled={cancelMut.isPending}
                onClick={() => {
                  if (window.confirm("Cancel this server transfer? This will abort the ongoing transfer."))
                    cancelMut.mutate();
                }}
                type="button"
              >
                {cancelMut.isPending ? "Cancelling…" : "Cancel transfer"}
              </button>
            ) : null}
          </div>
        ) : errorMessage ? (
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-3 rounded-lg border border-red-500/20 bg-red-500/10 p-4">
              <XCircle className="h-5 w-5 shrink-0 text-red-300" />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold text-red-200">Transfer failed</p>
                <p className="mt-0.5 text-xs text-red-300">{errorMessage}</p>
              </div>
            </div>
          </div>
        ) : transfer?.transferring ? (
          <div className="mt-4 flex items-center gap-3 rounded-lg border border-amber-500/20 bg-amber-500/10 p-4">
            <Loader2 className="h-5 w-5 shrink-0 animate-spin text-amber-300" />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-amber-200">Transfer pending</p>
              {phase ? (
                <p className="mt-0.5 text-xs text-amber-300">{phaseLabel}</p>
              ) : null}
            </div>
          </div>
        ) : (
          <div className="mt-4 flex items-center gap-3 rounded-lg border border-slate-500/20 bg-[#0a0e16] p-4">
            <CheckCircle2 className="h-5 w-5 shrink-0 text-slate-400" />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-slate-200">No active transfer</p>
              <p className="mt-0.5 text-xs text-slate-400">
                Transfer this server to another node when maintenance or rebalancing is needed.
              </p>
            </div>
          </div>
        )}
      </section>

      {/* Initiate transfer form – only when idle */}
      {!isTransferring && !transfer?.transferring && canTransfer ? (
        <section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-5">
          <h2 className="flex items-center gap-2 font-bold text-white">
            <Network size={18} />
            Initiate transfer
          </h2>

          <div className="mt-4 space-y-4">
            <label className="block text-xs font-semibold text-slate-300">
              Target node
              <select
                className={field}
                value={targetNodeId}
                onChange={(e) => { setTargetNodeId(e.target.value); setPrimaryAllocationId(""); }}
              >
                <option value="">Select a target node…</option>
                {(nodes ?? [])
                  .filter((n: ApiNode) => n.id !== server.nodeId && n.name !== server.node)
                  .map((n: ApiNode) => (
                    <option key={n.id} value={n.id}>{n.name}</option>
                  ))}
              </select>
            </label>

            {targetNodeId ? (
              <label className="block text-xs font-semibold text-slate-300">
                Primary allocation
                <select
                  className={field}
                  value={primaryAllocationId}
                  onChange={(e) => setPrimaryAllocationId(e.target.value)}
                >
                  <option value="">Select an allocation…</option>
                  {targetAllocations.map((a: ApiAllocation) => (
                    <option key={a.id} value={a.id}>{a.ip}:{a.port}</option>
                  ))}
                </select>
              </label>
            ) : null}

            {targetAllocations.length === 0 && targetNodeId ? (
              <p className="flex items-center gap-2 text-xs text-amber-300">
                <AlertCircle size={12} />
                No free allocations available on this node.
              </p>
            ) : null}

            <button
              className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-bold text-white disabled:opacity-40"
              disabled={!targetNodeId || !primaryAllocationId || startMut.isPending}
              onClick={() => {
                if (window.confirm("Start this server transfer? The server may become temporarily unavailable."))
                  startMut.mutate();
              }}
              type="button"
            >
              {startMut.isPending ? "Starting transfer…" : "Start transfer"}
            </button>

            {startMut.error ? (
              <p className="text-sm text-red-300" role="alert">
                {message(startMut.error, "Could not start transfer.")}
              </p>
            ) : null}
          </div>
        </section>
      ) : null}

      {/* Server information */}
      <section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-5">
        <h2 className="flex items-center gap-2 font-bold text-white">
          <Network size={18} />
          Current node details
        </h2>
        <dl className="mt-4 grid gap-4 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-xs text-slate-500">Current node</dt>
            <dd className="mt-1">{server.node ?? "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-slate-500">Node ID</dt>
            <dd className="mt-1 font-mono">{server.nodeId ?? "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-slate-500">Primary allocation</dt>
            <dd className="mt-1 font-mono">{server.allocation ?? "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-slate-500">Transfer state</dt>
            <dd className="mt-1">
              {server.transferring ? (
                <span className="inline-flex items-center gap-1 rounded-full border border-sky-500/30 bg-sky-500/10 px-2 py-0.5 text-xs font-medium text-sky-200">
                  <Loader2 size={10} className="animate-spin" /> Transferring
                </span>
              ) : server.transferError ? (
                <span className="inline-flex items-center gap-1 rounded-full border border-red-500/30 bg-red-500/10 px-2 py-0.5 text-xs font-medium text-red-200">
                  <XCircle size={10} /> Failed
                </span>
              ) : (
                <span className="inline-flex items-center gap-1 rounded-full border border-slate-500/30 bg-slate-500/10 px-2 py-0.5 text-xs font-medium text-slate-300">
                  Idle
                </span>
              )}
            </dd>
          </div>
        </dl>
      </section>
    </div>
  );
}
