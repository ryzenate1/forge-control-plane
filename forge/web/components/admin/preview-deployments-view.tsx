"use client";

// Project-scoped preview environments panel.
//
// Talks to /api/v1/projects/:id/previews (see lib/api/preview-deployments.ts).
// Provisions are asynchronous on the server: a row is created as `pending` and
// moves to `active`/`failed` in the background, so the list polls instead of
// assuming a create or redeploy already succeeded.

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock3, ExternalLink, GitBranch, KeyRound, RefreshCw, RotateCcw, Trash2, Webhook, XCircle } from "lucide-react";
import {
  AdminErrorState,
  AdminLoadingRows,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
  Textarea,
} from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { previewStatusTone } from "@/lib/api/status";
import { fetchOrganizations, fetchProjects } from "@/lib/api/tenancy";
import {
  closeProjectPreview,
  createProjectPreview,
  deleteProjectPreview,
  fetchProjectPreviewConfig,
  fetchProjectPreviewWebhookSecret,
  listProjectPreviews,
  redeployProjectPreview,
  rotateProjectPreviewWebhookSecret,
  type CreateProjectPreviewInput,
  type ProjectPreviewDeployment,
} from "@/lib/api/preview-deployments";
import { errorMessage, formatDate } from "@/lib/utils";

/** Statuses where the environment is expected to exist (or is being built). */
const LIVE_STATUSES = new Set(["pending", "deploying", "active"]);

function isLive(preview: ProjectPreviewDeployment): boolean {
  return LIVE_STATUSES.has(preview.status);
}

function wholeNumber(raw: string): number | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed)) return null;
  return Math.trunc(parsed);
}

/** Ticking clock so the expiry countdown moves without refetching the list. */
function useNow(enabled: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!enabled) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [enabled]);
  return now;
}

function formatCountdown(expiresAt: string | undefined, now: number): { text: string; tone: "green" | "yellow" | "red" | "neutral" } {
  if (!expiresAt) return { text: "No expiry", tone: "neutral" };
  const target = Date.parse(expiresAt);
  if (!Number.isFinite(target)) return { text: "Unknown expiry", tone: "neutral" };
  const remaining = target - now;
  if (remaining <= 0) return { text: "Expired", tone: "red" };
  const seconds = Math.floor(remaining / 1000);
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;
  const text = days > 0
    ? `${days}d ${hours}h`
    : hours > 0
      ? `${hours}h ${minutes}m`
      : minutes > 0
        ? `${minutes}m ${secs}s`
        : `${secs}s`;
  return { text, tone: remaining < 3_600_000 ? "red" : remaining < 86_400_000 ? "yellow" : "green" };
}

function formatTtl(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "uncapped";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  if (days > 0) return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  return `${Math.max(minutes, 1)}m`;
}

function PreviewStatusPill({ status }: { status: ProjectPreviewDeployment["status"] }) {
  return <Pill tone={previewStatusTone(status)}>{status}</Pill>;
}

