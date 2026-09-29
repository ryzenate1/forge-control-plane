"use client";

import { useMemo, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { fetchJSON } from "@/lib/api";
import {
  deleteComposeStack,
  importCompose,
  listComposeProjects,
  restartComposeStack,
  startComposeStack,
  stopComposeStack,
  type ComposeStack,
} from "@/lib/api/compose";
import { composeStatusTone } from "@/lib/api/status";
import { sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSection,
  AdminSelect,
  AdminToolbar,
  Btn,
  Card,
  EmptyState,
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { DegradedBanner } from "@/components/shared/states-connectivity";
import { Pagination } from "@/components/ui/primitives";
import { Plus, Trash2 } from "lucide-react";
import { errorMessage, formatDate } from "@/lib/utils";

/**
 * Compose stacks.
 *
 * Status colour has exactly one source here: `composeStatusTone`. The page used
 * to keep a nine-entry `statusConfig` of `text-emerald-400`/`bg-red-500/10`
 * class strings *plus* a `stackTone()` that cast the shared tone into a union it
 * could not produce, *plus* a `healthTone()`. The fallback
 * `statusConfig[stack.status] || statusConfig.failed` drew any state the backend
 * adds next as a red failure, next to a pill that correctly said `unknown`.
 *
 * The "Health: Healthy" chip is gone. It was derived from `stack.status` — a
 * `running` stack with three crashed services read Healthy — and nothing on this
 * route reports a health check. That is a claim, not a reading.
 */

const POLL_MS = 15_000;
const PAGE_SIZE = 10;

function humanToken(value: string): string {
  return value.replace(/_/g, " ");
}

export default function ComposeStacksPage() {
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const { toast } = useToast();
  const router = useRouter();
  const [deleteVolumes, setDeleteVolumes] = useState(false);
  const [showImport, setShowImport] = useState(false);
  const [importName, setImportName] = useState("");
  const [importContent, setImportContent] = useState("");

  const stacksQuery = useQuery<ComposeStack[]>({
    queryKey: ["compose-stacks"],
    queryFn: () => fetchJSON<ComposeStack[]>("/compose"),
    refetchInterval: POLL_MS,
  });

  const projectsQ = useQuery({
    queryKey: ["compose-projects"],
    queryFn: () => listComposeProjects(),
  });

  const safeStacks = useMemo(() => (Array.isArray(stacksQuery.data) ? stacksQuery.data : []), [stacksQuery.data]);
  const [search, setSearch] = useState("");
  const [nodeFilter, setNodeFilter] = useState("");
  const [typeFilter, setTypeFilter] = useState("");

  const nodeOptions = useMemo(
    () => Array.from(new Set(safeStacks.map((s) => s.nodeId).filter(Boolean))).sort(),
    [safeStacks],
  );
  const typeOptions = useMemo(
    () => Array.from(new Set(safeStacks.map((s) => s.composeType || s.sourceType).filter(Boolean))).sort(),
    [safeStacks],
  );

  const filteredStacks = useMemo(() => {
    return safeStacks.filter((s) => {
      if (search && !s.name.toLowerCase().includes(search.toLowerCase())) return false;
      if (nodeFilter && s.nodeId !== nodeFilter) return false;
      if (typeFilter && s.composeType !== typeFilter && s.sourceType !== typeFilter) return false;
      return true;
    });
  }, [safeStacks, search, nodeFilter, typeFilter]);

  const [page, setPage] = useState(1);
  const totalPages = Math.max(1, Math.ceil(filteredStacks.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const paginatedStacks = useMemo(() => {
    const start = (safePage - 1) * PAGE_SIZE;
    return filteredStacks.slice(start, start + PAGE_SIZE);
  }, [filteredStacks, safePage]);

  const hasDegraded = safeStacks.some((s) => s.status === "degraded" || s.status === "failed");
  const isDegraded = hasDegraded && !stacksQuery.isPending && !stacksQuery.isError;

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteComposeStack(id, { volumes: deleteVolumes }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({
        tone: "success",
        title: "Stack deleted",
        message: deleteVolumes ? "The stack and its volumes were removed." : "The stack was removed.",
      });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: errorMessage(e) }),
  });

  const importMut = useMutation({
    mutationFn: () => importCompose({ name: importName, content: importContent }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-projects"] });
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      setShowImport(false);
      setImportName("");
      setImportContent("");
      toast({ tone: "success", title: "Compose imported", message: "The stack was created from the pasted document." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Import failed", message: errorMessage(e) }),
  });

  // Per-row busy state: one shared flag used to disable every stack's controls
  // while any single one was starting.
  const actionMutation = useMutation({
    mutationFn: ({ id, action }: { id: string; action: "start" | "stop" | "restart" }) =>
      action === "start" ? startComposeStack(id) : action === "stop" ? stopComposeStack(id) : restartComposeStack(id),
    onSuccess: (_data, { action }) => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({
        tone: "success",
        title: action === "start" ? "Start requested" : action === "stop" ? "Stop requested" : "Restart requested",
        message: "The stack is changing state; this list refreshes as the agent reports it.",
      });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Stack operation failed", message: errorMessage(e) }),
  });

  async function askDelete(id: string, name: string) {
    const ok = await confirm({
      title: `Delete “${name}”?`,
      description: deleteVolumes
        ? "The stack and its services will be removed, including its volumes. This cannot be undone."
        : "The stack and its services will be removed. Its volumes are kept. This cannot be undone.",
      danger: true,
      confirmLabel: "Delete",
    });
    if (ok) deleteMutation.mutate(id);
  }

  const filtersActive = Boolean(search || nodeFilter || typeFilter);

  return (
    <AdminPageLayout>
      <AdminPageHeader
        status={<FreshnessBadge state={sourceState(stacksQuery, POLL_MS)} />}
        action={
          <div className="flex items-center gap-2">
            <Btn tone="ghost" onClick={() => setShowImport((v) => !v)}>
              Import
            </Btn>
            <Btn tone="primary" onClick={() => router.push("/admin/compose/new")}>
              <Plus aria-hidden="true" className="h-4 w-4" /> New stack
            </Btn>
          </div>
        }
      />
      <OfflineBanner onRetry={() => void stacksQuery.refetch()} />

      {showImport ? (
        <AdminSection title="Import a compose document">
          <Card className="p-4">
            <div className="space-y-3">
              <Input
                label="Stack name"
                value={importName}
                onChange={setImportName}
                placeholder="my-stack"
              />
              <label className="block">
                <span className="ui-label mb-1.5">Compose YAML</span>
                <textarea
                  className="ui-input min-h-0 w-full font-mono"
                  onChange={(e) => setImportContent(e.target.value)}
                  placeholder={"services:\n  web:\n    image: nginx:alpine"}
                  rows={8}
                  value={importContent}
                />
              </label>
              <p className="ui-hint block">
                Import creates the stack from the document as pasted. To validate it against the server
                before anything is created, use New stack, which runs validation and shows a service
                summary first.
              </p>
              <div className="flex justify-end gap-2">
                <Btn tone="ghost" size="sm" onClick={() => setShowImport(false)}>Cancel</Btn>
                <Btn
                  size="sm"
                  tone="primary"
                  onClick={() => importMut.mutate()}
                  disabled={importMut.isPending || !importName.trim() || !importContent.trim()}
                >
                  {importMut.isPending ? "Importing…" : "Import"}
                </Btn>
              </div>
              {importMut.isError ? (
                <AdminErrorState message={`Import failed: ${errorMessage(importMut.error)}`} />
              ) : null}
            </div>
          </Card>
        </AdminSection>
      ) : null}

      <AdminSection
        title="Stacks"
        description="Multi-service workloads as the agent last reported them."
        action={
          stacksQuery.data ? (
            <Pill tone="neutral">{`${safeStacks.length.toLocaleString()} stacks`}</Pill>
          ) : (
            <Pill tone="unknown">Count not loaded</Pill>
          )
        }
      >
        <AdminToolbar>
          <Input label="Search" placeholder="Stack name" value={search} onChange={setSearch} />
          <AdminSelect
            label="Node"
            value={nodeFilter}
            onChange={setNodeFilter}
            placeholder="All nodes"
            options={nodeOptions.map((nid) => ({ value: nid, label: nid.slice(0, 8) }))}
          />
          <AdminSelect
            label="Source type"
            value={typeFilter}
            onChange={setTypeFilter}
            placeholder="All types"
            options={typeOptions.map((tp) => ({ value: tp, label: humanToken(tp) }))}
          />
          {filtersActive ? (
            <Btn tone="ghost" size="sm" onClick={() => { setSearch(""); setNodeFilter(""); setTypeFilter(""); }}>
              Clear filters
            </Btn>
          ) : null}
        </AdminToolbar>

        {/* A mutation parameter does not belong in a filter bar: this checkbox
            changed what every row's Delete button does, silently. It now sits
            beside the list with its consequence written out. */}
        <label className="flex items-center gap-2 text-meta text-text-subtle">
          <input
            checked={deleteVolumes}
            className="accent-[var(--brand)]"
            onChange={(e) => setDeleteVolumes(e.target.checked)}
            type="checkbox"
          />
          Also remove volumes when deleting a stack
        </label>

        {stacksQuery.isFetching && !stacksQuery.isPending ? (
          <p className="text-meta text-text-subtle" role="status">Refreshing…</p>
        ) : null}
        {isDegraded ? (
          <DegradedBanner
            title="Some stacks are degraded"
            message="One or more stacks report degraded or failed status."
            onRetry={() => void stacksQuery.refetch()}
          />
        ) : null}
        {actionMutation.isError ? (
          <AdminErrorState
            message={errorMessage(actionMutation.error, "Stack operation failed")}
            retry={() => actionMutation.reset()}
          />
        ) : null}

        {stacksQuery.isPending ? (
          <AdminLoadingState label="Loading Compose stacks…" />
        ) : stacksQuery.isError ? (
          <AdminErrorState
            message={`Compose stacks could not be loaded: ${errorMessage(stacksQuery.error)}`}
            retry={() => void stacksQuery.refetch()}
          />
        ) : filteredStacks.length === 0 ? (
          <Card className="p-8">
            {safeStacks.length === 0 ? (
              <>
                <EmptyState
                  title="No Compose stacks"
                  message="Create a stack to deploy a multi-service workload."
                />
                <div className="mt-4 flex justify-center">
                  <Btn onClick={() => router.push("/admin/compose/new")}>
                    <Plus aria-hidden="true" className="h-4 w-4" /> Create stack
                  </Btn>
                </div>
              </>
            ) : (
              <>
                <EmptyState title="No results" message="No stack matches the current search or filters." />
                <div className="mt-4 flex justify-center">
                  <Btn tone="ghost" onClick={() => { setSearch(""); setNodeFilter(""); setTypeFilter(""); }}>
                    Clear filters
                  </Btn>
                </div>
              </>
            )}
          </Card>
        ) : (
          <>
            <div className="grid gap-4">
              {paginatedStacks.map((stack) => {
                const rowBusy = actionMutation.isPending && actionMutation.variables?.id === stack.id;
                // Start/Stop/Restart are not offered for a state they cannot apply,
                // and the control stays visible with the reason instead of vanishing.
                const canStart = stack.status === "stopped";
                const canStop = stack.status === "running";
                const canRestart = stack.status === "running" || stack.status === "degraded";
                return (
                  <Card key={stack.id} className="p-4">
                    <div className="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <Pill tone={composeStatusTone(stack.status)}>
                            {humanToken(stack.status) || "unknown"}
                          </Pill>
                          <Link
                            className="text-sm font-medium text-text hover:underline"
                            href={`/admin/compose/${encodeURIComponent(stack.id)}`}
                          >
                            {stack.name}
                          </Link>
                          <span className="text-meta text-text-subtle">
                            {humanToken(stack.composeType || stack.sourceType) || "Source not reported"}
                          </span>
                        </div>
                        <div className="mt-2 flex flex-wrap gap-2 text-meta">
                          <Pill tone="neutral">Node: {stack.nodeId ? stack.nodeId.slice(0, 8) : "Not reported"}</Pill>
                          <Pill tone="neutral">
                            Group: {stack.environmentId ? stack.environmentId.slice(0, 8) : "Not reported"}
                          </Pill>
                          <Pill tone="neutral">Created: {stack.createdAt ? formatDate(stack.createdAt) : "Not reported"}</Pill>
                        </div>
                      </div>
                      <div className="flex shrink-0 flex-wrap items-center gap-2">
                        <Btn
                          size="sm"
                          tone="ghost"
                          ariaLabel={`Start ${stack.name}`}
                          disabled={!canStart || rowBusy}
                          onClick={() => actionMutation.mutate({ id: stack.id, action: "start" })}
                          title={canStart ? undefined : `Cannot start while the stack is “${humanToken(stack.status)}”.`}
                        >
                          Start
                        </Btn>
                        <Btn
                          size="sm"
                          tone="ghost"
                          ariaLabel={`Stop ${stack.name}`}
                          disabled={!canStop || rowBusy}
                          onClick={() => actionMutation.mutate({ id: stack.id, action: "stop" })}
                          title={canStop ? undefined : `Cannot stop while the stack is “${humanToken(stack.status)}”.`}
                        >
                          Stop
                        </Btn>
                        <Btn
                          size="sm"
                          tone="ghost"
                          ariaLabel={`Restart ${stack.name}`}
                          disabled={!canRestart || rowBusy}
                          onClick={() => actionMutation.mutate({ id: stack.id, action: "restart" })}
                          title={
                            canRestart
                              ? undefined
                              : `Cannot restart while the stack is “${humanToken(stack.status)}”.`
                          }
                        >
                          Restart
                        </Btn>
                        <Btn
                          size="sm"
                          tone="danger"
                          ariaLabel={`Delete ${stack.name}`}
                          onClick={() => void askDelete(stack.id, stack.name)}
                        >
                          <Trash2 aria-hidden="true" className="h-4 w-4" />
                        </Btn>
                      </div>
                    </div>
                    {stack.error ? (
                      // Full text, no `truncate`: the failure reason is the one thing
                      // an operator needs when a stack is not healthy.
                      <p className="ui-alert ui-alert-danger mt-3" role="status">
                        {stack.error}
                      </p>
                    ) : null}
                  </Card>
                );
              })}
            </div>
            <Pagination page={safePage} pageCount={totalPages} onPageChange={setPage} label="Compose stacks pagination" />
          </>
        )}
      </AdminSection>

      {projectsQ.isError ? (
        <AdminErrorState
          message={`Compose projects could not be loaded: ${errorMessage(projectsQ.error)}`}
          retry={() => void projectsQ.refetch()}
        />
      ) : projectsQ.data && Array.isArray(projectsQ.data) && projectsQ.data.length > 0 ? (
        <AdminSection title="Projects">
          <Card>
            <div className="divide-y divide-line">
              {projectsQ.data.map((p: { id: string; name: string; status: string; revision: number }) => (
                <div className="flex items-center justify-between gap-3 px-4 py-3" key={p.id}>
                  <div className="min-w-0">
                    <span className="text-sm text-text">{p.name}</span>
                    <span className="mt-0.5 block text-meta text-text-subtle">
                      revision {Number.isFinite(p.revision) ? p.revision : "not reported"}
                    </span>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <Pill tone={composeStatusTone(p.status)}>{humanToken(p.status) || "unknown"}</Pill>
                    <span className="font-mono text-meta text-text-subtle">{p.id.slice(0, 8)}</span>
                  </div>
                </div>
              ))}
            </div>
          </Card>
        </AdminSection>
      ) : null}

      {renderConfirm()}
    </AdminPageLayout>
  );
}
