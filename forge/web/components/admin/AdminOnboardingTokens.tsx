"use client";
import { useNodesQuery } from "@/lib/admin/telemetry";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock, Copy, Check, RefreshCw, Ticket, ShieldCheck, ShieldX, Ban } from "lucide-react";
import {
  createOnboardingToken,
  listOnboardingTokens,
  approveOnboardingToken,
  rejectOnboardingToken,
  revokeOnboardingToken,
  type OnboardingToken,
} from "@/lib/api/onboarding";
import {  } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminPageLayout,
  SectionHeader,
  Card,
  CardHeader,
  Btn,
  Pill,
  AdminLoadingState,
  AdminErrorState,
  EmptyState,
  Modal,
  ModalFooter,
} from "./admin-ui";

function ttlLabel(hours: number): string {
  if (hours <= 24) return `${hours}h`;
  return `${hours}h (${Math.round(hours / 24)}d)`;
}

export function AdminOnboardingTokens() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [, renderConfirm] = useConfirm();
  const [selectedNodeId, setSelectedNodeId] = useState<string>("");
  const [ttlHours, setTtlHours] = useState<number>(24);
  const [created, setCreated] = useState<{ token: string; tokenId: string; expiresAt: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [rejectReason, setRejectReason] = useState<Record<string, string>>({});
  const [revokeReason, setRevokeReason] = useState<Record<string, string>>({});

  const nodesQ = useNodesQuery();
  const nodes = useMemo(() => nodesQ.data ?? [], [nodesQ.data]);

  const tokensQ = useQuery({
    queryKey: ["admin-onboarding-tokens", selectedNodeId],
    queryFn: () => listOnboardingTokens(selectedNodeId),
    enabled: !!selectedNodeId,
    retry: false,
  });

  const createMut = useMutation({
    mutationFn: () => createOnboardingToken({ nodeId: selectedNodeId, ttlHours }),
    onSuccess: (data) => {
      setCreated({ token: data.token, tokenId: data.tokenId, expiresAt: data.expiresAt });
      void qc.invalidateQueries({ queryKey: ["admin-onboarding-tokens", selectedNodeId] });
      toast({ tone: "success", title: `Token ${data.tokenId.slice(0, 8)}… created — expires ${new Date(data.expiresAt).toLocaleString()}` });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Create failed", message: e.message }),
  });

  const approveMut = useMutation({
    mutationFn: (tokenId: string) => approveOnboardingToken(tokenId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-onboarding-tokens", selectedNodeId] });
      toast({ tone: "success", title: "Token approved" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Approve failed", message: e.message }),
  });

  const rejectMut = useMutation({
    mutationFn: ({ tokenId, reason }: { tokenId: string; reason: string }) => rejectOnboardingToken(tokenId, reason),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-onboarding-tokens", selectedNodeId] });
      toast({ tone: "success", title: "Token rejected" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Reject failed", message: e.message }),
  });

  const revokeMut = useMutation({
    mutationFn: ({ tokenId, reason }: { tokenId: string; reason: string }) => revokeOnboardingToken(tokenId, reason),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-onboarding-tokens", selectedNodeId] });
      toast({ tone: "success", title: "Token revoked" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Revoke failed", message: e.message }),
  });

  const tokens = useMemo(() => tokensQ.data ?? [], [tokensQ.data]);

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { if (selectedNodeId) void tokensQ.refetch(); void nodesQ.refetch(); }} />
      <SectionHeader
        title="Onboarding Tokens"
        sub="Node onboarding token lifecycle — issue, approve, reject, revoke (handlers_capabilities.go:235). 72h TTL max. State: pending → approved → consumed, or pending → rejected/revoked."
        action={
          <Btn size="sm" tone="ghost" onClick={() => { if (selectedNodeId) void tokensQ.refetch(); void nodesQ.refetch(); }}>
            <RefreshCw size={14} /> Refresh
          </Btn>
        }
      />

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="border border-[var(--line)] bg-[var(--surface)] lg:col-span-1">
          <CardHeader title="Issue token — POST /onboarding-tokens" icon={Ticket} />
          <div className="space-y-4 p-4">
            <label className="block text-sm">
              <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Node</span>
              <select
                className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm text-[var(--text)]"
                value={selectedNodeId}
                onChange={(e) => setSelectedNodeId(e.target.value)}
              >
                <option value="">Select node…</option>
                {nodes.map((n) => (
                  <option key={n.id} value={n.id}>
                    {n.name} · {n.id.slice(0, 8)}
                  </option>
                ))}
              </select>
              {nodesQ.isError && <div className="mt-2 text-xs text-danger">{(nodesQ.error as Error).message}</div>}
            </label>

            <div>
              <label htmlFor="onboarding-ttl" className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">
                TTL — {ttlLabel(ttlHours)} <span className="normal-case tracking-normal text-[color-mix(in_srgb,var(--text-subtle)_70%,transparent)]">(max 72h)</span>
              </label>
              <input
                id="onboarding-ttl"
                type="range"
                min={1}
                max={72}
                step={1}
                value={ttlHours}
                onChange={(e) => setTtlHours(Number(e.target.value))}
                aria-label={`TTL ${ttlLabel(ttlHours)}, range 1 to 72 hours`}
                aria-valuemin={1}
                aria-valuemax={72}
                aria-valuenow={ttlHours}
                className="w-full accent-[var(--brand)]"
              />
              <div className="flex justify-between font-mono text-[11px] text-[var(--text-subtle)]" aria-hidden>
                <span>1h</span>
                <span className="font-semibold text-[var(--text)]">{ttlLabel(ttlHours)}</span>
                <span>72h</span>
              </div>
            </div>

            <Btn tone="primary" disabled={!selectedNodeId || ttlHours < 1 || ttlHours > 72} loading={createMut.isPending} onClick={() => createMut.mutate()}>
              <Ticket size={14} /> Create token
            </Btn>

            {ttlHours > 72 && <div className="rounded-lg border border-danger-line bg-danger-subtle p-2 text-xs text-danger">TTL must not exceed 72h (handler returns 400).</div>}

            <div className="rounded-lg border border-[var(--line)] bg-[var(--canvas)] p-3 text-xs leading-5 text-[var(--text-subtle)]">
              Tokens are <span className="font-medium text-[var(--text)]">bcrypt-hashed</span> at rest; the plaintext (<code className="font-mono text-[11px]">id.secret</code>) is returned once on create. The beacon presents it once at <code className="font-mono">POST /nodes/:id/beacon</code> and it is consumed.
            </div>
          </div>
        </Card>

        <Card className="border border-[var(--line)] bg-[var(--surface)] lg:col-span-2">
          <CardHeader title={`Tokens — GET /onboarding-tokens?nodeId=${selectedNodeId ? selectedNodeId.slice(0, 8) + "…" : "…"}`} icon={Clock} action={tokens.length ? <Pill tone="neutral">{tokens.length}</Pill> : null} />
          {!selectedNodeId ? (
            <div className="p-8 text-center">
              <Ticket size={20} className="mx-auto text-[var(--text-subtle)]" />
              <div className="mt-2 text-sm font-medium text-[var(--text)]">Select a node</div>
              <div className="text-xs text-[var(--text-subtle)]">Listing requires <code className="font-mono text-[11px]">?nodeId=</code> (handler 422 otherwise). Choose a node to load its tokens.</div>
            </div>
          ) : tokensQ.isLoading ? (
            <AdminLoadingState label="Loading tokens…" />
          ) : tokensQ.isError ? (
            <div className="p-4"><AdminErrorState message={(tokensQ.error as Error).message} retry={() => void tokensQ.refetch()} /></div>
          ) : tokens.length === 0 ? (
            <EmptyState icon={Ticket} title="No tokens" message={`No onboarding tokens for this node. Issue one with a TTL up to 72h.`} />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--line)] bg-[var(--surface-raised)] text-left text-[10px] uppercase tracking-[0.12em] text-[var(--text-subtle)]">
                    <th className="px-4 py-3">ID</th>
                    <th className="px-4 py-3">State</th>
                    <th className="px-4 py-3">Expires</th>
                    <th className="px-4 py-3">Created</th>
                    <th className="px-4 py-3">Approved</th>
                    <th className="px-4 py-3 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--line)]">
                  {tokens.map((tok) => (
                    <TokenRow
                      key={tok.id}
                      tok={tok}
                      rejectReason={rejectReason[tok.id] ?? ""}
                      revokeReason={revokeReason[tok.id] ?? ""}
                      onRejectReason={(v) => setRejectReason((s) => ({ ...s, [tok.id]: v }))}
                      onRevokeReason={(v) => setRevokeReason((s) => ({ ...s, [tok.id]: v }))}
                      onApprove={() => approveMut.mutate(tok.id)}
                      onReject={() => rejectMut.mutate({ tokenId: tok.id, reason: rejectReason[tok.id] ?? "" })}
                      onRevoke={() => revokeMut.mutate({ tokenId: tok.id, reason: revokeReason[tok.id] ?? "" })}
                      approving={approveMut.isPending && approveMut.variables === tok.id}
                      rejecting={rejectMut.isPending}
                      revoking={revokeMut.isPending}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div className="border-t border-[var(--line)] p-3 text-xs leading-5 text-[var(--text-subtle)]">
            Endpoints: <code className="rounded bg-overlay-strong px-1 font-mono text-[11px]">POST /onboarding-tokens</code> ·{" "}
            <code className="font-mono text-[11px]">GET /onboarding-tokens/:id</code> ·{" "}
            <code className="font-mono text-[11px]">POST /:id/approve</code> ·{" "}
            <code className="font-mono text-[11px]">POST /:id/reject</code> ·{" "}
            <code className="font-mono text-[11px]">POST /:id/revoke</code>
          </div>
        </Card>
      </div>

      {created && (
        <Modal title="Token created" description="Copy once — store it securely" onClose={() => setCreated(null)} wide>
          <div className="space-y-4">
            <div className="rounded-lg border border-warn-line bg-warn-subtle p-3 text-sm text-warn">
              This plaintext token is shown <span className="font-semibold">once</span>. Store it securely — the backend only keeps the bcrypt hash. It expires {new Date(created.expiresAt).toLocaleString()}.
            </div>
            <div className="space-y-2">
              <div className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Plaintext (id.secret)</div>
              <div className="flex gap-2">
                <pre className="flex-1 overflow-auto rounded-lg border border-[var(--line)] bg-well p-3 font-mono text-xs leading-5 text-ok">{created.token}</pre>
                <Btn
                  size="sm"
                  tone="ghost"
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(created.token);
                      setCopied(true);
                      toast({ tone: "success", title: "Token copied" });
                      setTimeout(() => setCopied(false), 1500);
                    } catch {
                      toast({ tone: "error", title: "Copy failed" });
                    }
                  }}
                >
                  {copied ? <Check size={14} /> : <Copy size={14} />} {copied ? "Copied" : "Copy"}
                </Btn>
              </div>
              <div className="text-xs text-text-subtle">tokenId: <code className="font-mono">{created.tokenId}</code></div>
            </div>
          </div>
          <ModalFooter onCancel={() => setCreated(null)} onConfirm={() => { void navigator.clipboard.writeText(created.token); setCreated(null); }} confirmLabel="Copy & close" />
        </Modal>
      )}

      {renderConfirm()}
    </AdminPageLayout>
  );
}

function TokenRow({
  tok,
  rejectReason,
  revokeReason,
  onRejectReason,
  onRevokeReason,
  onApprove,
  onReject,
  onRevoke,
  approving,
  rejecting,
  revoking,
}: {
  tok: OnboardingToken;
  rejectReason: string;
  revokeReason: string;
  onRejectReason: (v: string) => void;
  onRevokeReason: (v: string) => void;
  onApprove: () => void;
  onReject: () => void;
  onRevoke: () => void;
  approving: boolean;
  rejecting: boolean;
  revoking: boolean;
}) {
  const expired = new Date(tok.expiresAt).getTime() < Date.now();
  const stateTone: Record<string, "green" | "yellow" | "red" | "neutral"> = {
    pending: "yellow",
    approved: "green",
    rejected: "red",
    revoked: "red",
    consumed: "neutral",
  };
  const canApprove = tok.state === "pending" && !expired;
  const canReject = tok.state === "pending";
  const canRevoke = tok.state === "pending" || tok.state === "approved";

  return (
    <tr className="hover:bg-[var(--surface-hover)] motion-safe:transition-colors">
      <td className="px-4 py-3">
        <div className="font-mono text-xs font-medium text-[var(--text)]">{tok.id.slice(0, 12)}…</div>
        <div className="font-mono text-[11px] text-[var(--text-subtle)]">{tok.nodeId.slice(0, 8)}…</div>
      </td>
      <td className="px-4 py-3">
        <Pill tone={stateTone[tok.state] ?? "neutral"} className="capitalize">{tok.state}</Pill>
        {expired && tok.state === "pending" && <Pill tone="red" className="ml-1">expired</Pill>}
        {tok.revokedReason && <div className="mt-1 max-w-[14rem] truncate text-[11px] text-[var(--text-subtle)]" title={tok.revokedReason}>{tok.revokedReason}</div>}
      </td>
      <td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">
        <span className={expired ? "text-danger" : ""}>{new Date(tok.expiresAt).toLocaleString()}</span>
      </td>
      <td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">{new Date(tok.createdAt).toLocaleString()}</td>
      <td className="px-4 py-3 text-xs text-[var(--text-subtle)]">
        {tok.approvedAt ? new Date(tok.approvedAt).toLocaleString() : "—"}
        {tok.approvedBy && <div className="font-mono text-[11px]">{tok.approvedBy.slice(0, 8)}…</div>}
      </td>
      <td className="px-4 py-3">
        <div className="flex flex-wrap justify-end gap-1.5">
          {canApprove && (
            <Btn size="sm" tone="success" loading={approving} onClick={onApprove}>
              <ShieldCheck size={12} /> Approve
            </Btn>
          )}
          {canReject && (
            <span className="inline-flex items-center gap-1">
              <input
                placeholder="reason"
                value={rejectReason}
                onChange={(e) => onRejectReason(e.target.value)}
                className="h-7 w-20 rounded border border-[var(--line)] bg-[var(--surface-raised)] px-2 text-xs"
              />
              <Btn size="sm" tone="ghost" loading={rejecting} onClick={onReject}>
                <ShieldX size={12} /> Reject
              </Btn>
            </span>
          )}
          {canRevoke && (
            <span className="inline-flex items-center gap-1">
              <input
                placeholder="reason"
                value={revokeReason}
                onChange={(e) => onRevokeReason(e.target.value)}
                className="h-7 w-20 rounded border border-[var(--line)] bg-[var(--surface-raised)] px-2 text-xs"
              />
              <Btn size="sm" tone="danger" loading={revoking} onClick={onRevoke}>
                <Ban size={12} /> Revoke
              </Btn>
            </span>
          )}
          {!canApprove && !canReject && !canRevoke && <span className="text-xs text-[var(--text-subtle)]">—</span>}
        </div>
      </td>
    </tr>
  );
}