function CreatePreviewDialog({
  onClose,
  onCreate,
  busy,
}: {
  onClose: () => void;
  onCreate: (input: CreateProjectPreviewInput) => void;
  busy: boolean;
}) {
  const [branch, setBranch] = useState("");
  const [prNumber, setPrNumber] = useState("");
  const [commitSha, setCommitSha] = useState("");
  const [title, setTitle] = useState("");
  const [prUrl, setPrUrl] = useState("");
  const [ttlHours, setTtlHours] = useState("");
  const [baseStackId, setBaseStackId] = useState("");
  const [composeContent, setComposeContent] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  // Validation copy stays hidden until the user actually tries to submit, so an
  // empty form does not open covered in red errors.
  const [attempted, setAttempted] = useState(false);

  const branchError = branch.trim() ? null : "Branch is required — it identifies the preview.";
  const prNumberValue = wholeNumber(prNumber);
  const prNumberError = prNumber.trim() && prNumberValue === null ? "PR number must be a whole number." : null;
  const ttlValue = wholeNumber(ttlHours);
  const ttlError = ttlHours.trim() && (ttlValue === null || ttlValue <= 0) ? "TTL must be a positive number of hours." : null;
  const invalid = Boolean(branchError || prNumberError || ttlError);

  function submit() {
    setAttempted(true);
    if (invalid) return;
    const ttlSeconds = ttlValue && ttlValue > 0 ? ttlValue * 3600 : undefined;
    onCreate({
      branch: branch.trim(),
      prNumber: prNumberValue && prNumberValue > 0 ? prNumberValue : undefined,
      commitSha: commitSha.trim() || undefined,
      title: title.trim() || undefined,
      prUrl: prUrl.trim() || undefined,
      baseStackId: baseStackId.trim() || undefined,
      composeContent: composeContent.trim() || undefined,
      ttlSeconds,
    });
  }

  return (
    <Modal description="Creates an ephemeral environment on its own subdomain. The deployment runs in the background; the row reports progress." onClose={onClose} title="New preview environment">
      <div className="grid gap-4">
        <Input label="Branch" value={branch} onChange={setBranch} placeholder="feature/checkout-flow" mono required />
        {attempted && branchError ? <p className="-mt-2 text-sm text-danger">{branchError}</p> : null}
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label="Pull request number" value={prNumber} onChange={setPrNumber} type="number" placeholder="42" />
          <Input label="Commit SHA" value={commitSha} onChange={setCommitSha} placeholder="Optional — deploy the branch head" mono />
        </div>
        {attempted && prNumberError ? <p className="-mt-2 text-sm text-danger">{prNumberError}</p> : null}
        <Input label="Title" value={title} onChange={setTitle} placeholder="Optional label shown in the list" />
        <Input label="Pull request URL" value={prUrl} onChange={setPrUrl} placeholder="https://github.com/…" mono />
        <Input label="Lifetime (hours)" value={ttlHours} onChange={setTtlHours} type="number" placeholder="Blank uses the platform default" />
        {attempted && ttlError ? <p className="-mt-2 text-sm text-danger">{ttlError}</p> : null}
        <Btn tone="subtle" size="sm" onClick={() => setShowAdvanced((prev) => !prev)}>{showAdvanced ? "Hide advanced" : "Show advanced"}</Btn>
        {showAdvanced ? (
          <>
            <Input label="Base compose stack id" value={baseStackId} onChange={setBaseStackId} placeholder="Optional — required when the project has several stacks" mono />
            <Textarea label="Compose override" value={composeContent} onChange={setComposeContent} rows={6} placeholder="Optional YAML. Leave blank to clone the base stack." />
          </>
        ) : null}
        <p className="text-xs leading-5 text-text-subtle">
          Without an override, Forge deploys a copy of the project&apos;s compose stack. Tearing the preview down removes the copy, never the base stack.
        </p>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={submit} confirmLabel={busy ? "Queuing…" : "Create preview"} disabled={busy} />
    </Modal>
  );
}

