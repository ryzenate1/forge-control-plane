"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertCircle, ArrowRightLeft, CheckCircle2, Loader2, Network, XCircle,
} from "lucide-react";
import { useToast } from "@/components/ui/toast";
import { ConfirmDialog, StatusPill } from "@/components/ui/primitives";
import { StatusCard } from "@/components/ui/status-card";
import {
  type ApiAllocation, type ApiNode, type ApiServer,
  fetchNodeAllocations, fetchNodes,
} from "@/lib/api";
import { cancelServerTransfer, fetchServerTransferStatus, transferServer } from "@/lib/api/servers";
import { errorMessage as message } from "@/lib/utils";
import { hasServerPermission, useOptionalServerContext } from "./server-context";

export function TransferView({ server }: { server: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const canTransfer = hasServerPermission(access, "settings.reinstall");
  const qc = useQueryClient();
  const { toast } = useToast();

  const [targetNodeId, setTargetNodeId] = useState(server.transferTargetNodeId ?? "");
  const [primaryAllocationId, setPrimaryAllocationId] = useState("");
  const [cancelConfirmOpen, setCancelConfirmOpen] = useState(false);
  const [startConfirmOpen, setStartConfirmOpen] = useState(false);

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
    mutationFn: () => transferServer(server.id, targetNodeId, primaryAllocationId || undefined),
    onSuccess: () => {
      setStartConfirmOpen(false);
      void transferStatusQuery.refetch();
      void qc.invalidateQueries({ queryKey: ["server", server.id] });
    },
    onError: (error) =>
      toast({ tone: "error", title: "Transfer failed", message: error instanceof Error ? error.message : "Could not transfer server" }),
  });

  const cancelMut = useMutation({
    mutationFn: () => cancelServerTransfer(server.id),
    onSuccess: () => {
      setCancelConfirmOpen(false);
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
      <StatusCard
        icon={<ArrowRightLeft size={18} />}
        title="Server transfer"
        tone={isTransferring ? "ok" : errorMessage ? "danger" : transfer?.transferring ? "warning" : "neutral"}
      >
        {isTransferring ? (
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-3 rounded-lg border border-emerald-500/25 bg-emerald-500/[0.09] p-4">
              <Loader2 className="h-5 w-5 shrink-0 animate-spin text-emerald-300" />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold text-emerald-200">Transfer in progress</p>
                {phaseLabel ? (
                  <p className="mt-0.5 font-mono text-xs text-emerald-300">{phaseLabel}</p>
                ) : null}
              </div>
            </div>

            {progress !== null && (
              <div>
                <div className="flex items-center justify-between text-xs text-slate-400">
                  <span>Progress</span>
                  <span className="font-mono">{Math.round(progress)}%</span>
                </div>
                <div className="mt-1 h-2 overflow-hidden rounded-full bg-white/[0.06]">
                  <div
                    className="h-full rounded-full bg-emerald-500 transition-all"
                    style={{ width: `${Math.min(100, Math.max(0, progress))}%` }}
                  />
                </div>
              </div>
            )}

            {errorMessage ? (
              <p className="ui-alert ui-alert-error" role="alert">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />{errorMessage}
              </p>
            ) : null}

            {canTransfer ? (
              <button
                className="ui-button ui-button-danger"
                disabled={cancelMut.isPending}
                onClick={() => setCancelConfirmOpen(true)}
                type="button"
              >
                {cancelMut.isPending ? "Cancelling…" : "Cancel transfer"}
              </button>
            ) : null}
          </div>
        ) : errorMessage ? (
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-3 rounded-lg border border-red-500/25 bg-red-500/[0.09] p-4">
              <XCircle className="h-5 w-5 shrink-0 text-red-300" />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold text-red-200">Transfer failed</p>
                <p className="mt-0.5 text-xs text-red-300">{errorMessage} Check the daemon logs on the target node and the network connection between nodes, then retry.</p>
              </div>
            </div>
          </div>
        ) : transfer?.transferring ? (
          <div className="mt-4 flex items-center gap-3 rounded-lg border border-amber-500/25 bg-amber-500/[0.09] p-4">
            <Loader2 className="h-5 w-5 shrink-0 animate-spin text-amber-300" />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-amber-200">Transfer pending</p>
              {phase ? (
                <p className="mt-0.5 font-mono text-xs text-amber-300">{phaseLabel}</p>
              ) : null}
            </div>
          </div>
        ) : (
          <div className="mt-4 flex items-center gap-3 rounded-lg border border-slate-500/25 bg-white/[0.02] p-4">
            <CheckCircle2 className="h-5 w-5 shrink-0 text-slate-400" />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-slate-200">No active transfer</p>
              <p className="mt-0.5 text-xs text-slate-400">
                Transfer this server to another node when maintenance or rebalancing is needed.
              </p>
            </div>
          </div>
        )}
      </StatusCard>

      {/* Initiate transfer form – only when idle */}
      {!isTransferring && !transfer?.transferring && canTransfer ? (
        <section className="ui-card">
          <h2 className="flex items-center gap-2 font-bold text-white">
            <Network size={18} />
            Initiate transfer
          </h2>

          <div className="mt-4 space-y-4">
            <label className="ui-label">
              Target node
              <select
                className="ui-input mt-1.5"
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
              <label className="ui-label">
                Primary allocation
                <select
                  className="ui-input mt-1.5"
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
              <p className="ui-alert ui-alert-warning" role="alert">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>No free allocations available on this node. Free one on the target node first, then retry.</span>
              </p>
            ) : null}

            <button
              className="ui-button ui-button-primary"
              disabled={!targetNodeId || !primaryAllocationId || startMut.isPending}
              onClick={() => setStartConfirmOpen(true)}
              type="button"
            >
              {startMut.isPending ? "Starting transfer…" : "Start transfer"}
            </button>

            {startMut.error ? (
              <p className="ui-alert ui-alert-error" role="alert">
                {message(startMut.error, "Could not start transfer.")}
              </p>
            ) : null}
          </div>
        </section>
      ) : null}

      {/* Server information */}
      <section className="ui-card">
        <h2 className="flex items-center gap-2 font-bold text-white">
          <Network size={18} />
          Current node details
        </h2>
        <dl className="mt-4 grid gap-4 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-xs text-slate-500">Current node</dt>
            <dd className="mt-1 text-slate-200">{server.node ?? "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-slate-500">Node ID</dt>
            <dd className="mt-1 font-mono text-slate-200">{server.nodeId ?? "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-slate-500">Primary allocation</dt>
            <dd className="mt-1 font-mono text-slate-200">{server.allocation ?? "—"}</dd>
          </div>
          <div>
            <dt className="text-xs text-slate-500">Transfer state</dt>
            <dd className="mt-1">
              {server.transferring ? (
                <StatusPill tone="info"><Loader2 size={10} className="animate-spin" /> Transferring</StatusPill>
              ) : server.transferError ? (
                <StatusPill tone="danger"><XCircle size={10} /> Failed</StatusPill>
              ) : (
                <StatusPill tone="neutral">Idle</StatusPill>
              )}
            </dd>
          </div>
        </dl>
      </section>

      <ConfirmDialog
        confirmAction={() => cancelMut.mutate()}
        confirmLabel="Cancel transfer"
        description="Aborting this transfer stops the move mid-flight. The server remains on its current node, but partial data may need a consistency check before it starts cleanly."
        destructive
        loading={cancelMut.isPending}
        closeAction={() => { if (!cancelMut.isPending) setCancelConfirmOpen(false); }}
        open={cancelConfirmOpen}
        title="Cancel this server transfer?"
      />

      <ConfirmDialog
        confirmAction={() => startMut.mutate()}
        confirmLabel="Start transfer"
        description="The server may become temporarily unavailable while its data moves to the target node. Deployment and console actions will be blocked until the transfer completes."
        destructive
        loading={startMut.isPending}
        closeAction={() => { if (!startMut.isPending) setStartConfirmOpen(false); }}
        open={startConfirmOpen}
        title="Start this server transfer?"
      />
    </div>
  );
}
