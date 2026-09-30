"use client";

import { useState, useMemo, Suspense, type ReactNode } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { Play, Pencil, Square, RotateCcw, Trash2, Rocket, GitCompare } from "lucide-react";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSection,
  AdminSelect,
  AdminTabs,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  EmptyState,
  Modal,
  ModalFooter,
  Pill,
} from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  getComposeStackStatus,
  getComposeStackLogs,
  stopComposeStack,
  startComposeStack,
  deployComposeStack,
  deleteComposeStack,
  restartComposeStack,
  updateComposeStack,
  validateCompose,
  detectDrift,
  getComposeGitStatus,
  getLastWebhook,
  redeployFromGit,
  pullAndRedeploy,
  rollbackCompose,
  checkUpdate,
} from "@/lib/api/compose";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { composeStatusTone } from "@/lib/api/status";
import { sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { errorMessage, formatDate } from "@/lib/utils";

const STATUS_POLL_MS = 10_000;
const LOG_POLL_MS = 5_000;

const COMPOSE_TABS = ["overview", "services", "logs", "yaml", "gitops"] as const;
type ComposeTab = (typeof COMPOSE_TABS)[number];

const TAB_LABELS: Record<ComposeTab, string> = {
  overview: "Overview",
  services: "Services",
  logs: "Logs",
  yaml: "Compose file",
  gitops: "GitOps",
};

/** `GET /compose/git/:id/drift` → `DriftCheckResult` in `compose/gitops.go:77`. */
type DriftResult = {
  stackId?: string;
  deployedSha?: string;
  currentSha?: string;
  hasDrift?: boolean;
  servicesDiff?: { service?: string; change?: string; detail?: string }[];
};

function humanToken(value: string): string {
  return value.replace(/_/g, " ");
}

/** `stack.memoryMb` legitimately reads 0; 0 must not render as "not reported". */
function numberOrNotReported(value: number | undefined | null, unit: string): string {
  return typeof value === "number" && Number.isFinite(value) ? `${value.toLocaleString()} ${unit}` : "Not reported";
}

function ComposeStackDetailContent() {
  const [confirm, renderConfirm] = useConfirm();
  const params = useParams();
  const router = useRouter();
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const id = params.id as string;

  const rawTab = searchParams.get("tab");
  const tab: ComposeTab = (COMPOSE_TABS as readonly string[]).includes(rawTab ?? "")
    ? (rawTab as ComposeTab)
    : "overview";
  const setTab = (next: ComposeTab) => {
    router.replace(`/admin/compose/${encodeURIComponent(id)}?tab=${encodeURIComponent(next)}`, { scroll: false });
  };

  const [logService, setLogService] = useState("");
  const [logTail, setLogTail] = useState(100);
  const [editYaml, setEditYaml] = useState("");
  const [editingYaml, setEditingYaml] = useState(false);
  const [deleteVolumes, setDeleteVolumes] = useState(false);
  const [showRawDrift, setShowRawDrift] = useState(false);
  const [showRawGit, setShowRawGit] = useState(false);
  const [updateCheck, setUpdateCheck] = useState<unknown>(null);

  const statusQuery = useQuery({
    queryKey: ["compose-stack-status", id],
    queryFn: () => getComposeStackStatus(id),
    refetchInterval: STATUS_POLL_MS,
  });

  const logsQuery = useQuery<string[]>({
    queryKey: ["compose-stack-logs", id, logService, logTail],
    queryFn: async () => {
      const data = (await getComposeStackLogs(id, logService || undefined, logTail)) as {
        services?: Record<string, string>;
      };
      if (data?.services) {
        // A named service with no `_all` key would silently read as "no logs at
        // all"; ask for the service's own stream when one is selected.
        const stream = logService ? (data.services[logService] ?? data.services._all ?? "") : (data.services._all ?? "");
        return stream.split("\n").filter(Boolean);
      }
      return [];
    },
    refetchInterval: LOG_POLL_MS,
    enabled: tab === "logs",
  });

  const gitStatusQ = useQuery({
    queryKey: ["compose-git-status", id],
    queryFn: () => getComposeGitStatus(id),
    enabled: tab === "gitops",
  });

  const driftQ = useQuery<DriftResult>({
    queryKey: ["compose-drift", id],
    queryFn: () => detectDrift(id) as Promise<DriftResult>,
    enabled: tab === "gitops",
  });

  const webhookQ = useQuery({
    queryKey: ["compose-webhook", id],
    queryFn: () => getLastWebhook(id),
    enabled: tab === "gitops",
  });

  const safeLogs = useMemo(() => (Array.isArray(logsQuery.data) ? logsQuery.data : []), [logsQuery.data]);
  const stack = statusQuery.data?.stack;
  const safeServices = useMemo(
    () => (Array.isArray(statusQuery.data?.services) ? statusQuery.data.services : []),
    [statusQuery.data?.services],
  );

  function refreshed() {
    queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
  }

  const stopMutation = useMutation({
    mutationFn: () => stopComposeStack(id),
    onSuccess: () => { refreshed(); toast({ tone: "success", title: "Stop requested", message: "The agent is stopping the stack." }); },
    onError: (e: Error) => toast({ tone: "error", title: "Stop failed", message: errorMessage(e) }),
  });

  const startMutation = useMutation({
    mutationFn: () => startComposeStack(id),
    onSuccess: () => { refreshed(); toast({ tone: "success", title: "Start requested", message: "The agent is starting the stack." }); },
    onError: (e: Error) => toast({ tone: "error", title: "Start failed", message: errorMessage(e) }),
  });

  const redeployMutation = useMutation({
    mutationFn: () => deployComposeStack(id),
    onSuccess: () => { refreshed(); toast({ tone: "success", title: "Redeploy requested", message: "The stack is being redeployed from its stored compose file." }); },
    onError: (e: Error) => toast({ tone: "error", title: "Redeploy failed", message: errorMessage(e) }),
  });

  const restartMutation = useMutation({
    mutationFn: () => restartComposeStack(id),
    onSuccess: () => { refreshed(); toast({ tone: "success", title: "Restart requested", message: "The agent is restarting the stack's containers." }); },
    onError: (e: Error) => toast({ tone: "error", title: "Restart failed", message: errorMessage(e) }),
  });

  const updateMutation = useMutation({
    // The document is validated before it is saved: an invalid compose file
    // used to be written straight to the stack with no server check.
    mutationFn: async (yaml: string) => {
      const verdict = await validateCompose(yaml);
      if (!verdict.valid) {
        const first = verdict.errors?.[0];
        throw new Error(
          first ? `${first.field}: ${first.message}` : "The control plane rejected this compose document.",
        );
      }
      return updateComposeStack(id, { composeYaml: yaml });
    },
    onSuccess: () => {
      refreshed();
      setEditingYaml(false);
      toast({ tone: "success", title: "Compose file saved", message: "The stack now holds the edited document." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Compose file not saved", message: errorMessage(e) }),
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteComposeStack(id, { volumes: deleteVolumes }),
    onSuccess: () => {
      toast({
        tone: "success",
        title: "Stack deleted",
        message: deleteVolumes ? "The stack and its volumes were removed." : "The stack was removed.",
      });
      router.push("/admin/compose");
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: errorMessage(e) }),
  });

  const gitRedeployMut = useMutation({
    mutationFn: () => redeployFromGit(id),
    onSuccess: () => { refreshed(); toast({ tone: "success", title: "Git redeploy requested" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Git redeploy failed", message: errorMessage(e) }),
  });
  const gitPullMut = useMutation({
    mutationFn: () => pullAndRedeploy(id),
    onSuccess: () => { refreshed(); toast({ tone: "success", title: "Pull and redeploy requested" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Pull failed", message: errorMessage(e) }),
  });
  const gitRollbackMut = useMutation({
    mutationFn: () => rollbackCompose(id),
    onSuccess: () => { refreshed(); toast({ tone: "success", title: "Rollback requested", message: "The stack is being rolled back to its previous commit." }); },
    onError: (e: Error) => toast({ tone: "error", title: "Rollback failed", message: errorMessage(e) }),
  });
  const checkUpdateMut = useMutation({
    mutationFn: () => checkUpdate(id),
    onSuccess: (data) => {
      // The answer belongs on the panel, not in a toast that has scrolled away.
      setUpdateCheck(data);
      toast({ tone: "info", title: "Update check finished", message: "The result is shown in the GitOps panel." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Update check failed", message: errorMessage(e) }),
  });

  if (statusQuery.isPending) {
    return (
      <AdminPageLayout>
        <AdminPageHeader title="Compose stack" backAction={() => router.push("/admin/compose")} backLabel="Compose Stacks" />
        <OfflineBanner onRetry={() => window.location.reload()} />
        <AdminLoadingState label="Loading stack…" />
      </AdminPageLayout>
    );
  }

  // Every failure — 500, 403, timeout — used to render "Stack not found.",
  // reporting a record as missing when its state is simply unknown.
  if (statusQuery.isError || !stack) {
    return (
      <AdminPageLayout>
        <AdminPageHeader title="Compose stack" backAction={() => router.push("/admin/compose")} backLabel="Compose Stacks" />
        <AdminErrorState
          message={
            statusQuery.isError
              ? `This stack could not be read: ${errorMessage(statusQuery.error)}`
              : "The control plane answered without a stack record."
          }
          retry={() => void statusQuery.refetch()}
        />
        <Btn tone="ghost" onClick={() => router.push("/admin/compose")}>Back to stacks</Btn>
      </AdminPageLayout>
    );
  }

  const busy =
    startMutation.isPending || stopMutation.isPending || restartMutation.isPending || redeployMutation.isPending;

  const canStart = stack.status === "stopped";
  const canStop = stack.status === "running";
  const canRestart = stack.status === "running" || stack.status === "degraded";
  const canRedeploy = !["deploying", "deleting", "deleted"].includes(stack.status);
  // The only git signal the stored stack carries is `sourceType`. Anything else
  // is said as "not recorded as git-backed" rather than guessed at from
  // `composeType`, and the controls are disabled with that reason.
  const isGitBacked = stack.sourceType === "git";

  const drift = driftQ.data;

  return (
    <AdminPageLayout className="max-w-6xl">
      <AdminPageHeader
        // Detail route: the resource name is the title. Status is said once, as
        // the pill; it used to appear as title-subtitle *and* pill *and* a chip.
        title={stack.name}
        description={`Node ${stack.nodeId ? stack.nodeId.slice(0, 12) : "not assigned"} · ${humanToken(
          stack.composeType || stack.sourceType || "source not reported",
        )}`}
        status={<FreshnessBadge state={sourceState(statusQuery, STATUS_POLL_MS)} />}
        backAction={() => router.push("/admin/compose")}
        backLabel="Compose Stacks"
        action={
          <div className="flex flex-wrap items-center gap-2">
            <Pill tone={composeStatusTone(stack.status)}>{humanToken(stack.status) || "unknown"}</Pill>
            <Btn
              size="sm"
              tone="success"
              onClick={() => startMutation.mutate()}
              disabled={!canStart || busy}
              title={canStart ? undefined : `Cannot start while the stack is “${humanToken(stack.status)}”.`}
            >
              <Play aria-hidden="true" className="h-4 w-4" /> Start
            </Btn>
            <Btn
              size="sm"
              tone="warning"
              onClick={() => stopMutation.mutate()}
              disabled={!canStop || busy}
              title={canStop ? undefined : `Cannot stop while the stack is “${humanToken(stack.status)}”.`}
            >
              <Square aria-hidden="true" className="h-4 w-4" /> Stop
            </Btn>
            <Btn
              size="sm"
              tone="ghost"
              onClick={() => restartMutation.mutate()}
              disabled={!canRestart || busy}
              title={canRestart ? undefined : `Cannot restart while the stack is “${humanToken(stack.status)}”.`}
            >
              <RotateCcw aria-hidden="true" className="h-4 w-4" /> Restart
            </Btn>
            <Btn
              size="sm"
              tone="primary"
              onClick={() => void askRedeploy()}
              disabled={!canRedeploy || busy}
              title={canRedeploy ? undefined : `Cannot redeploy while the stack is “${humanToken(stack.status)}”.`}
            >
              <Rocket aria-hidden="true" className="h-4 w-4" /> Redeploy
            </Btn>
          </div>
        }
      />
      <OfflineBanner onRetry={() => void statusQuery.refetch()} />

      {/* One visible line covering the disabled controls in the cluster above. */}
      {!canStart || !canStop || !canRestart || !canRedeploy ? (
        <p className="text-meta text-text-subtle">
          Start, Stop, Restart and Redeploy are gated on the stack's reported state
          {` (“${humanToken(stack.status)}”). `}
          They re-enable as the agent reports a state that allows them.
        </p>
      ) : null}

      <AdminTabs
        active={tab}
        label="Stack sections"
        onChange={(next) => setTab(next as ComposeTab)}
        tabs={COMPOSE_TABS.map((t) => ({ id: t, label: TAB_LABELS[t] }))}
      />

      {stack.error ? (
        <p className="ui-alert ui-alert-danger" role="alert">
          <span className="font-semibold">Last failure:</span> {stack.error}
        </p>
      ) : null}

      {tab === "overview" ? (
        <div className="grid gap-6 lg:grid-cols-2">
          <AdminSection title="Stack">
            <Card>
              <dl className="space-y-3 p-4 text-sm">
                <Row label="ID">{mono(stack.id)}</Row>
                <Row label="Type">{humanToken(stack.composeType) || "Not reported"}</Row>
                <Row label="Source">{humanToken(stack.sourceType) || "Not reported"}</Row>
                <Row label="Node">{stack.nodeId ? mono(stack.nodeId.slice(0, 12)) : "Not assigned"}</Row>
                <Row label="Group">
                  {stack.environmentId ? mono(stack.environmentId.slice(0, 12)) : "Not reported"}
                </Row>
                <Row label="Created">{stack.createdAt ? formatDate(stack.createdAt) : "Not reported"}</Row>
                <Row label="Updated">{stack.updatedAt ? formatDate(stack.updatedAt) : "Not reported"}</Row>
              </dl>
            </Card>
          </AdminSection>

          <AdminSection title="Recorded limits">
            <Card>
              <dl className="space-y-3 p-4 text-sm">
                <Row label="Memory">{numberOrNotReported(stack.memoryMb, "MB")}</Row>
                <Row label="CPU">{numberOrNotReported(stack.cpuShares, "shares")}</Row>
                <Row label="Disk">{numberOrNotReported(stack.diskMb, "MB")}</Row>
              </dl>
              <p className="px-4 pb-4 text-meta text-text-subtle">
                These are the limits stored for the stack, not live usage.{" "}
                {stack.reservationId ? `Reservation ${stack.reservationId.slice(0, 12)}.` : "No reservation recorded."}
              </p>
            </Card>

            <div className="mt-4">
              <AdminSection title="Remove this stack" description="Destructive and irreversible.">
                <Card className="space-y-3 p-4">
                  <label className="flex items-start gap-2 text-meta text-text-subtle">
                    <input
                      checked={deleteVolumes}
                      className="mt-0.5 accent-[var(--brand)]"
                      onChange={(e) => setDeleteVolumes(e.target.checked)}
                      type="checkbox"
                    />
                    <span>
                      Also remove the stack&apos;s volumes. Left unchecked the volumes survive the delete.
                    </span>
                  </label>
                  <Btn
                    size="sm"
                    tone="danger"
                    onClick={() => void askDelete()}
                  >
                    <Trash2 aria-hidden="true" className="h-4 w-4" /> Delete stack
                  </Btn>
                </Card>
              </AdminSection>
            </div>
          </AdminSection>
        </div>
      ) : null}

      {tab === "services" ? (
        <AdminSection
          title="Services"
          description="Container state as the agent last reported it."
          action={
            statusQuery.isFetching ? <span className="text-meta text-text-subtle">Refreshing…</span> : null
          }
        >
          <Card>
            {safeServices.length === 0 ? (
              <div className="p-4">
                <EmptyState
                  title="No services reported"
                  message="The agent reported no containers for this stack. If the stack was never deployed, start or redeploy it."
                />
              </div>
            ) : (
              <AdminTable label="Compose services">
                <AdminTHead>
                  <AdminTh>Service</AdminTh>
                  <AdminTh>Status</AdminTh>
                  <AdminTh>Image</AdminTh>
                  <AdminTh>Ports</AdminTh>
                </AdminTHead>
                <AdminTBody>
                  {safeServices.map((svc) => {
                    const reported = svc.status || svc.state;
                    return (
                      <AdminTr key={svc.name}>
                        <AdminTd className="text-sm">{svc.name || "Unnamed"}</AdminTd>
                        <AdminTd>
                          {/* `status` and `state` are two names for one reading; showing
                              both disagreed when only one was populated. */}
                          <Pill tone={composeStatusTone(reported)}>
                            {reported ? humanToken(reported) : "Not reported"}
                          </Pill>
                        </AdminTd>
                        <AdminTd className="max-w-96 break-all font-mono text-meta">
                          {svc.image || "Not reported"}
                        </AdminTd>
                        <AdminTd className="font-mono text-meta">{svc.ports || "None reported"}</AdminTd>
                      </AdminTr>
                    );
                  })}
                </AdminTBody>
              </AdminTable>
            )}
          </Card>
        </AdminSection>
      ) : null}

      {tab === "logs" ? (
        <AdminSection
          title="Logs"
          description="The last lines the agent returned for this stack."
          action={
            <div className="flex flex-wrap items-end gap-3">
              <AdminSelect
                label="Service"
                value={logService}
                onChange={setLogService}
                options={[{ value: "", label: "All services" }, ...safeServices.map((s) => ({ value: s.name, label: s.name }))]}
              />
              <AdminSelect
                label="Lines"
                value={String(logTail)}
                onChange={(v) => setLogTail(Number(v))}
                options={[
                  { value: "50", label: "50 lines" },
                  { value: "100", label: "100 lines" },
                  { value: "500", label: "500 lines" },
                ]}
              />
            </div>
          }
        >
          <Card className="p-4">
            {logsQuery.isPending ? (
              <AdminLoadingState label="Loading logs…" />
            ) : logsQuery.isError ? (
              <AdminErrorState
                message={`Logs could not be read: ${errorMessage(logsQuery.error)}`}
                retry={() => void logsQuery.refetch()}
              />
            ) : safeLogs.length === 0 ? (
              <p className="text-sm text-text-subtle">
                The log request completed and returned nothing for{" "}
                {logService ? `service “${logService}”` : "this stack"}. That is an empty log, not a failure.
              </p>
            ) : (
              <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all font-mono text-meta text-text-subtle">
                {safeLogs.map((line, i) => (
                  <div key={`${i}-${line.slice(0, 12)}`}>{line}</div>
                ))}
              </pre>
            )}
          </Card>
        </AdminSection>
      ) : null}

      {tab === "yaml" ? (
        <AdminSection
          title="Compose file"
          description="The stored document the agent deploys from."
          action={
            <div className="flex items-center gap-2">
              <Btn
                size="sm"
                tone="ghost"
                onClick={() => { setEditYaml(stack.composeYaml); setEditingYaml(true); }}
              >
                <Pencil aria-hidden="true" className="h-4 w-4" /> Edit
              </Btn>
            </div>
          }
        >
          <Card className="p-4">
            <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all font-mono text-meta text-text-subtle">
              {stack.composeYaml || "The stack has no stored compose document."}
            </pre>
          </Card>
        </AdminSection>
      ) : null}

      {tab === "gitops" ? (
        <AdminSection title="GitOps" description="Operations the git-backed stack exposes.">
          <Card className="space-y-4 p-4">
            {stack.sourceType !== "git" ? (
              <p className="text-meta text-text-subtle">
                This stack is recorded as source type “{humanToken(stack.sourceType) || "unknown"}”, not
                git, so the controls below are disabled: there is no repository for the agent to pull from.
              </p>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Btn size="sm" tone="ghost" onClick={() => void askGit("redeploy")} disabled={!isGitBacked || gitRedeployMut.isPending}>
                Redeploy from git
              </Btn>
              <Btn size="sm" tone="ghost" onClick={() => void askGit("pull")} disabled={!isGitBacked || gitPullMut.isPending}>
                Pull and redeploy
              </Btn>
              <Btn size="sm" tone="warning" onClick={() => void askGit("rollback")} disabled={!isGitBacked || gitRollbackMut.isPending}>
                Roll back to previous commit
              </Btn>
              <Btn size="sm" tone="ghost" onClick={() => checkUpdateMut.mutate()} disabled={!isGitBacked || checkUpdateMut.isPending}>
                Check for updates
              </Btn>
              <Btn size="sm" tone="ghost" onClick={() => void gitStatusQ.refetch()}>
                Refresh git status
              </Btn>
              <Btn size="sm" tone="ghost" onClick={() => void driftQ.refetch()}>
                Re-check drift
              </Btn>
            </div>

            {/* The question the panel exists to answer, stated as a verdict rather
                than emitted as a payload. */}
            {driftQ.isPending ? (
              <p className="text-meta text-text-subtle" role="status">Checking drift…</p>
            ) : driftQ.isError ? (
              <AdminErrorState
                message={`Drift could not be checked: ${errorMessage(driftQ.error)}`}
                retry={() => void driftQ.refetch()}
              />
            ) : drift ? (
              <div className="space-y-2">
                <p className="ui-alert ui-alert-warning flex-wrap items-center gap-2" role="status">
                  {drift.hasDrift ? (
                    <>
                      <GitCompare aria-hidden="true" className="h-4 w-4 shrink-0" />
                      <span>
                        <span className="font-semibold">Drifted.</span> The stack is serving{" "}
                        <code className="font-mono">{short(drift.deployedSha)}</code> and the repository is at{" "}
                        <code className="font-mono">{short(drift.currentSha)}</code>.
                      </span>
                    </>
                  ) : (
                    <span>
                      <span className="font-semibold">Not drifted.</span> Serving{" "}
                      <code className="font-mono">{short(drift.deployedSha)}</code>, which matches the repository.
                    </span>
                  )}
                </p>
                {Array.isArray(drift.servicesDiff) && drift.servicesDiff.length > 0 ? (
                  <AdminTable label="Drifted services">
                    <AdminTHead>
                      <AdminTh>Service</AdminTh>
                      <AdminTh>Change</AdminTh>
                      <AdminTh>Detail</AdminTh>
                    </AdminTHead>
                    <AdminTBody>
                      {drift.servicesDiff.map((d, i) => (
                        <AdminTr key={`${d.service ?? "service"}-${i}`}>
                          <AdminTd className="text-sm">{d.service || "Unnamed"}</AdminTd>
                          <AdminTd className="text-meta">{d.change ? humanToken(d.change) : "Not reported"}</AdminTd>
                          <AdminTd className="max-w-96 break-words text-meta">{d.detail || "—"}</AdminTd>
                        </AdminTr>
                      ))}
                    </AdminTBody>
                  </AdminTable>
                ) : null}
                <button
                  className="text-meta text-text-subtle underline underline-offset-2"
                  onClick={() => setShowRawDrift((v) => !v)}
                  type="button"
                >
                  {showRawDrift ? "Hide raw drift response" : "Show raw drift response"}
                </button>
                {showRawDrift ? <RawPayload payload={drift} /> : null}
              </div>
            ) : null}

            {updateCheck !== null ? (
              <div className="space-y-1">
                <p className="text-meta text-text-subtle" role="status">Update check result</p>
                <RawPayload payload={updateCheck} />
              </div>
            ) : null}

            {gitStatusQ.isError ? (
              <p className="text-meta text-text-subtle" role="status">
                Git status is unavailable — {errorMessage(gitStatusQ.error)}
              </p>
            ) : gitStatusQ.isPending ? (
              <p className="text-meta text-text-subtle" role="status">Loading git status…</p>
            ) : gitStatusQ.data ? (
              <div className="space-y-1">
                <button
                  className="text-meta text-text-subtle underline underline-offset-2"
                  onClick={() => setShowRawGit((v) => !v)}
                  type="button"
                >
                  {showRawGit ? "Hide raw git status" : "Show raw git status"}
                </button>
                {showRawGit ? <RawPayload payload={gitStatusQ.data} /> : null}
              </div>
            ) : null}

            {webhookQ.isPending ? (
              <p className="text-meta text-text-subtle" role="status">Checking the last webhook…</p>
            ) : webhookQ.isError ? (
              <p className="text-meta text-text-subtle" role="status">
                Last webhook could not be read: {errorMessage(webhookQ.error)}
              </p>
            ) : (
              <p className="text-meta text-text-subtle">
                {webhookQ.data?.lastWebhookAt
                  ? `Last webhook received ${formatDate(webhookQ.data.lastWebhookAt)}.`
                  : "No webhook delivery has been recorded for this stack."}
              </p>
            )}
          </Card>
        </AdminSection>
      ) : null}

      {editingYaml ? (
        <Modal
          title="Edit compose file"
          description="The document is validated before it is saved; an invalid file is not written to the stack."
          onClose={() => setEditingYaml(false)}
          wide
        >
          <div className="space-y-3">
            <label className="block">
              <span className="ui-label mb-1.5">Compose YAML</span>
              <textarea
                className="ui-input min-h-0 w-full font-mono"
                onChange={(e) => setEditYaml(e.target.value)}
                placeholder="services: …"
                rows={20}
                value={editYaml}
              />
            </label>
            {updateMutation.isError ? (
              <AdminErrorState message={errorMessage(updateMutation.error, "The file was not saved.")} />
            ) : null}
          </div>
          <ModalFooter
            onCancel={() => setEditingYaml(false)}
            onConfirm={() => updateMutation.mutate(editYaml)}
            disabled={updateMutation.isPending || !editYaml.trim()}
            confirmLabel={updateMutation.isPending ? "Saving…" : "Save file"}
          />
        </Modal>
      ) : null}

      {renderConfirm()}
    </AdminPageLayout>
  );

  async function askRedeploy() {
    const ok = await confirm({
      title: "Redeploy this stack?",
      description: `“${stack?.name ?? "this stack"}” is torn down and rebuilt from its stored compose document on node ${
        stack?.nodeId ? stack.nodeId.slice(0, 12) : "the assigned node"
      }. Running containers restart.`,
      danger: true,
      confirmLabel: "Redeploy",
    });
    if (ok) redeployMutation.mutate();
  }

  async function askDelete() {
    const ok = await confirm({
      title: `Delete “${stack?.name ?? "this stack"}”?`,
      description: deleteVolumes
        ? "The stack, its services and its volumes will be removed. This cannot be undone."
        : "The stack and its services will be removed. Its volumes are kept. This cannot be undone.",
      danger: true,
      confirmLabel: "Delete",
    });
    if (ok) deleteMutation.mutate();
  }

  async function askGit(kind: "redeploy" | "pull" | "rollback") {
    const copy = {
      redeploy: {
        title: "Redeploy from git?",
        description: "The stack is redeployed from the commit it currently tracks.",
        label: "Redeploy",
      },
      pull: {
        title: "Pull and redeploy?",
        description: "The repository is fetched again and the stack moves to the newest commit on its branch.",
        label: "Pull and redeploy",
      },
      rollback: {
        title: "Roll back to the previous commit?",
        description:
          "The stack is redeployed from the commit it last served, and the current one is abandoned. Health checks run against the rolled-back stack.",
        label: "Roll back",
      },
    }[kind];
    const ok = await confirm({ title: copy.title, description: copy.description, danger: true, confirmLabel: copy.label });
    if (!ok) return;
    if (kind === "redeploy") gitRedeployMut.mutate();
    if (kind === "pull") gitPullMut.mutate();
    if (kind === "rollback") gitRollbackMut.mutate();
  }
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <dt className="text-text-subtle">{label}</dt>
      <dd className="min-w-0 break-all text-right text-text">{children}</dd>
    </div>
  );
}

function mono(value: string): ReactNode {
  return <code className="font-mono text-meta">{value}</code>;
}

function short(sha?: string): string {
  return sha ? sha.slice(0, 12) : "not reported";
}

function RawPayload({ payload }: { payload: unknown }) {
  return (
    <pre className="max-h-60 overflow-auto rounded-lg border border-line bg-overlay-subtle p-3 font-mono text-meta text-text-subtle">
      {JSON.stringify(payload, null, 2)}
    </pre>
  );
}

export default function ComposeStackDetailPage() {
  return (
    <Suspense fallback={<AdminLoadingState label="Loading compose stack…" />}>
      <ComposeStackDetailContent />
    </Suspense>
  );
}
