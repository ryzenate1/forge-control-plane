"use client";

import { useMemo, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { Plus, Play, XCircle, Trash2, GitBranch } from "lucide-react";
import {
  cancelSourceDeployment,
  createSourceDeployment,
  deleteSourceDeployment,
  deploySourceDeployment,
  listSourceDeployments,
  type SourceDeployment,
} from "@/lib/api/source-deployments";
import { listGitProviderTokens, type GitProviderToken } from "@/lib/api/git-admin";
import { fetchServers } from "@/lib/api/servers";
import { statusLabel } from "@/lib/api/apps";
import { sourceStatusTone } from "@/lib/api/status";
import {
  AdminErrorState,
  AdminLoadingRows,
  AdminPageHeader,
  AdminPageLayout,
  AdminSection,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminToolbar,
  AdminTr,
  Btn,
  Card,
  EmptyState,
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { errorMessage, formatDate } from "@/lib/utils";

/**
 * Terminal states as the API defines them: `POST /:id/cancel` answers 409 for
 * exactly these three (`handlers_source_deployments.go:220-223`), so the Cancel
 * control is disabled with that sentence instead of firing a request that cannot
 * succeed.
 */
const TERMINAL = new Set(["completed", "failed", "canceled"]);

/** Build types the create handler accepts (`handlers_source_deployments.go:88`). */
const BUILD_TYPES = ["dockerfile", "nixpacks", "heroku", "paketo", "static"];

const EMPTY_FORM = {
  repository: "",
  branch: "main",
  buildType: "dockerfile",
  buildContext: ".",
  dockerfilePath: "Dockerfile",
  gitProviderId: "",
  serverId: "",
  autoDeploy: false,
  registry: "",
};

function repoName(repository: string): string {
  const last = repository.split("/").pop() ?? repository;
  return last.replace(/\.git$/i, "") || repository;
}

function timeOrDash(value?: string | null): string {
  return value ? formatDate(value) : "—";
}

export default function SourceDeploymentsPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [confirm, renderConfirm] = useConfirm();
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);

  const statusFilter = searchParams.get("status") ?? "";
  const search = searchParams.get("q") ?? "";

  const setParam = (key: string, value: string) => {
    const params = new URLSearchParams(searchParams.toString());
    if (value) params.set(key, value);
    else params.delete(key);
    const query = params.toString();
    router.replace(query ? `/admin/source-deployments?${query}` : "/admin/source-deployments", { scroll: false });
  };

  const hasFilters = Boolean(statusFilter || search);

  const deploymentsQuery = useQuery({
    queryKey: ["sourceDeployments"],
    queryFn: () => listSourceDeployments(),
    // This list used to be static: a build could finish while the operator was
    // looking at it and nothing moved. Poll only while something is in flight.
    refetchInterval: (query) => {
      const rows = query.state.data;
      if (!Array.isArray(rows)) return false;
      return rows.some((d) => !TERMINAL.has(d.status)) ? 10_000 : false;
    },
  });

  const providersQuery = useQuery({
    queryKey: ["git-providers"],
    queryFn: () => listGitProviderTokens(),
    enabled: showCreate,
  });

  const serversQuery = useQuery({
    queryKey: ["servers"],
    queryFn: () => fetchServers(),
    enabled: showCreate,
  });

  const deployments = useMemo(
    () => (Array.isArray(deploymentsQuery.data) ? deploymentsQuery.data : []),
    [deploymentsQuery.data],
  );
  const providers = useMemo(
    () => (Array.isArray(providersQuery.data) ? providersQuery.data : []),
    [providersQuery.data],
  );
  const servers = useMemo(
    () => (Array.isArray(serversQuery.data) ? serversQuery.data : []),
    [serversQuery.data],
  );

  const statusOptions = useMemo(
    () => Array.from(new Set(deployments.map((d) => d.status))).sort(),
    [deployments],
  );

  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return deployments.filter((d) => {
      if (statusFilter && d.status !== statusFilter) return false;
      if (!needle) return true;
      return `${d.repository} ${d.branch} ${d.commitMessage ?? ""} ${d.imageTag ?? ""}`
        .toLowerCase()
        .includes(needle);
    });
  }, [deployments, statusFilter, search]);

  const createMutation = useMutation({
    mutationFn: () =>
      createSourceDeployment({
        serverId: form.serverId,
        repository: form.repository,
        branch: form.branch,
        buildType: form.buildType,
        buildContext: form.buildContext,
        dockerfilePath: form.buildType === "dockerfile" ? form.dockerfilePath : undefined,
        gitProviderId: form.gitProviderId || undefined,
        autoDeploy: form.autoDeploy,
        registry: form.registry || undefined,
      }),
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployments"] });
      setShowCreate(false);
      setForm(EMPTY_FORM);
      toast({
        tone: "success",
        title: "Source deployment created",
        message: `${repoName(created.repository)} · ${created.branch} is ready to build.`,
      });
    },
    onError: (err) => toast({ tone: "error", title: "Not created", message: errorMessage(err) }),
  });

  const deployMutation = useMutation({
    mutationFn: (id: string) => deploySourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployments"] });
      toast({ tone: "success", title: "Build queued", message: "The deployment was queued for a rebuild." });
    },
    onError: (err) => toast({ tone: "error", title: "Build not queued", message: errorMessage(err) }),
  });

  const cancelMutation = useMutation({
    mutationFn: (id: string) => cancelSourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployments"] });
      toast({ tone: "success", title: "Deployment canceled" });
    },
    onError: (err) => toast({ tone: "error", title: "Cancel failed", message: errorMessage(err) }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteSourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployments"] });
      toast({ tone: "success", title: "Source deployment removed", message: "Its build history is gone too." });
    },
    onError: (err) => toast({ tone: "error", title: "Delete failed", message: errorMessage(err) }),
  });

  const createReason = !form.repository.trim()
    ? "Give the repository URL to continue."
    : !form.serverId
      ? "Choose the server this source deploys to."
      : undefined;

  return (
    <AdminPageLayout>
      <AdminPageHeader
        action={
          <Btn onClick={() => setShowCreate((v) => !v)} tone={showCreate ? "ghost" : "primary"}>
            <Plus aria-hidden="true" size={14} /> {showCreate ? "Close" : "New source deployment"}
          </Btn>
        }
        status={<FreshnessBadge state={sourceState(deploymentsQuery, 10_000)} />}
      />

      {showCreate ? (
        <AdminSection
          description="The record is created in the state the API stores it in; nothing is built until you deploy it."
          title="Create source deployment"
        >
          <Card className="p-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <Input
                label="Repository URL"
                onChange={(v) => setForm({ ...form, repository: v })}
                placeholder="https://github.com/user/repo.git"
                value={form.repository}
              />
              <Input
                label="Branch"
                onChange={(v) => setForm({ ...form, branch: v })}
                placeholder="main"
                value={form.branch}
              />
              <AdminSelect
                label="Build type"
                onChange={(v) => setForm({ ...form, buildType: v })}
                options={BUILD_TYPES.map((t) => ({ value: t, label: statusLabel(t) }))}
                value={form.buildType}
              />
              <Input
                label="Build context"
                onChange={(v) => setForm({ ...form, buildContext: v })}
                placeholder="."
                value={form.buildContext}
              />
              {form.buildType === "dockerfile" ? (
                <Input
                  label="Dockerfile path"
                  onChange={(v) => setForm({ ...form, dockerfilePath: v })}
                  placeholder="Dockerfile"
                  value={form.dockerfilePath}
                />
              ) : null}
              <Input
                label="Registry (optional)"
                onChange={(v) => setForm({ ...form, registry: v })}
                placeholder="ghcr.io"
                value={form.registry}
              />

              <div>
                {providersQuery.isPending ? (
                  <AdminLoadingRows cols={1} label="Loading git providers…" rows={1} />
                ) : providersQuery.isError ? (
                  <AdminErrorState
                    message={`Git providers could not be listed: ${errorMessage(providersQuery.error)}`}
                    retry={() => void providersQuery.refetch()}
                  />
                ) : (
                  <AdminSelect
                    label="Git provider (optional)"
                    onChange={(v) => setForm({ ...form, gitProviderId: v })}
                    options={[
                      { value: "", label: "None — clone anonymously or by URL" },
                      ...providers.map((p: GitProviderToken) => ({
                        value: p.id,
                        label: `${p.username || p.providerName} (${p.provider})`,
                      })),
                    ]}
                    value={form.gitProviderId}
                  />
                )}
              </div>

              <div>
                {serversQuery.isPending ? (
                  <AdminLoadingRows cols={1} label="Loading servers…" rows={1} />
                ) : serversQuery.isError ? (
                  <AdminErrorState
                    message={`Servers could not be listed: ${errorMessage(serversQuery.error)}`}
                    retry={() => void serversQuery.refetch()}
                  />
                ) : servers.length === 0 ? (
                  <p className="ui-alert ui-alert-warning">
                    No server is registered, so there is nowhere to deploy this source. Register one first.
                  </p>
                ) : (
                  <AdminSelect
                    label="Target server"
                    onChange={(v) => setForm({ ...form, serverId: v })}
                    options={[
                      { value: "", label: "Choose a server" },
                      ...servers.map((s) => ({
                        value: s.id,
                        label: s.name ? `${s.name} · ${s.id.slice(0, 8)}` : s.id,
                      })),
                    ]}
                    value={form.serverId}
                  />
                )}
              </div>
            </div>

            <label className="mt-4 flex cursor-pointer items-center gap-2 text-sm text-text">
              <input
                checked={form.autoDeploy}
                className="h-4 w-4 rounded border-line bg-overlay-subtle"
                onChange={(e) => setForm({ ...form, autoDeploy: e.target.checked })}
                type="checkbox"
              />
              Auto-deploy on push
            </label>

            <div className="mt-4 flex flex-wrap items-center justify-end gap-2">
              {createReason ? <span className="mr-auto text-meta text-text-muted">{createReason}</span> : null}
              <Btn onClick={() => setShowCreate(false)} tone="ghost">
                Cancel
              </Btn>
              <Btn
                disabled={Boolean(createReason) || createMutation.isPending}
                loading={createMutation.isPending}
                onClick={() => createMutation.mutate()}
                tone="primary"
              >
                Create
              </Btn>
            </div>
          </Card>
        </AdminSection>
      ) : null}

      <AdminToolbar>
        <Input
          label="Search deployments"
          onChange={(v) => setParam("q", v)}
          placeholder="Repository, branch, commit or image"
          value={search}
        />
        <AdminSelect
          label="Status"
          onChange={(v) => setParam("status", v)}
          options={statusOptions.map((s) => ({ value: s, label: statusLabel(s) }))}
          placeholder="All statuses"
          value={statusFilter}
        />
        {hasFilters ? (
          <Btn onClick={() => router.replace("/admin/source-deployments", { scroll: false })} size="sm" tone="ghost">
            Clear filters
          </Btn>
        ) : null}
      </AdminToolbar>

      <AdminSection
        description={
          deploymentsQuery.data !== undefined
            ? hasFilters
              ? `${filtered.length.toLocaleString()} of ${deployments.length.toLocaleString()} source deployments match these filters.`
              : `${deployments.length.toLocaleString()} source deployment${deployments.length === 1 ? "" : "s"} recorded on this control plane.`
            : "Count not loaded."
        }
        title="Source deployments"
      >
        <Card>
          {deploymentsQuery.isPending ? (
            <AdminLoadingRows cols={5} rows={4} label="Loading source deployments…" />
          ) : deploymentsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`Source deployments could not be loaded: ${errorMessage(deploymentsQuery.error)}`}
                retry={() => void deploymentsQuery.refetch()}
              />
            </div>
          ) : deployments.length === 0 ? (
            <EmptyState
              icon={GitBranch}
              message="No source deployment has been created yet. Each one builds an image from a Git repository and deploys it to a server."
              title="No source deployments"
            />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={GitBranch}
              message="Every source deployment was filtered out. Clear the filters above to see all of them."
              title="No deployment matches these filters"
            />
          ) : (
            <AdminTable label="Source deployments">
              <AdminTHead>
                <AdminTh>Source</AdminTh>
                <AdminTh>Build</AdminTh>
                <AdminTh>Commit</AdminTh>
                <AdminTh>Status</AdminTh>
                <AdminTh>Image</AdminTh>
                <AdminTh>Updated</AdminTh>
                <AdminTh>Actions</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {filtered.map((d) => {
                  const busy =
                    (deployMutation.isPending && deployMutation.variables === d.id) ||
                    (cancelMutation.isPending && cancelMutation.variables === d.id) ||
                    (deleteMutation.isPending && deleteMutation.variables === d.id);
                  const cancelBlocked = TERMINAL.has(d.status)
                    ? `Already ${statusLabel(d.status)} — the control plane refuses to cancel a deployment in a terminal state.`
                    : undefined;
                  return (
                    <AdminTr key={d.id}>
                      <AdminTd className="max-w-64 text-sm">
                        <Link
                          className="flex items-start gap-2 underline decoration-line-strong underline-offset-2 hover:text-text"
                          href={`/admin/source-deployments/${encodeURIComponent(d.id)}`}
                        >
                          <GitBranch aria-hidden="true" className="mt-0.5 shrink-0 text-text-muted" size={13} />
                          <span className="min-w-0">
                            <span className="block break-words font-medium">{repoName(d.repository)}</span>
                            <span className="mt-0.5 block font-mono text-meta text-text-muted">
                              {d.branch || "branch not reported"}
                            </span>
                          </span>
                        </Link>
                      </AdminTd>
                      <AdminTd className="text-meta">
                        {statusLabel(d.buildType)}
                        <span className="mt-0.5 block text-text-muted">
                          {d.autoDeploy ? "Auto-deploy on" : "Auto-deploy off"}
                          {d.buildContext ? ` · context ${d.buildContext}` : ""}
                        </span>
                      </AdminTd>
                      <AdminTd className="max-w-56 text-meta">
                        {d.commitHash ? (
                          <span className="font-mono">{d.commitHash.slice(0, 8)}</span>
                        ) : (
                          <span className="text-text-muted">No commit recorded</span>
                        )}
                        {d.commitMessage ? (
                          <span className="mt-0.5 block break-words text-text-muted">{d.commitMessage}</span>
                        ) : null}
                      </AdminTd>
                      {/* One tone source. This page carried two private colour maps and a
                          `bg-current/10` chip that resolved to no background at all. */}
                      <AdminTd>
                        <Pill tone={sourceStatusTone(d.status)}>{statusLabel(d.status) || "unknown"}</Pill>
                      </AdminTd>
                      <AdminTd className="max-w-48 break-all font-mono text-meta">
                        {d.imageTag || <span className="text-text-muted">Not built yet</span>}
                      </AdminTd>
                      <AdminTd className="whitespace-nowrap text-meta">{timeOrDash(d.updatedAt)}</AdminTd>
                      <AdminTd>
                        <div className="flex flex-wrap items-center gap-1">
                          <Btn
                            ariaLabel={`Rebuild and deploy ${repoName(d.repository)} on branch ${d.branch}`}
                            disabled={busy}
                            loading={busy && deployMutation.variables === d.id}
                            onClick={() => deployMutation.mutate(d.id)}
                            size="sm"
                            tone="ghost"
                          >
                            <Play aria-hidden="true" size={12} /> Deploy
                          </Btn>
                          <Btn
                            ariaLabel={`Cancel ${repoName(d.repository)} on branch ${d.branch}`}
                            disabled={Boolean(cancelBlocked) || busy}
                            loading={busy && cancelMutation.variables === d.id}
                            onClick={() => cancelMutation.mutate(d.id)}
                            size="sm"
                            tone="ghost"
                          >
                            <XCircle aria-hidden="true" size={12} /> Cancel
                          </Btn>
                          <Btn
                            ariaLabel={`Delete ${repoName(d.repository)} on branch ${d.branch}`}
                            disabled={busy}
                            loading={busy && deleteMutation.variables === d.id}
                            onClick={() => void askDelete(d)}
                            size="sm"
                            tone="danger"
                          >
                            <Trash2 aria-hidden="true" size={12} /> Delete
                          </Btn>
                        </div>
                        {cancelBlocked ? (
                          <p className="mt-1 text-meta text-text-muted">{cancelBlocked}</p>
                        ) : null}
                      </AdminTd>
                    </AdminTr>
                  );
                })}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      </AdminSection>

      {renderConfirm()}
    </AdminPageLayout>
  );

  async function askDelete(deployment: SourceDeployment) {
    const ok = await confirm({
      confirmLabel: "Delete source deployment",
      danger: true,
      description: `${repoName(deployment.repository)} (${deployment.branch}) is removed together with its build history. The deployed server and its running container are not undone by this.`,
      title: "Delete this source deployment?",
    });
    if (ok) deleteMutation.mutate(deployment.id);
  }
}
