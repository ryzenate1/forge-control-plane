"use client";
import { useNodesQuery } from "@/lib/admin/telemetry";

import { useEffect, useMemo, useState } from "react";
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
import { REFRESH, nodeStatus, sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge, NotReported } from "@/components/admin/telemetry-ui";
import { copySecret } from "@/lib/clipboard";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { OfflineBanner } from "@/components/shared/states-offline";
import { toneStyles } from "@/components/ui/forge/status";
import {
  AdminPageLayout,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
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
  cn,
} from "./admin-ui";
import { formatDate } from "@/lib/utils";

const MAX_TTL_HOURS = 72;

function ttlLabel(hours: number): string {
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  const rest = hours % 24;
  return rest === 0 ? `${hours}h (${days}d)` : `${hours}h (${days}d ${rest}h)`;
}

export function AdminOnboardingTokens() {
  const { toast } = useToast();
  const qc = useQueryClient();
  // `confirm` was discarded here (`const [, renderConfirm]`), so approve, reject
  // and revoke each fired on a single click while an empty dialog was still
  // rendered on screen. All three now go through it.
  const [confirm, renderConfirm] = useConfirm();
  const [selectedNodeId, setSelectedNodeId] = useState<string>("");
  const [ttlHours, setTtlHours] = useState<number>(24);
  const [created, setCreated] = useState<{ token: string; tokenId: string; expiresAt: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [masked, setMasked] = useState(false);
  const [rejectReason, setRejectReason] = useState<Record<string, string>>({});
  const [revokeReason, setRevokeReason] = useState<Record<string, string>>({});

  // Expiry is a function of time, not of render. Computed once per paint with no
  // timer, a token that expired while the operator read the table kept its
  // pending pill and its Approve button until something else redrew the page.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(id);
  }, []);

  // A one-time secret must not survive the operator leaving the tab: masking on
  // blur and on hidden matches the node credential surfaces.
  useEffect(() => {
    if (!created) return;
    const hide = () => setMasked(true);
    const onVisibilityChange = () => { if (document.visibilityState === "hidden") hide(); };
    window.addEventListener("blur", hide);
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      window.removeEventListener("blur", hide);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [created]);

  const nodesQ = useNodesQuery();
  const nodesSource = sourceState(nodesQ, REFRESH.inventory);
  const nodes = useMemo(() => nodesQ.data ?? [], [nodesQ.data]);
  const selectedNode = nodes.find((n) => n.id === selectedNodeId);

  const tokensQ = useQuery({
    queryKey: ["admin-onboarding-tokens", selectedNodeId],
    queryFn: () => listOnboardingTokens(selectedNodeId),
    enabled: !!selectedNodeId,
    // An approval queue that only moves when the operator presses Refresh is not
    // a queue; poll it on the inventory cadence.
    refetchInterval: REFRESH.inventory,
    retry: false,
  });
  const tokensSource = sourceState(tokensQ, REFRESH.inventory);

  const invalidateTokens = () => void qc.invalidateQueries({ queryKey: ["admin-onboarding-tokens", selectedNodeId] });

  const createMut = useMutation({
    mutationFn: () => createOnboardingToken({ nodeId: selectedNodeId, ttlHours }),
    onSuccess: (data) => {
      setMasked(false);
      setCreated({ token: data.token, tokenId: data.tokenId, expiresAt: data.expiresAt });
      invalidateTokens();
      toast({ tone: "success", title: `Token ${data.tokenId.slice(0, 8)}… created`, message: `Expires ${formatDate(data.expiresAt)}.` });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Create failed", message: e.message }),
  });

  const approveMut = useMutation({
    mutationFn: (tokenId: string) => approveOnboardingToken(tokenId),
    onSuccess: () => { invalidateTokens(); toast({ tone: "success", title: "Token approved", message: "The Beacon can now present it once." }); },
    onError: (e: Error) => toast({ tone: "error", title: "Approve failed", message: e.message }),
  });

  const rejectMut = useMutation({
    mutationFn: ({ tokenId, reason }: { tokenId: string; reason: string }) => rejectOnboardingToken(tokenId, reason),
    onSuccess: () => { invalidateTokens(); toast({ tone: "success", title: "Token rejected" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Reject failed", message: e.message }),
  });

  const revokeMut = useMutation({
    mutationFn: ({ tokenId, reason }: { tokenId: string; reason: string }) => revokeOnboardingToken(tokenId, reason),
    onSuccess: () => { invalidateTokens(); toast({ tone: "success", title: "Token revoked" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Revoke failed", message: e.message }),
  });

  const requestApprove = (tok: OnboardingToken) => {
    void (async () => {
      const ok = await confirm({
        title: `Approve token ${tok.id.slice(0, 12)}…?`,
        description: `This lets one Beacon running on node "${selectedNode?.name ?? tok.nodeId}" present this credential once to join. Approval cannot be undone; a later compromise has to be revoked.`,
        confirmLabel: "Approve",
      });
      if (ok) approveMut.mutate(tok.id);
    })();
  };
  const requestReject = (tok: OnboardingToken) => {
    void (async () => {
      const ok = await confirm({
        title: `Reject token ${tok.id.slice(0, 12)}…?`,
        description: `The pending credential for node "${selectedNode?.name ?? tok.nodeId}" ends now and cannot be approved later. No workload is affected.`,
        danger: true,
        confirmLabel: "Reject",
      });
      if (ok) rejectMut.mutate({ tokenId: tok.id, reason: rejectReason[tok.id] ?? "" });
    })();
  };
  const requestRevoke = (tok: OnboardingToken) => {
    void (async () => {
      const ok = await confirm({
        title: `Revoke ${tok.state === "approved" ? "an approved" : "pending"} token ${tok.id.slice(0, 12)}…?`,
        description: tok.state === "approved"
          ? `This credential for node "${selectedNode?.name ?? tok.nodeId}" stops working immediately. If a Beacon is mid-enrolment it will fail and need a new token. This cannot be undone.`
          : `The pending credential for node "${selectedNode?.name ?? tok.nodeId}" is withdrawn. This cannot be undone.`,
        danger: true,
        confirmLabel: "Revoke",
      });
      if (ok) revokeMut.mutate({ tokenId: tok.id, reason: revokeReason[tok.id] ?? "" });
    })();
  };

  const tokens = useMemo(() => tokensQ.data ?? [], [tokensQ.data]);
  const pendingCount = tokens.filter((t) => t.state === "pending").length;

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { if (selectedNodeId) void tokensQ.refetch(); void nodesQ.refetch(); }} />
      <SectionHeader
        status={<FreshnessBadge state={nodesSource} />}
        info={{
          title: "Onboarding Tokens",
          triggerLabel: "About Onboarding Tokens",
          eyebrow: "Architecture & Semantics",
          description: "Issue, approve and revoke node onboarding tokens.",
          sections: [
            { title: "Issue & TTL", content: "Tokens are bcrypt-hashed at rest; the plaintext is shown once on create and can never be recovered — rotate instead. TTL caps at 72h. Copying writes to the system clipboard, which is wiped 15s later and when this window loses focus." },
            { title: "Approve, reject, revoke", content: "Pending tokens move to approved and end as consumed on use, or end as rejected or revoked. An expired pending token cannot be approved. Revoking an approved credential is immediate and unrecoverable, so every one of these asks first." },
            { title: "What this list covers", content: "Listing is per node — the API has no fleet-wide queue, so an approval waiting on a node you have not selected is invisible here. A token can only be issued for a node that already exists in Nodes; there is no enrol-a-machine-that-is-not-registered path." },
          ],
        }}
        action={
          <Btn size="sm" tone="ghost" onClick={() => { if (selectedNodeId) void tokensQ.refetch(); void nodesQ.refetch(); }}>
            <RefreshCw size={14} /> Refresh
          </Btn>
        }
      />

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="border border-line bg-overlay-subtle lg:col-span-1">
          <CardHeader icon={Ticket} title="Issue token" />
          <div className="space-y-4 p-4">
            <AdminSelect
              label="Node"
              value={selectedNodeId}
              onChange={setSelectedNodeId}
              placeholder="Select node…"
              /* One naming contract: name, a short id and the canonical verdict,
                 so an offline host cannot be mistaken for a place to enrol. */
              options={nodes.map((n) => {
                const verdict = nodeStatus(n);
                return { value: n.id, label: `${n.name} · ${n.id.slice(0, 8)} · ${verdict.label}` };
              })}
            />
            {nodesQ.isPending ? (
              <p className="text-xs text-text-muted">Loading nodes…</p>
            ) : nodesQ.isError ? (
              <div className={cn("rounded-lg border p-2 text-xs", toneStyles.danger.chip)}>
                Nodes could not be loaded: {nodesSource.message ?? "the request failed"}. No token can be issued until the list reads.
              </div>
            ) : nodes.length === 0 ? (
              <p className="text-xs text-warn">No nodes exist yet. A token is issued against an existing node — register it under Nodes first.</p>
            ) : null}
            {selectedNode ? (
              <p className="text-xs leading-5 text-text-muted">
                {`Node “${selectedNode.name}” reads ${nodeStatus(selectedNode).label.toLowerCase()} right now. A Beacon has to reach the panel to use a token, so an offline host will not consume one immediately.`}
              </p>
            ) : (
              <p className="text-xs leading-5 text-text-muted">Choose a node explicitly — nothing is picked for you.</p>
            )}

            <div>
              <label htmlFor="onboarding-ttl" className="mb-1 block text-xs font-semibold uppercase tracking-wide text-text-subtle">
                TTL — {ttlLabel(ttlHours)} <span className="normal-case tracking-normal text-text-muted">(max {MAX_TTL_HOURS}h)</span>
              </label>
              <input
                id="onboarding-ttl"
                type="range"
                min={1}
                max={MAX_TTL_HOURS}
                step={1}
                value={ttlHours}
                onChange={(e) => setTtlHours(Math.max(1, Math.min(MAX_TTL_HOURS, Number(e.target.value) || 1)))}
                aria-label={`TTL ${ttlLabel(ttlHours)}, range 1 to ${MAX_TTL_HOURS} hours`}
                aria-valuemin={1}
                aria-valuemax={MAX_TTL_HOURS}
                aria-valuenow={ttlHours}
                className="w-full accent-[var(--brand)]"
              />
              <div className="flex justify-between font-mono text-[11px] text-text-subtle" aria-hidden>
                <span>1h</span>
                <span className="font-semibold text-text">{ttlLabel(ttlHours)}</span>
                <span>{MAX_TTL_HOURS}h</span>
              </div>
            </div>

            <Btn
              tone="primary"
              disabled={!selectedNodeId || nodesQ.isError || createMut.isPending}
              loading={createMut.isPending}
              onClick={() => createMut.mutate()}
            >
              <Ticket size={14} /> Create token
            </Btn>
            {!selectedNodeId ? (
              <p className="text-xs leading-5 text-warn">Selecting a node enables creation.</p>
            ) : null}

            <div className="rounded-lg border border-line bg-overlay px-3 p-3 text-xs leading-5 text-text-subtle">
              Tokens are <span className="font-medium text-text">bcrypt-hashed</span> at rest; the plaintext (<code className="font-mono text-[11px]">id.secret</code>) is returned once on create. The Beacon presents it once when it calls in and it is consumed.
            </div>
          </div>
        </Card>

        <Card className="border border-line bg-overlay-subtle lg:col-span-2">
          <CardHeader
            icon={Clock}
            title={tokensSource.status === "ready"
              ? `${tokens.length} token${tokens.length === 1 ? "" : "s"} · ${pendingCount} pending`
              : "Tokens"}
            action={<FreshnessBadge state={tokensSource} />}
          />
          {!selectedNodeId ? (
            <div className="p-8 text-center">
              <Ticket size={20} className="mx-auto text-text-subtle" />
              <div className="mt-2 text-sm font-medium text-text">Select a node</div>
              <div className="text-xs text-text-subtle">Choose a node to load its tokens. Listing is per node — the API has no fleet-wide view, so tokens waiting on other hosts are not shown here.</div>
            </div>
          ) : tokensQ.isPending ? (
            <AdminLoadingState label="Loading tokens…" />
          ) : tokensQ.isError ? (
            <div className="p-4"><AdminErrorState message={(tokensQ.error as Error).message} retry={() => void tokensQ.refetch()} /></div>
          ) : tokens.length === 0 ? (
            <EmptyState icon={Ticket} title="No tokens for this node" message="No onboarding tokens issued for this node. Issue one with a TTL up to 72h." />
          ) : (
            <AdminTable label="Onboarding tokens">
              <AdminTHead>
                <AdminTh>Token</AdminTh>
                <AdminTh>State</AdminTh>
                <AdminTh>Expires</AdminTh>
                <AdminTh>Created</AdminTh>
                <AdminTh>Approved / ended</AdminTh>
                <AdminTh className="text-right">Actions</AdminTh>
              </AdminTHead>
              <AdminTBody>
                  {tokens.map((tok) => (
                    <TokenRow
                      key={tok.id}
                      tok={tok}
                      now={now}
                      rejectReason={rejectReason[tok.id] ?? ""}
                      revokeReason={revokeReason[tok.id] ?? ""}
                      onRejectReason={(v) => setRejectReason((s) => ({ ...s, [tok.id]: v }))}
                      onRevokeReason={(v) => setRevokeReason((s) => ({ ...s, [tok.id]: v }))}
                      onApprove={() => requestApprove(tok)}
                      onReject={() => requestReject(tok)}
                      onRevoke={() => requestRevoke(tok)}
                      approving={approveMut.isPending && approveMut.variables === tok.id}
                      rejecting={rejectMut.isPending}
                      revoking={revokeMut.isPending}
                    />
                  ))}
              </AdminTBody>
            </AdminTable>
          )}
          <div className="border-t border-line p-3 text-xs leading-5 text-text-subtle">
            Tokens move pending → approved → consumed, or end as rejected or revoked. A consumed token is a completed enrolment, shown deliberately apart from a revoked or rejected one. {tokensSource.status === "error" ? "This list failed to refresh and may not show the current state." : `Expiry is re-evaluated every 30s against ${formatDate(now)}.`}
          </div>
        </Card>
      </div>

      {created && (
        <Modal title="Token created" description="Copy once — store it securely" onClose={() => setCreated(null)} wide>
          <div className="space-y-4">
            <div className={cn("rounded-lg border p-3 text-sm", toneStyles.warn.chip)}>
              This plaintext token is shown <span className="font-semibold">once</span>. Store it securely — the backend only keeps the bcrypt hash. It expires {formatDate(created.expiresAt)}.
            </div>
            <div className="space-y-2">
              <div className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Plaintext (id.secret)</div>
              <div className="flex gap-2">
                <pre className="flex-1 overflow-auto rounded-lg border border-line bg-overlay-strong p-3 font-mono text-xs leading-5 text-text">{masked ? "••••••••••••••••••••••••••••" : created.token}</pre>
                <Btn
                  size="sm"
                  tone="ghost"
                  onClick={() => setMasked((m) => !m)}
                  ariaLabel={masked ? "Reveal the plaintext token" : "Hide the plaintext token"}
                >
                  {masked ? "Reveal" : "Hide"}
                </Btn>
                <Btn
                  size="sm"
                  tone="ghost"
                  ariaLabel="Copy the plaintext token to the clipboard"
                  onClick={async () => {
                    // `copySecret` rather than a bare clipboard write: it wipes the
                    // clipboard after 15s and when the window loses focus.
                    if (await copySecret(created.token)) {
                      setCopied(true);
                      toast({ tone: "success", title: "Token copied", message: "The clipboard is cleared 15s after copying." });
                      setTimeout(() => setCopied(false), 2000);
                    } else {
                      toast({ tone: "error", title: "Copy failed", message: "The browser refused clipboard access. Reveal the value and copy it manually." });
                    }
                  }}
                >
                  {copied ? <Check size={14} /> : <Copy size={14} />} {copied ? "Copied" : "Copy"}
                </Btn>
              </div>
              <div className="text-xs text-text-muted">tokenId: <code className="font-mono">{created.tokenId}</code></div>
              <p className="text-xs leading-5 text-text-muted">
                Closing this dialog does not copy anything. The value is hidden when this window loses focus, and it cannot be shown again afterwards — rotate to get a new one.
              </p>
            </div>
          </div>
          {/* "Copy & close" wrote the plaintext to the clipboard silently on
              dismissal, where it stayed indefinitely. */}
          <ModalFooter onCancel={() => setCreated(null)} onConfirm={() => setCreated(null)} confirmLabel="Done" />
        </Modal>
      )}

      {renderConfirm()}
    </AdminPageLayout>
  );
}

function TokenRow({
  tok,
  now,
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
  now: number;
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
  const expiresAt = Date.parse(tok.expiresAt);
  const expired = Number.isFinite(expiresAt) ? expiresAt < now : null;
  const terminal = tok.state === "rejected" || tok.state === "revoked" || tok.state === "consumed";
  // Consumed is the successful end of the workflow; it used to share grey with
  // "we do not know", so a used credential and an unreadable one looked alike.
  const stateTone: Record<string, "green" | "yellow" | "red" | "blue" | "neutral"> = {
    pending: "yellow",
    approved: "green",
    consumed: "blue",
    rejected: "red",
    revoked: "red",
  };
  // Approval is withheld when the token is *known* to have expired. An
  // unparseable expiry stays approvable — the server decides, and the row says
  // the expiry is unknown rather than pretending it has passed.
  const canApprove = tok.state === "pending" && expired !== true;
  const canReject = tok.state === "pending";
  const canRevoke = tok.state === "pending" || tok.state === "approved";

  return (
    <AdminTr>
      <AdminTd>
        <div className="font-mono text-xs font-medium text-text">{tok.id.slice(0, 12)}…</div>
        <div className="font-mono text-[11px] text-text-subtle">node {tok.nodeId.slice(0, 8)}…</div>
      </AdminTd>
      <AdminTd>
        <Pill tone={stateTone[tok.state] ?? "neutral"}>{stateTone[tok.state] ? tok.state : "unknown state"}</Pill>
        {tok.state === "pending" && expired === true ? <span className="ml-1"><Pill tone="red">expired</Pill></span> : null}
        {tok.state === "pending" && expired === null ? <span className="ml-1"><Pill tone="neutral">expiry unknown</Pill></span> : null}
        {/* Truncated-reason-with-`title`-only hid the only copy of the text;
            the reason is the point of the row, so it is shown in full. */}
        {tok.revokedReason ? <div className="mt-1 max-w-[18rem] text-[11px] leading-4 text-text-subtle">Reason: {tok.revokedReason}</div> : null}
      </AdminTd>
      <AdminTd className="font-mono text-xs text-text-subtle">
        {Number.isFinite(expiresAt) ? (
          <span className={expired === true && !terminal ? toneStyles.danger.fg : undefined}>
            {formatDate(tok.expiresAt)}
            {expired === true && !terminal ? " · passed" : ""}
          </span>
        ) : <NotReported reason="No expiry timestamp on this token" />}
      </AdminTd>
      <AdminTd className="font-mono text-xs text-text-subtle">{tok.createdAt ? formatDate(tok.createdAt) : <NotReported reason="No creation timestamp" />}</AdminTd>
      <AdminTd className="text-xs text-text-subtle">
        {tok.approvedAt ? (
          <div>approved {formatDate(tok.approvedAt)}{tok.approvedBy ? <span className="block font-mono text-[11px]">by {tok.approvedBy.slice(0, 8)}…</span> : null}</div>
        ) : null}
        {tok.revokedAt ? <div>ended {formatDate(tok.revokedAt)}</div> : null}
        {!tok.approvedAt && !tok.revokedAt ? <NotReported reason={tok.state === "pending" ? "Awaiting a decision" : "No decision recorded"} /> : null}
      </AdminTd>
      <AdminTd>
        <div className="flex flex-wrap justify-end gap-1.5">
          {canApprove ? (
            <Btn size="sm" tone="success" loading={approving} onClick={onApprove} ariaLabel={`Approve token ${tok.id.slice(0, 12)}`}>
              <ShieldCheck size={12} /> Approve
            </Btn>
          ) : tok.state === "pending" && expired === true ? (
            /* The withheld action says why it is withheld. */
            <span className="text-xs leading-6 text-warn">Expired — cannot be approved; issue a new token.</span>
          ) : null}
          {canReject ? (
            <span className="inline-flex items-center gap-1">
              <label className="sr-only" htmlFor={`reject-reason-${tok.id}`}>Reason for rejecting token {tok.id.slice(0, 8)}</label>
              <input
                id={`reject-reason-${tok.id}`}
                placeholder="reason (optional)"
                value={rejectReason}
                onChange={(e) => onRejectReason(e.target.value)}
                className="h-8 w-36 rounded border border-line bg-overlay px-2 text-xs text-text"
              />
              <Btn size="sm" tone="ghost" loading={rejecting} onClick={onReject} ariaLabel={`Reject token ${tok.id.slice(0, 12)}`}>
                <ShieldX size={12} /> Reject
              </Btn>
            </span>
          ) : null}
          {canRevoke ? (
            <span className="inline-flex items-center gap-1">
              <label className="sr-only" htmlFor={`revoke-reason-${tok.id}`}>Reason for revoking token {tok.id.slice(0, 8)}</label>
              <input
                id={`revoke-reason-${tok.id}`}
                placeholder="reason (optional)"
                value={revokeReason}
                onChange={(e) => onRevokeReason(e.target.value)}
                className="h-8 w-36 rounded border border-line bg-overlay px-2 text-xs text-text"
              />
              <Btn size="sm" tone="danger" loading={revoking} onClick={onRevoke} ariaLabel={`Revoke token ${tok.id.slice(0, 12)}`}>
                <Ban size={12} /> Revoke
              </Btn>
            </span>
          ) : null}
          {!canApprove && !canReject && !canRevoke ? <span className="text-xs text-text-subtle">No action — {tok.state} is final</span> : null}
        </div>
      </AdminTd>
    </AdminTr>
  );
}