function PreviewConfigCard({
  projectId,
  baseDomain,
  ttlSeconds,
  maxPerProject,
  webhookUrl,
  error,
  onRotate,
  rotating,
}: {
  projectId: string;
  baseDomain: string;
  ttlSeconds: number;
  maxPerProject: number;
  webhookUrl: string;
  error: string | null;
  onRotate: () => void;
  rotating: boolean;
}) {
  const { toast } = useToast();
  const [revealed, setRevealed] = useState(false);
  const secretQuery = useQuery({
    queryKey: ["project-preview-webhook-secret", projectId],
    queryFn: () => fetchProjectPreviewWebhookSecret(projectId),
    enabled: revealed,
    staleTime: 60_000,
  });

  async function copy(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      toast({ tone: "success", title: `${label} copied` });
    } catch {
      toast({ tone: "error", title: "Copy failed", message: "Your browser blocked clipboard access; select the text manually." });
    }
  }

  const secret = secretQuery.data?.secret ?? "";

  return (
    <Card>
      <CardHeader title="How previews are reached" icon={Webhook} action={
        <Btn size="sm" tone="ghost" onClick={onRotate} loading={rotating} title="Invalidates the current signing secret immediately">
          <KeyRound size={13} /> Rotate secret
        </Btn>
      } />
      <dl className="grid gap-4 sm:grid-cols-2">
        <div>
          <dt className="text-xs font-semibold uppercase tracking-wider text-text-muted">Preview domain</dt>
          <dd className="mt-1 break-all font-mono text-sm text-text">
            {baseDomain ? <>{"<slug>."}<span className="text-text-subtle">{baseDomain}</span></> : "not configured on this control plane"}
          </dd>
        </div>
        <div>
          <dt className="text-xs font-semibold uppercase tracking-wider text-text-muted">Lifetime / cap</dt>
          <dd className="mt-1 text-sm text-text">
            {formatTtl(ttlSeconds)} · {maxPerProject > 0 ? `${maxPerProject} live per project` : "no live cap"}
          </dd>
        </div>
        <div className="sm:col-span-2">
          <dt className="text-xs font-semibold uppercase tracking-wider text-text-muted">Webhook endpoint</dt>
          <dd className="mt-1 flex flex-wrap items-center gap-2">
            <code className="break-all rounded-lg border border-line bg-[var(--surface-input)] px-2 py-1 font-mono text-xs text-text">{webhookUrl || "not configured"}</code>
            {webhookUrl ? <Btn size="sm" tone="subtle" onClick={() => void copy(webhookUrl, "Webhook URL")}>Copy</Btn> : null}
          </dd>
        </div>
        <div className="sm:col-span-2">
          <dt className="text-xs font-semibold uppercase tracking-wider text-text-muted">Signing secret</dt>
          <dd className="mt-1 flex flex-wrap items-center gap-2">
            {revealed ? (
              secretQuery.isLoading
                ? <span className="text-sm text-text-subtle">Loading…</span>
                : <code className="break-all rounded-lg border border-line bg-[var(--surface-input)] px-2 py-1 font-mono text-xs text-text">{secret || "not generated yet"}</code>
            ) : (
              <code className="rounded-lg border border-line bg-[var(--surface-input)] px-2 py-1 font-mono text-xs text-text-muted">••••••••••••••••</code>
            )}
            {revealed && secret ? <Btn size="sm" tone="subtle" onClick={() => void copy(secret, "Signing secret")}>Copy</Btn> : null}
            <Btn size="sm" tone="ghost" onClick={() => setRevealed((prev) => !prev)}>{revealed ? "Hide" : "Reveal"}</Btn>
          </dd>
          <p className="mt-2 text-xs leading-5 text-text-subtle">
            GitHub: send <code className="font-mono">pull_request</code> events with an <code className="font-mono">X-Hub-Signature-256</code> HMAC of this secret.
            GitLab: send <code className="font-mono">Merge Request Events</code> with the secret as <code className="font-mono">X-Gitlab-Token</code>.
            Closing or merging a pull request, or deleting its branch, tears the preview down.
          </p>
        </div>
      </dl>
      {error ? <div className="mt-4"><AdminErrorState message={error} /></div> : null}
      {secretQuery.isError ? <div className="mt-4"><AdminErrorState message={errorMessage(secretQuery.error, "Could not load the signing secret.")} retry={() => void secretQuery.refetch()} /></div> : null}
    </Card>
  );
}

