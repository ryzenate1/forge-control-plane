"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Globe, History, KeyRound, Plus } from "lucide-react";
import {
  AdminErrorState,
  AdminIconButton,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSelect,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import { Dialog } from "@/components/ui/primitives";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  createEnvironment,
  deleteEnvVar,
  fetchEnvironments,
  fetchEnvVarRevisions,
  fetchOrganizations,
  fetchProjects,
  type EnvVarRevision,
} from "@/lib/api/tenancy";
import { createEnvVar, fetchEnvVars, type EnvVarResponse } from "@/lib/api/env-vars";
import { useToast } from "@/components/ui/toast";
import { environmentColorChoices } from "@/lib/design-tokens";
import { errorMessage, formatDate } from "@/lib/utils";

/**
 * Secrets are encrypted at rest and `store.EnvironmentVariable.ValueEncrypted`
 * is `json:"-"`, so the list endpoint never returns a value. The old page drew
 * an Eye/EyeOff toggle that flipped local state and revealed nothing — an
 * enabled control lying about what it did. There is now no reveal affordance:
 * the row states plainly that the value is not returned. A reveal would need a
 * real server endpoint (see the impl report).
 */
function ValueState({ sensitive }: { sensitive: boolean }) {
  return (
    <span className="font-mono text-eyebrow uppercase tracking-wider text-text-muted">
      {sensitive ? "••••••••" : "—"} · value not returned
    </span>
  );
}