export function PreviewDeploymentsView({ projectId: fixedProjectId, className }: { projectId?: string; className?: string }) {
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const queryClient = useQueryClient();

  const [orgId, setOrgId] = useState("");
  const [pickedProjectId, setPickedProjectId] = useState("");
  const [filter, setFilter] = useState<"live" | "all" | "ended">("live");
  const [showCreate, setShowCreate] = useState(false);

  const projectId = fixedProjectId || pickedProjectId;

  const organizationsQuery = useQuery({
    queryKey: ["tenancy", "organizations"],
    queryFn: fetchOrganizations,
    enabled: !fixedProjectId,
  });
  const organizations = useMemo(() => organizationsQuery.data ?? [], [organizationsQuery.data]);
  // The list defaults to the caller's first organization so a single-org panel
  // works without a click, but the project list still needs an explicit pick.
  const activeOrgId = fixedProjectId ? "" : (orgId || (organizations[0]?.id ?? ""));

  const projectsQuery = useQuery({
    queryKey: ["tenancy", "projects", activeOrgId],
    queryFn: () => fetchProjects(activeOrgId),
    enabled: Boolean(activeOrgId),
  });
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);

  const configQuery = useQuery({
    queryKey: ["project-preview-config", projectId],
    queryFn: () => fetchProjectPreviewConfig(projectId),
    enabled: Boolean(projectId),
    staleTime: 5 * 60_000,
  });

  const previewsQuery = useQuery({
    queryKey: ["project-previews", projectId],
    queryFn: () => listProjectPreviews(projectId),
    enabled: Boolean(projectId),
    // Provisioning is asynchronous server-side, so the list is the source of
    // truth for "did this environment actually come up".
    refetchInterval: 10_000,
  });

  const previews = useMemo(() => {
    const items = previewsQuery.data ?? [];
    if (filter === "live") return items.filter(isLive);
    if (filter === "ended") return items.filter((item) => !isLive(item));
    return items;
  }, [previewsQuery.data, filter]);

  const anyLive = useMemo(() => (previewsQuery.data ?? []).some(isLive), [previewsQuery.data]);
  const now = useNow(anyLive);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["project-previews", projectId] });
  };

  const createMutation = useMutation({
    mutationFn: (input: CreateProjectPreviewInput) => createProjectPreview(projectId, input),
    onSuccess: (preview) => {
      invalidate();
      setShowCreate(false);
      toast({ tone: "success", title: "Preview queued", message: `${preview.branch} → ${preview.slug} (status: ${preview.status})` });
    },
    onError: (error) => toast({ tone: "error", title: "Could not create preview", message: errorMessage(error, "An unexpected error occurred.") }),
  });

  const redeployMutation = useMutation({
    mutationFn: (id: string) => redeployProjectPreview(projectId, id),
    onSuccess: (preview) => {
      invalidate();
      toast({ tone: "success", title: "Redeploy queued", message: `${preview.branch} (status: ${preview.status})` });
    },
    onError: (error) => toast({ tone: "error", title: "Could not redeploy preview", message: errorMessage(error, "An unexpected error occurred.") }),
  });

  const closeMutation = useMutation({
    mutationFn: (id: string) => closeProjectPreview(projectId, id, "closed from the panel"),
    onSuccess: () => {
      invalidate();
      toast({ tone: "success", title: "Preview torn down", message: "The row is kept as history with status teardown." });
    },
    onError: (error) => toast({ tone: "error", title: "Could not close preview", message: errorMessage(error, "An unexpected error occurred.") }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteProjectPreview(projectId, id),
    onSuccess: () => {
      invalidate();
      toast({ tone: "success", title: "Preview removed" });
    },
    onError: (error) => toast({ tone: "error", title: "Could not delete preview", message: errorMessage(error, "An unexpected error occurred.") }),
  });

  const rotateMutation = useMutation({
    mutationFn: () => rotateProjectPreviewWebhookSecret(projectId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-preview-webhook-secret", projectId] });
      toast({ tone: "success", title: "Signing secret rotated", message: "Update the provider webhook configuration — old signatures are rejected." });
    },
    onError: (error) => toast({ tone: "error", title: "Could not rotate secret", message: errorMessage(error, "An unexpected error occurred.") }),
  });

  const projectOptions = projects.map((project) => ({ value: project.id, label: project.name ? `${project.name} · ${project.slug}` : project.id }));

  return (
    <div className={className}>
      <SectionHeader
        title="Preview Environments"
        sub="Ephemeral preview deployments with their own subdomain, scoped to a project. Created by a pull-request webhook or by hand, destroyed when the branch lands, closes, or expires."
        status={projectId ? <FreshnessBadge state={sourceState(previewsQuery, 10_000)} /> : undefined}
        action={projectId ? <Btn onClick={() => setShowCreate(true)}><GitBranch size={14} /> New preview</Btn> : undefined}
      />

      {!fixedProjectId ? (
        <Card className="mb-5">
          <CardHeader title="Project" icon={GitBranch} />
          <div className="grid gap-4 sm:grid-cols-2">
            <AdminSelect
              label="Organization"
              value={activeOrgId}
              onChange={(value) => { setOrgId(value); setPickedProjectId(""); }}
              options={organizations.map((org) => ({ value: org.id, label: org.name || org.slug }))}
              placeholder={organizationsQuery.isLoading ? "Loading organizations…" : "Select an organization"}
            />
            <AdminSelect
              label="Project"
              value={pickedProjectId}
              onChange={setPickedProjectId}
              options={projectOptions}
              placeholder={projectsQuery.isLoading ? "Loading projects…" : projectOptions.length ? "Select a project" : "No projects in this organization"}
              disabled={!activeOrgId}
            />
          </div>
          {organizationsQuery.isError ? <div className="mt-4"><AdminErrorState message={errorMessage(organizationsQuery.error, "Could not load organizations.")} retry={() => void organizationsQuery.refetch()} /></div> : null}
          {projectsQuery.isError ? <div className="mt-4"><AdminErrorState message={errorMessage(projectsQuery.error, "Could not load projects.")} retry={() => void projectsQuery.refetch()} /></div> : null}
        </Card>
      ) : null}

      {projectId ? (
        <>
          <div className="mb-5">
            <PreviewConfigCard
              projectId={projectId}
              baseDomain={configQuery.data?.baseDomain ?? ""}
              ttlSeconds={configQuery.data?.ttlSeconds ?? 0}
              maxPerProject={configQuery.data?.maxPerProject ?? 0}
              webhookUrl={configQuery.data?.webhookUrl ?? ""}
              error={configQuery.isError ? errorMessage(configQuery.error, "Could not load the preview configuration.") : null}
              onRotate={() => rotateMutation.mutate()}
              rotating={rotateMutation.isPending}
            />
          </div>

          <Card>
            <CardHeader
              title="Environments"
              icon={Clock3}
              action={(
                <div className="flex items-center gap-2">
                  <AdminSelect
                    value={filter}
                    onChange={(value) => setFilter(value as "live" | "all" | "ended")}
                    options={[{ value: "live", label: "Live" }, { value: "all", label: "All" }, { value: "ended", label: "Ended" }]}
                  />
                  <Btn size="sm" tone="ghost" onClick={() => void previewsQuery.refetch()} disabled={previewsQuery.isFetching}>
                    <RefreshCw size={13} /> {previewsQuery.isFetching ? "Refreshing…" : "Refresh"}
                  </Btn>
                </div>
              )}
            />
            {previewsQuery.isLoading ? (
              <AdminLoadingRows rows={3} cols={5} label="Loading preview environments…" />
            ) : previewsQuery.isError ? (
              <AdminErrorState
                message={errorMessage(previewsQuery.error, "Could not load preview environments.")}
                retry={() => void previewsQuery.refetch()}
              />
            ) : previews.length === 0 ? (
              <EmptyState
                icon={GitBranch}
                title={filter === "live" ? "No live previews" : "No preview environments"}
                sub={filter === "live" ? "Point a pull-request webhook at the endpoint above, or create a preview by hand." : "Nothing here for this filter."}
              />
            ) : (
              <AdminTable label="Preview environments">
                <AdminTHead>
                  <AdminTh>Branch</AdminTh>
                  <AdminTh>Status</AdminTh>
                  <AdminTh>URL</AdminTh>
                  <AdminTh>Expires</AdminTh>
                  <AdminTh className="text-right">Actions</AdminTh>
                </AdminTHead>
                <AdminTBody>
                  {previews.map((preview) => {
                    const countdown = formatCountdown(preview.expiresAt, now);
                    const mutating = redeployMutation.isPending || closeMutation.isPending || deleteMutation.isPending;
                    // Mirrors the server: a provision already in flight must not
                    // be stacked, and a torn-down preview lost its hostname, so
                    // it has to be recreated rather than redeployed.
                    const canRedeploy = preview.status !== "teardown" && preview.status !== "pending" && preview.status !== "deploying";
                    return (
                      <AdminTr key={preview.id}>
                        <AdminTd>
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-medium text-text">{preview.branch}</span>
                            {preview.prNumber ? <Pill tone="neutral">PR #{preview.prNumber}</Pill> : null}
                            {preview.source ? <Pill tone="neutral">{preview.source}</Pill> : null}
                          </div>
                          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-text-subtle">
                            <span className="font-mono">{preview.slug}</span>
                            {preview.commitSha ? <span className="font-mono">{preview.commitSha.slice(0, 8)}</span> : null}
                            {preview.title ? <span>{preview.title}</span> : null}
                            {preview.prUrl ? (
                              <a className="inline-flex items-center gap-1 text-text underline decoration-white/20 hover:text-text" href={preview.prUrl} rel="noreferrer noopener" target="_blank">
                                pull request <ExternalLink size={11} />
                              </a>
                            ) : null}
                          </div>
                          {preview.error ? <p className="mt-1 text-xs text-danger">{preview.error}</p> : null}
                          {preview.closeReason ? <p className="mt-1 text-xs text-text-muted">Closed: {preview.closeReason}</p> : null}
                        </AdminTd>
                        <AdminTd><PreviewStatusPill status={preview.status} /></AdminTd>
                        <AdminTd>
                          {preview.url ? (
                            <a className="inline-flex items-center gap-1 break-all font-mono text-xs text-text underline decoration-white/20 hover:text-text" href={preview.url} rel="noreferrer noopener" target="_blank">
                              {preview.url.replace(/^https?:\/\//, "")} <ExternalLink size={11} />
                            </a>
                          ) : <span className="text-text-muted">—</span>}
                          <p className="mt-1 text-xs text-text-muted">Updated {formatDate(preview.updatedAt)}</p>
                        </AdminTd>
                        <AdminTd>
                          <Pill tone={countdown.tone}>{countdown.text}</Pill>
                          {preview.expiresAt ? <p className="mt-1 text-xs text-text-muted">{formatDate(preview.expiresAt)}</p> : null}
                        </AdminTd>
                        <AdminTd className="text-right">
                          <div className="flex flex-wrap justify-end gap-2">
                            <Btn
                              size="sm"
                              tone="ghost"
                              disabled={mutating || !canRedeploy}
                              loading={redeployMutation.isPending && redeployMutation.variables === preview.id}
                              title={canRedeploy ? "Deploy this branch again and extend the lifetime" : "Unavailable while a provision is in flight, or after teardown"}
                              onClick={() => redeployMutation.mutate(preview.id)}
                            >
                              <RotateCcw size={13} /> Redeploy
                            </Btn>
                            <Btn
                              size="sm"
                              tone="warning"
                              disabled={mutating || !isLive(preview)}
                              loading={closeMutation.isPending && closeMutation.variables === preview.id}
                              title="Destroy the environment, keep the history row"
                              onClick={async () => {
                                if (await confirm({ title: "Tear down this preview?", description: `${preview.branch} at ${preview.url || preview.slug}. The base stack is untouched.`, confirmLabel: "Tear down", danger: true })) {
                                  closeMutation.mutate(preview.id);
                                }
                              }}
                            >
                              <XCircle size={13} /> Close
                            </Btn>
                            <Btn
                              size="sm"
                              tone="danger"
                              disabled={mutating}
                              loading={deleteMutation.isPending && deleteMutation.variables === preview.id}
                              title="Destroy the environment and remove the row"
                              onClick={async () => {
                                if (await confirm({ title: "Delete this preview record?", description: isLive(preview) ? `The running environment for ${preview.branch} is destroyed first, then the record is removed.` : `The record for ${preview.branch} is removed.`, confirmLabel: "Delete", danger: true })) {
                                  deleteMutation.mutate(preview.id);
                                }
                              }}
                            >
                              <Trash2 size={13} /> Delete
                            </Btn>
                          </div>
                        </AdminTd>
                      </AdminTr>
                    );
                  })}
                </AdminTBody>
              </AdminTable>
            )}
          </Card>
        </>
      ) : (
        <Card>
          <EmptyState icon={GitBranch} title="Select a project" sub="Preview environments belong to a project; pick one above to list and manage its environments." />
        </Card>
      )}

      {showCreate && projectId ? (
        <CreatePreviewDialog busy={createMutation.isPending} onClose={() => setShowCreate(false)} onCreate={(input) => createMutation.mutate(input)} />
      ) : null}
      {renderConfirm()}
    </div>
  );
}

export default PreviewDeploymentsView;