export default function AdminEnvironmentsPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();

  const [selectedOrg, setSelectedOrg] = useState("");
  const [selectedProject, setSelectedProject] = useState("");
  const [envName, setEnvName] = useState("");
  const [envColor, setEnvColor] = useState<string>(environmentColorChoices[0]);
  const [envProtected, setEnvProtected] = useState(false);

  const [selectedEnv, setSelectedEnv] = useState("");
  const [varKey, setVarKey] = useState("");
  const [varValue, setVarValue] = useState("");
  const [historyVar, setHistoryVar] = useState<EnvVarResponse | null>(null);
  const [revisions, setRevisions] = useState<EnvVarRevision[]>([]);
  const [revisionsLoading, setRevisionsLoading] = useState(false);
  const [revisionsError, setRevisionsError] = useState<string | null>(null);

  const colors = environmentColorChoices;

  const orgsQuery = useQuery({ queryKey: ["organizations"], queryFn: fetchOrganizations });

  const projectsQuery = useQuery({
    queryKey: ["projects", selectedOrg],
    queryFn: () => fetchProjects(selectedOrg),
    enabled: Boolean(selectedOrg),
  });

  const environmentsQuery = useQuery({
    queryKey: ["environments", selectedProject],
    queryFn: () => fetchEnvironments(selectedProject),
    enabled: Boolean(selectedProject),
  });

  const envVarsQuery = useQuery({
    queryKey: ["env-vars", selectedEnv],
    queryFn: () => fetchEnvVars(selectedEnv),
    enabled: Boolean(selectedEnv),
  });

  const createEnvMutation = useMutation({
    mutationFn: () => createEnvironment(selectedProject, envName.trim(), envColor, envProtected),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["environments", selectedProject] });
      setEnvName("");
      toast({ tone: "success", title: "Environment created" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create environment", message: errorMessage(err) }),
  });

  const addVarMutation = useMutation({
    mutationFn: () => createEnvVar(selectedEnv, varKey.trim(), varValue, false),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["env-vars", selectedEnv] });
      setVarKey("");
      setVarValue("");
    },
    onError: (err) => toast({ tone: "error", title: "Failed to add variable", message: errorMessage(err) }),
  });

  const deleteVarMutation = useMutation({
    mutationFn: (varId: string) => deleteEnvVar(varId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["env-vars", selectedEnv] });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to delete variable", message: errorMessage(err) }),
  });

  const orgs = useMemo(() => (Array.isArray(orgsQuery.data) ? orgsQuery.data : []), [orgsQuery.data]);
  const projects = useMemo(() => (Array.isArray(projectsQuery.data) ? projectsQuery.data : []), [projectsQuery.data]);
  const environments = useMemo(() => (Array.isArray(environmentsQuery.data) ? environmentsQuery.data : []), [environmentsQuery.data]);
  const envVars = useMemo(() => (Array.isArray(envVarsQuery.data) ? envVarsQuery.data : []), [envVarsQuery.data]);

  const handleCreateEnv = (event: React.FormEvent) => {
    event.preventDefault();
    if (!envName.trim() || !selectedProject) return;
    createEnvMutation.mutate();
  };

  const handleAddVar = (event: React.FormEvent) => {
    event.preventDefault();
    if (!varKey.trim() || !selectedEnv) return;
    addVarMutation.mutate();
  };

  // Deleting an environment variable changes what a deployment resolves. It goes
  // through the shared confirm dialog like every other Access-group deletion.
  const handleDeleteVar = async (v: EnvVarResponse) => {
    const ok = await confirm({
      title: `Delete ${v.key}?`,
      description: `Version ${v.version} of this ${v.scope} variable will be removed. Workloads that resolve it will fall back to the value inherited from their parent scope, or to nothing. This cannot be undone.`,
      danger: true,
      confirmLabel: "Delete variable",
    });
    if (ok) deleteVarMutation.mutate(v.id);
  };

  // A failed revision read must never look like "no history": the error is kept
  // and rendered as an error with a retry.
  const openRevisions = async (v: EnvVarResponse) => {
    setHistoryVar(v);
    setRevisions([]);
    setRevisionsError(null);
    setRevisionsLoading(true);
    try {
      const data = await fetchEnvVarRevisions(v.id);
      setRevisions(Array.isArray(data) ? data : []);
    } catch (err) {
      setRevisionsError(errorMessage(err, "Revision history could not be loaded."));
    } finally {
      setRevisionsLoading(false);
    }
  };

  const closeHistory = () => {
    setHistoryVar(null);
    setRevisions([]);
    setRevisionsError(null);
  };

  return (
    <AdminPageLayout>
      <AdminPageHeader />

      <div className="grid gap-3 sm:grid-cols-2 lg:max-w-2xl">
        <AdminSelect
          label="Organization"
          value={selectedOrg}
          onChange={(v) => { setSelectedOrg(v); setSelectedProject(""); setSelectedEnv(""); }}
          placeholder={orgsQuery.isLoading ? "Loading organizations…" : "Select organization"}
          options={orgs.map((org) => ({ value: org.id, label: org.name }))}
        />
        <AdminSelect
          label="Project"
          value={selectedProject}
          onChange={(v) => { setSelectedProject(v); setSelectedEnv(""); }}
          placeholder={selectedOrg ? (projectsQuery.isLoading ? "Loading projects…" : "Select project") : "Select an organization first"}
          options={projects.map((p) => ({ value: p.id, label: p.name }))}
          disabled={!selectedOrg}
        />
      </div>

      {orgsQuery.isError ? (
        <AdminErrorState message={errorMessage(orgsQuery.error, "Organizations could not be loaded.")} retry={() => void orgsQuery.refetch()} />
      ) : null}
      {selectedOrg && projectsQuery.isError ? (
        <AdminErrorState message={errorMessage(projectsQuery.error, "Projects could not be loaded.")} retry={() => void projectsQuery.refetch()} />
      ) : null}

      <Card>
        <CardHeader title="Create environment" icon={Plus} />
        <form className="space-y-4 p-4" onSubmit={handleCreateEnv}>
          <div className="grid gap-3 sm:grid-cols-2">
            <Input label="Name" value={envName} onChange={setEnvName} placeholder="e.g. production" required disabled={!selectedProject} />
            <div>
              <span className="ui-label mb-1.5 block">Colour</span>
              <div className="flex gap-1.5" role="group" aria-label="Environment colour">
                {colors.map((c) => (
                  <button
                    key={c}
                    type="button"
                    aria-label={`Use colour ${c}`}
                    aria-pressed={envColor === c}
                    onClick={() => setEnvColor(c)}
                    className={envColor === c ? "h-6 w-6 rounded-full border-2 border-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" : "h-6 w-6 rounded-full border-2 border-transparent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"}
                    style={{ backgroundColor: c }}
                  />
                ))}
              </div>
            </div>
          </div>
          <label className="flex items-center gap-2 text-xs text-text-subtle">
            <input type="checkbox" checked={envProtected} onChange={(e) => setEnvProtected(e.target.checked)} className="accent-[var(--brand)]" />
            Protected
          </label>
          {/* Disabled with the reason visible, rather than hidden until a project
              happens to be chosen. */}
          {!selectedProject ? (
            <p className="ui-hint">Select a project above — an environment belongs to exactly one project.</p>
          ) : null}
          <Btn type="submit" loading={createEnvMutation.isPending} disabled={!selectedProject || !envName.trim()}>
            <Plus size={14} /> Create
          </Btn>
          {createEnvMutation.isError ? (
            <AdminErrorState message={errorMessage(createEnvMutation.error, "Environment could not be created.")} />
          ) : null}
        </form>
      </Card>

      <div className="grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader title="Environments" icon={Globe} />
          {!selectedProject ? (
            <EmptyState icon={Globe} title="Select a project" message="Environments are listed per project. Choose an organization and project above." />
          ) : environmentsQuery.isLoading ? (
            <div className="p-4"><AdminLoadingState label="Loading environments…" /></div>
          ) : environmentsQuery.isError ? (
            <div className="p-4"><AdminErrorState message={errorMessage(environmentsQuery.error, "Environments could not be loaded.")} retry={() => void environmentsQuery.refetch()} /></div>
          ) : environments.length === 0 ? (
            <EmptyState icon={Globe} title="No environments" message="This project has no environments yet. Create one above." />
          ) : (
            <div className="divide-y divide-line">
              {environments.map((env) => (
                <button
                  key={env.id}
                  type="button"
                  aria-pressed={selectedEnv === env.id}
                  onClick={() => setSelectedEnv(env.id)}
                  className={`flex w-full items-center justify-between gap-3 px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)] ${selectedEnv === env.id ? "bg-overlay-subtle" : "hover:bg-overlay-subtle"}`}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <span aria-hidden="true" className="h-3 w-3 shrink-0 rounded-full" style={{ backgroundColor: env.color }} />
                    <span className="truncate text-sm font-medium text-text">{env.name}</span>
                    {env.protected ? <Pill tone="warn">Protected</Pill> : null}
                  </span>
                  {selectedEnv === env.id ? <span className="t-meta shrink-0 text-brand">selected</span> : null}
                </button>
              ))}
            </div>
          )}
        </Card>

        <Card>
          <CardHeader
            title="Environment variables"
            icon={KeyRound}
            action={selectedEnv && envVarsQuery.isSuccess ? <span className="t-meta">{envVars.length} recorded</span> : null}
          />
          {!selectedEnv ? (
            <EmptyState icon={KeyRound} title="Select an environment" message="Variables are listed per environment. Select one on the left." />
          ) : (
            <>
              <form className="flex flex-wrap items-end gap-2 border-b border-line p-3" onSubmit={handleAddVar}>
                <div className="min-w-40 flex-1">
                  <Input label="Key" value={varKey} onChange={setVarKey} placeholder="EXAMPLE_KEY" required mono />
                </div>
                <div className="min-w-40 flex-1">
                  <Input label="Value" value={varValue} onChange={setVarValue} placeholder="value" />
                </div>
                <Btn type="submit" loading={addVarMutation.isPending} disabled={!varKey.trim()} ariaLabel="Add variable">
                  <Plus size={14} />
                </Btn>
              </form>
              {addVarMutation.isError ? (
                <div className="p-3"><AdminErrorState message={errorMessage(addVarMutation.error, "Variable could not be added.")} /></div>
              ) : null}
              {envVarsQuery.isLoading ? (
                <div className="p-4"><AdminLoadingState label="Loading variables…" /></div>
              ) : envVarsQuery.isError ? (
                <div className="p-4"><AdminErrorState message={errorMessage(envVarsQuery.error, "Environment variables could not be loaded.")} retry={() => void envVarsQuery.refetch()} /></div>
              ) : envVars.length === 0 ? (
                <EmptyState icon={KeyRound} title="No environment variables" message="This environment has no variables of its own. Values inherited from the project are resolved at deploy time and are not listed here." />
              ) : (
                <div className="divide-y divide-line">
                  {envVars.map((v) => (
                    <div key={v.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-2.5">
                      <span className="flex min-w-0 flex-wrap items-center gap-2">
                        <span className="truncate font-mono text-xs text-text">{v.key}</span>
                        <span className="t-meta">v{v.version}</span>
                        {v.isSensitive ? <Pill tone="warn">Sensitive</Pill> : null}
                      </span>
                      <span className="flex shrink-0 items-center gap-2">
                        <ValueState sensitive={Boolean(v.isSensitive)} />
                        <AdminIconButton label={`Revision history for ${v.key}`} onClick={() => void openRevisions(v)}>
                          <History size={14} />
                        </AdminIconButton>
                        <Btn
                          size="sm"
                          tone="ghost"
                          ariaLabel={`Delete ${v.key}`}
                          disabled={deleteVarMutation.isPending}
                          onClick={() => void handleDeleteVar(v)}
                        >
                          Delete
                        </Btn>
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </Card>
      </div>

      {historyVar ? (
        <Dialog
          open
          className="max-w-lg"
          title={`Revision history — ${historyVar.key}`}
          description={`Version history for ${historyVar.key} (current v${historyVar.version}). Values are encrypted at rest and never returned by the API.`}
          closeAction={closeHistory}
        >
          {revisionsLoading ? (
            <AdminLoadingState label="Loading revisions…" />
          ) : revisionsError ? (
            <AdminErrorState message={revisionsError} retry={() => void openRevisions(historyVar)} />
          ) : revisions.length === 0 ? (
            <EmptyState icon={History} title="No revisions recorded" message="This variable has no stored revision history. Older values may predate revision recording." />
          ) : (
            <div className="divide-y divide-line rounded-lg border border-line">
              {revisions.map((r) => (
                <div key={r.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-2.5">
                  <span className="flex items-center gap-2">
                    <span className="ui-badge ui-badge-brand font-mono font-semibold">v{r.version}</span>
                    {r.createdBy ? <span className="t-meta">by {r.createdBy}</span> : null}
                  </span>
                  <span className="t-meta">{formatDate(r.createdAt, "Unknown time")}</span>
                </div>
              ))}
            </div>
          )}
        </Dialog>
      ) : null}

      {renderConfirm()}
    </AdminPageLayout>
  );
}
