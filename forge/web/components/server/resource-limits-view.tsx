"use client";

import { useEffect, useMemo, useState } from "react";
import { Activity, Gauge, HeartPulse, Plus, RefreshCw, Rocket, Save, ServerOff, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ApiServer } from "@/lib/api";
import {
  MAX_REPLICAS,
  type HealthCheckType,
  type ProcessConfig,
  type ProcessConfigInput,
  type ProcessHealthState,
  bytesToMegabytes,
  coresToNanoCores,
  fetchServerResourceLimits,
  formatCpuLimit,
  formatMemoryLimit,
  mbToBytes,
  nanoCoresToCores,
  previewComposeRender,
  applyComposeRender,
  reportServerHealth,
  scaleProcess,
  secondsToDuration,
  suggestedProcessTypes,
  upsertProcessConfig,
  deleteProcessConfig,
} from "@/lib/api/resource-limits";
import { Alert, Badge, Card, ConfirmDialog, EmptyState, StatusPill, Switch } from "@/components/ui/primitives";
import { CardSkeleton } from "@/components/ui/loading-skeleton";
import { useToast } from "@/components/ui/toast";
import { errorMessage } from "@/lib/utils";
import { hasServerPermission, useOptionalServerContext } from "./server-context";

const healthTone = (status: ProcessHealthState["status"] | undefined) =>
  status === "healthy" ? "success" : status === "unhealthy" ? "danger" : status === "pending" ? "warning" : "neutral";

const healthLabel = (status: ProcessHealthState["status"] | undefined) =>
  status === "healthy" ? "Healthy" : status === "unhealthy" ? "Failing" : status === "pending" ? "Awaiting probe" : "No check";

/** The editable shape of one process card. Numbers are kept as text so an
 * empty field means "no limit" instead of collapsing into 0. */
type Draft = {
  cpuCores: string;
  memoryMb: string;
  replicas: string;
  enabled: boolean;
  gating: boolean;
  checkType: HealthCheckType;
  path: string;
  port: string;
  command: string;
  interval: string;
  timeout: string;
  retries: string;
  startPeriod: string;
};

function draftFromConfig(config: ProcessConfig): Draft {
  const cores = nanoCoresToCores(config.cpuLimitNanoCores);
  const mb = bytesToMegabytes(config.memoryLimitBytes);
  return {
    cpuCores: cores === null ? "" : String(cores),
    memoryMb: mb === null ? "" : String(mb),
    replicas: String(config.replicas),
    enabled: config.enabled,
    gating: config.healthCheckGating,
    checkType: config.healthCheckType,
    path: config.healthCheckPath || "/healthz",
    port: config.healthCheckPort ? String(config.healthCheckPort) : "",
    command: config.healthCheckCommand,
    interval: String(config.healthCheckIntervalSeconds || 30),
    timeout: String(config.healthCheckTimeoutSeconds || 10),
    retries: String(config.healthCheckRetries || 3),
    startPeriod: String(config.healthCheckStartPeriodSeconds || 40),
  };
}

function emptyDraft(): Draft {
  return {
    cpuCores: "",
    memoryMb: "512",
    replicas: "1",
    enabled: true,
    gating: true,
    checkType: "http",
    path: "/healthz",
    port: "8080",
    command: "",
    interval: "30",
    timeout: "10",
    retries: "3",
    startPeriod: "40",
  };
}

/** Build the upsert payload. 0 on a limit field means "clear the limit", which
 * is how the API distinguishes removing a constraint from omitting the card. */
function inputFromDraft(draft: Draft): ProcessConfigInput {
  const cores = Number.parseFloat(draft.cpuCores);
  const mb = Number.parseInt(draft.memoryMb, 10);
  const replicas = Number.parseInt(draft.replicas, 10);
  const input: ProcessConfigInput = {
    cpuLimitNanoCores: Number.isFinite(cores) && cores > 0 ? coresToNanoCores(cores) : 0,
    memoryLimitBytes: Number.isFinite(mb) && mb > 0 ? mbToBytes(mb) : 0,
    replicas: Number.isFinite(replicas) && replicas >= 0 ? replicas : 1,
    enabled: draft.enabled,
    healthCheckGating: draft.gating,
    healthCheckType: draft.checkType,
  };
  if (draft.checkType === "http") {
    input.healthCheckPath = draft.path.startsWith("/") ? draft.path : `/${draft.path}`;
    input.healthCheckPort = Number.parseInt(draft.port, 10) || 0;
    input.healthCheckCommand = "";
  } else if (draft.checkType === "tcp") {
    input.healthCheckPort = Number.parseInt(draft.port, 10) || 0;
    input.healthCheckPath = "";
    input.healthCheckCommand = "";
  } else if (draft.checkType === "command") {
    input.healthCheckCommand = draft.command;
    input.healthCheckPath = "";
    input.healthCheckPort = 0;
  }
  if (draft.checkType !== "") {
    input.healthCheckIntervalSeconds = Number.parseInt(draft.interval, 10) || 30;
    input.healthCheckTimeoutSeconds = Number.parseInt(draft.timeout, 10) || 10;
    input.healthCheckRetries = Number.parseInt(draft.retries, 10) || 3;
    input.healthCheckStartPeriodSeconds = Number.parseInt(draft.startPeriod, 10) || 40;
  }
  return input;
}

export function ResourceLimitsView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  // An empty permission list means "any verified server member", which mirrors
  // the backend: reads are gated on server access, writes on organization
  // membership, and the manual-probe button on workload control.
  const canView = hasServerPermission(access, []);
  const canProbe = hasServerPermission(access, "control.start");
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const [newProcessType, setNewProcessType] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [renderResult, setRenderResult] = useState<{ mode: "preview" | "apply"; applied: number; skipped: number; yaml: string; skippedReasons: string[] } | null>(null);

  const limitsQuery = useQuery({
    queryKey: ["server-resource-limits", server?.id],
    queryFn: () => fetchServerResourceLimits(server?.id ?? ""),
    enabled: Boolean(server?.id) && canView,
    // Health is live data; polling is the only way this tab shows a probe that
    // landed after the page was opened.
    refetchInterval: 15_000,
  });

  const application = limitsQuery.data?.application ?? null;
  // Memoised off `data`, not `data.processes`: the fallback `[]` below would
  // otherwise be a fresh array identity on every render, and the effect that
  // seeds the drafts would then re-run forever.
  const processes = useMemo(() => limitsQuery.data?.processes ?? [], [limitsQuery.data]);
  // Memoised off `data` for the same reason as `processes` above: `health` feeds
  // two dependency arrays, so a fresh `[]` identity each render rebuilt the
  // process map and recomputed the aggregate verdict on every single render —
  // on a 15s-refetching view.
  const health = useMemo(() => limitsQuery.data?.health ?? [], [limitsQuery.data]);
  const observations = limitsQuery.data?.observations ?? [];
  const rollback = limitsQuery.data?.rollback ?? null;
  const healthByProcess = useMemo(() => new Map(health.map((state) => [state.processType, state])), [health]);

  useEffect(() => {
    setDrafts((current) => {
      const next: Record<string, Draft> = {};
      for (const config of processes) {
        // An untouched card tracks the server; a card being edited keeps the
        // operator's keystrokes until they save or discard.
        next[config.processType] = current[config.processType] ?? draftFromConfig(config);
      }
      // A type that was just created and has not come back in the refetch yet
      // stays on screen; dropping it would flash the card away mid-save.
      for (const [processType, draft] of Object.entries(current)) {
        if (!(processType in next)) next[processType] = draft;
      }
      return next;
    });
  }, [processes]);

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["server-resource-limits", server?.id] });
  };

  const saveMutation = useMutation({
    mutationFn: ({ processType, input }: { processType: string; input: ProcessConfigInput }) =>
      upsertProcessConfig(application?.id ?? "", processType, input),
    onSuccess: (config) => {
      setDrafts((current) => ({ ...current, [config.processType]: draftFromConfig(config) }));
      refresh();
      toast({ tone: "success", title: "Process configuration saved", message: `${config.processType} stored. Apply it to the compose document to enforce it on the next deploy.` });
    },
    onError: (error) => toast({ tone: "error", title: "Save failed", message: errorMessage(error, "The process configuration could not be saved.") }),
  });

  const scaleMutation = useMutation({
    mutationFn: ({ processType, replicas }: { processType: string; replicas: number }) =>
      scaleProcess(application?.id ?? "", processType, replicas),
    onSuccess: (result) => {
      setDrafts((current) => ({ ...current, [result.process.processType]: { ...current[result.process.processType], replicas: String(result.process.replicas) } }));
      refresh();
    },
    onError: (error) => toast({ tone: "error", title: "Scale failed", message: errorMessage(error, "The replica count could not be changed.") }),
  });

  const deleteMutation = useMutation({
    mutationFn: (processType: string) => deleteProcessConfig(application?.id ?? "", processType),
    onSuccess: (_result, processType) => {
      setDeleteTarget(null);
      setDrafts((current) => {
        const next = { ...current };
        delete next[processType];
        return next;
      });
      refresh();
      toast({ tone: "success", title: "Process configuration removed", message: "The stored configuration is gone; the compose document still contains the previous values until you apply it again." });
    },
    onError: (error) => toast({ tone: "error", title: "Delete failed", message: errorMessage(error, "The process configuration could not be deleted.") }),
  });

  const previewMutation = useMutation({
    mutationFn: () => previewComposeRender(application?.id ?? ""),
    onSuccess: (result) => {
      setRenderResult({
        mode: "preview",
        applied: result.applied.length,
        skipped: result.skipped.length,
        yaml: result.composeYaml,
        skippedReasons: result.skipped.map((item) => `${item.processType}: ${item.reason}`),
      });
    },
    onError: (error) => toast({ tone: "error", title: "Preview failed", message: errorMessage(error, "The compose document could not be rendered.") }),
  });

  const applyMutation = useMutation({
    mutationFn: (deploy: boolean) => applyComposeRender(application?.id ?? "", { deploy, allowPartial: false }),
    onSuccess: (result) => {
      setRenderResult({
        mode: "apply",
        applied: result.applied.length,
        skipped: result.skipped.length,
        yaml: result.composeYaml,
        skippedReasons: result.skipped.map((item) => `${item.processType}: ${item.reason}`),
      });
      refresh();
      toast({
        tone: result.deployed ? "success" : "warning",
        title: result.deployed ? "Applied and deployed" : "Applied to the compose document",
        message: result.deployed
          ? "The enriched document was released."
          : "The enriched document was stored; the next deploy will ship it.",
      });
    },
    onError: (error) => toast({ tone: "error", title: "Apply failed", message: errorMessage(error, "Nothing was written to the compose document.") }),
  });

  const probeMutation = useMutation({
    mutationFn: ({ processType, healthy }: { processType: string; healthy: boolean }) =>
      reportServerHealth(server?.id ?? "", { processType, healthy, detail: "Recorded manually from the panel" }),
    onSuccess: () => {
      refresh();
      toast({ tone: "success", title: "Probe recorded", message: "The observation was appended to the health log." });
    },
    onError: (error) => toast({ tone: "error", title: "Probe failed", message: errorMessage(error, "The health observation could not be recorded.") }),
  });

  const aggregate = useMemo(() => {
    if (health.length === 0) return "unknown" as const;
    if (health.some((state) => state.status === "unhealthy")) return "unhealthy" as const;
    if (health.some((state) => state.status === "pending")) return "pending" as const;
    if (health.every((state) => state.status === "unknown")) return "unknown" as const;
    return "healthy" as const;
  }, [health]);

  const queryError = limitsQuery.error ? errorMessage(limitsQuery.error, "Process configuration could not be loaded.") : null;
  const mutating = saveMutation.isPending || scaleMutation.isPending || deleteMutation.isPending || applyMutation.isPending || previewMutation.isPending;

  return (
    <div className="space-y-4">
      <Card
        title="Resource limits and health checks"
        description="Per-process CPU, memory, replicas and deploy-gating health checks for the application bound to this server."
        icon={<Gauge size={18} />}
        badge={
          application ? (
            <StatusPill tone={aggregate === "healthy" ? "success" : aggregate === "unhealthy" ? "danger" : aggregate === "pending" ? "warning" : "neutral"} pulse={aggregate === "pending"}>
              {healthLabel(aggregate === "unknown" ? undefined : aggregate)}
            </StatusPill>
          ) : null
        }
      >
        <p className="ui-hint">
          Limits are enforced by Docker once they are written into the compose document; the panel does not restart containers on its own.
          Stored configuration and enforced configuration are two different things — the buttons below close the gap.
        </p>
      </Card>

      {queryError ? <Alert tone="error" title="Could not load process configuration">{queryError}</Alert> : null}

      {limitsQuery.isLoading ? <CardSkeleton /> : null}

      {!limitsQuery.isLoading && !application ? (
        <EmptyState
          icon={<ServerOff size={20} />}
          title="No application bound to this server"
          description="Process configuration belongs to an application. Bind one to this server first, then its process types will appear here."
        />
      ) : null}

      {application && rollback ? (
        <Alert
          tone={rollback.rollback ? "error" : rollback.gatingProcesses.length === 0 ? "warning" : "success"}
          title={rollback.rollback ? "Rollback recommended" : rollback.gatingProcesses.length === 0 ? "No deploy-gating checks" : "Deploy gating is satisfied"}
        >
          {rollback.reason}
          {rollback.gatingProcesses.length > 0 ? ` Grace window: ${secondsToDuration(rollback.graceWindowSeconds)}.` : null}
        </Alert>
      ) : null}

      {processes.map((config) => {
        const draft = drafts[config.processType];
        const state = healthByProcess.get(config.processType);
        if (!draft) return null;
        const update = (patch: Partial<Draft>) => setDrafts((current) => ({ ...current, [config.processType]: { ...current[config.processType], ...patch } }));
        const stored = draftFromConfig(config);
        const dirty = JSON.stringify(draft) !== JSON.stringify(stored);
        const desiredReplicas = Number.parseInt(draft.replicas, 10);
        const replicasValid = Number.isFinite(desiredReplicas) && desiredReplicas >= 0 && desiredReplicas <= MAX_REPLICAS;
        const requestScale = (next: number) => {
          update({ replicas: String(next) });
          scaleMutation.mutate({ processType: config.processType, replicas: next });
        };
        return (
          <Card
            key={config.processType}
            title={config.processType}
            description={`CPU ${formatCpuLimit(config.cpuLimitNanoCores)} · memory ${formatMemoryLimit(config.memoryLimitBytes)} · ${config.replicas} replica${config.replicas === 1 ? "" : "s"}`}
            icon={<Activity size={18} />}
            badge={
              <div className="flex flex-wrap items-center gap-2">
                {!config.enabled ? <Badge tone="neutral">Disabled</Badge> : null}
                <StatusPill tone={healthTone(state?.status)} pulse={state?.status === "pending"}>{healthLabel(state?.status)}</StatusPill>
              </div>
            }
          >
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <label className="block text-xs font-semibold text-slate-300">
                CPU limit (cores)
                <input className="ui-input mt-2 w-full" inputMode="decimal" onChange={(event) => update({ cpuCores: event.target.value })} placeholder="No limit" value={draft.cpuCores} />
              </label>
              <label className="block text-xs font-semibold text-slate-300">
                Memory limit (MB)
                <input className="ui-input mt-2 w-full" inputMode="numeric" onChange={(event) => update({ memoryMb: event.target.value })} placeholder="No limit" value={draft.memoryMb} />
              </label>
              <label className="block text-xs font-semibold text-slate-300">
                Replicas
                <div className="mt-2 flex items-center gap-2">
                  <button aria-label={`Decrease ${config.processType} replicas`} className="ui-button ui-button-secondary px-3" disabled={!canView || !replicasValid || desiredReplicas <= 0 || scaleMutation.isPending} onClick={() => requestScale(desiredReplicas - 1)} title="Scale this process type now" type="button">−</button>
                  <input aria-label={`${config.processType} replicas`} className="ui-input w-20 text-center" inputMode="numeric" onChange={(event) => update({ replicas: event.target.value })} value={draft.replicas} />
                  <button aria-label={`Increase ${config.processType} replicas`} className="ui-button ui-button-secondary px-3" disabled={!canView || !replicasValid || desiredReplicas >= MAX_REPLICAS || scaleMutation.isPending} onClick={() => requestScale(desiredReplicas + 1)} title="Scale this process type now" type="button">+</button>
                </div>
              </label>
              <div className="flex flex-col justify-end gap-3">
                <Switch checked={draft.enabled} disabled={!canView} label="Enforce this process type" onCheckedChange={(checked) => update({ enabled: checked })} />
                <button
                  className="ui-button ui-button-secondary"
                  disabled={!canView || !replicasValid || !dirty || saveMutation.isPending}
                  onClick={() => saveMutation.mutate({ processType: config.processType, input: inputFromDraft(draft) })}
                  title={!replicasValid ? `Replicas must be a whole number between 0 and ${MAX_REPLICAS}` : undefined}
                  type="button"
                >
                  <Save size={15} />{saveMutation.isPending && dirty ? "Saving…" : dirty ? "Save changes" : "Saved"}
                </button>
              </div>
            </div>

            {state?.detail ? <p className="ui-hint mt-3">Health: {state.detail}</p> : null}

            <div className="mt-4 rounded-xl border border-white/[0.07] bg-white/[0.02] p-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="flex items-center gap-2 text-sm font-bold text-white"><HeartPulse size={16} />Health check</p>
                <div className="flex items-center gap-4">
                  <Switch checked={draft.gating} disabled={!canView || draft.checkType === ""} label="Gate deploys on this check" onCheckedChange={(checked) => update({ gating: checked })} />
                  <select
                    aria-label={`${config.processType} health check type`}
                    className="ui-input"
                    disabled={!canView}
                    onChange={(event) => update({ checkType: event.target.value as HealthCheckType })}
                    value={draft.checkType}
                  >
                    <option value="">No check</option>
                    <option value="http">HTTP</option>
                    <option value="tcp">TCP</option>
                    <option value="command">Command</option>
                  </select>
                </div>
              </div>

              {draft.checkType === "" ? (
                <p className="ui-hint mt-3">Nothing is probed, so nothing can roll this release back. &ldquo;Unknown&rdquo; below is not a passing grade.</p>
              ) : (
                <div className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                  {draft.checkType === "http" ? (
                    <>
                      <label className="block text-xs font-semibold text-slate-300">Path<input className="ui-input mt-2 w-full" onChange={(event) => update({ path: event.target.value })} value={draft.path} /></label>
                      <label className="block text-xs font-semibold text-slate-300">Port<input className="ui-input mt-2 w-full" inputMode="numeric" onChange={(event) => update({ port: event.target.value })} value={draft.port} /></label>
                    </>
                  ) : null}
                  {draft.checkType === "tcp" ? (
                    <label className="block text-xs font-semibold text-slate-300">Port<input className="ui-input mt-2 w-full" inputMode="numeric" onChange={(event) => update({ port: event.target.value })} value={draft.port} /></label>
                  ) : null}
                  {draft.checkType === "command" ? (
                    <label className="block text-xs font-semibold text-slate-300 sm:col-span-2 lg:col-span-3">Command
                      <input className="ui-input mt-2 w-full font-mono" onChange={(event) => update({ command: event.target.value })} placeholder="pg_isready -U postgres" value={draft.command} />
                    </label>
                  ) : null}
                  <label className="block text-xs font-semibold text-slate-300">Interval (s)<input className="ui-input mt-2 w-full" inputMode="numeric" onChange={(event) => update({ interval: event.target.value })} value={draft.interval} /></label>
                  <label className="block text-xs font-semibold text-slate-300">Timeout (s)<input className="ui-input mt-2 w-full" inputMode="numeric" onChange={(event) => update({ timeout: event.target.value })} value={draft.timeout} /></label>
                  <label className="block text-xs font-semibold text-slate-300">Retries<input className="ui-input mt-2 w-full" inputMode="numeric" onChange={(event) => update({ retries: event.target.value })} value={draft.retries} /></label>
                  <label className="block text-xs font-semibold text-slate-300">Start period (s)<input className="ui-input mt-2 w-full" inputMode="numeric" onChange={(event) => update({ startPeriod: event.target.value })} value={draft.startPeriod} /></label>
                  <p className="ui-hint self-end">Grace window {secondsToDuration(state?.graceWindowSeconds ?? 0)} — the panel waits this long for a passing probe before it trusts the verdict.</p>
                </div>
              )}

              <div className="mt-4 flex flex-wrap items-center gap-2">
                <button
                  className="ui-button ui-button-secondary"
                  disabled={!canProbe || !application || probeMutation.isPending}
                  onClick={() => probeMutation.mutate({ processType: config.processType, healthy: true })}
                  title="Append a passing observation to the health log"
                  type="button"
                >Record passing probe</button>
                <button
                  className="ui-button ui-button-secondary"
                  disabled={!canProbe || !application || probeMutation.isPending}
                  onClick={() => probeMutation.mutate({ processType: config.processType, healthy: false })}
                  title="Append a failing observation to the health log"
                  type="button"
                >Record failing probe</button>
                <button className="ui-button ui-button-danger ml-auto" disabled={!canView || deleteMutation.isPending} onClick={() => setDeleteTarget(config.processType)} type="button"><Trash2 size={14} />Remove</button>
              </div>
              {!canProbe ? <p className="ui-hint mt-2">Recording a probe requires the workload-control permission.</p> : null}
            </div>
          </Card>
        );
      })}

      {application ? (
        <Card description="Process types must match a service name in the application's compose document, or they cannot be enforced." icon={<Plus size={18} />} title="Add a process type">
          <div className="flex flex-col gap-2 sm:flex-row">
            <input
              aria-label="New process type"
              className="ui-input min-w-0 flex-1 font-mono"
              list="forge-process-type-suggestions"
              onChange={(event) => setNewProcessType(event.target.value.toLowerCase())}
              placeholder="web"
              value={newProcessType}
            />
            <datalist id="forge-process-type-suggestions">
              {suggestedProcessTypes.filter((type) => !processes.some((config) => config.processType === type)).map((type) => <option key={type} value={type} />)}
            </datalist>
            <button
              className="ui-button ui-button-primary"
              disabled={!canView || !newProcessType.trim() || saveMutation.isPending || Boolean(drafts[newProcessType.trim()])}
              onClick={() => {
                const processType = newProcessType.trim();
                setDrafts((current) => ({ ...current, [processType]: emptyDraft() }));
                saveMutation.mutate({ processType, input: inputFromDraft(emptyDraft()) });
                setNewProcessType("");
              }}
              type="button"
            >
              {saveMutation.isPending ? "Saving…" : drafts[newProcessType.trim()] ? "Already added" : "Create configuration"}
            </button>
          </div>
          <p className="ui-hint mt-2">Names are lowercased and limited to letters, digits, <code>-</code>, <code>_</code> and <code>.</code> because they double as compose service keys.</p>
        </Card>
      ) : null}

      {application ? (
        <Card description="Render the stored configuration into the application's compose document, which is what the deploy actually ships." icon={<Rocket size={18} />} title="Enforce on the compose document">
          <div className="flex flex-wrap gap-2">
            <button className="ui-button ui-button-secondary" disabled={!canView || previewMutation.isPending || mutating} onClick={() => previewMutation.mutate()} type="button">
              <RefreshCw size={15} />{previewMutation.isPending ? "Rendering…" : "Preview render"}
            </button>
            <button className="ui-button ui-button-primary" disabled={!canView || applyMutation.isPending || mutating} onClick={() => applyMutation.mutate(false)} type="button">
              <Save size={15} />{applyMutation.isPending ? "Applying…" : "Apply to document"}
            </button>
            <button className="ui-button ui-button-primary" disabled={!canView || applyMutation.isPending || mutating} onClick={() => applyMutation.mutate(true)} type="button">
              <Rocket size={15} />{applyMutation.isPending ? "Working…" : "Apply and deploy"}
            </button>
          </div>
          {renderResult ? (
            <div className="mt-4 space-y-3">
              <Alert
                tone={renderResult.skipped > 0 ? "warning" : "success"}
                title={`${renderResult.applied} process type${renderResult.applied === 1 ? "" : "s"} ${renderResult.mode === "preview" ? "rendered" : "written"}${renderResult.skipped > 0 ? `, ${renderResult.skipped} skipped` : ""}`}
              >
                {renderResult.skippedReasons.length > 0 ? (
                  <ul className="mt-1 list-disc space-y-1 pl-5">{renderResult.skippedReasons.map((reason) => <li key={reason}>{reason}</li>)}</ul>
                ) : renderResult.mode === "apply" ? (
                  "Every enabled process configuration landed in the document."
                ) : (
                  "Nothing was written; this is the document a deploy would ship."
                )}
              </Alert>
              <pre className="max-h-72 overflow-auto rounded-xl bg-[var(--canvas)] p-3 font-mono text-xs text-slate-300">{renderResult.yaml}</pre>
            </div>
          ) : (
            <p className="ui-hint mt-3">Preview changes nothing. &ldquo;Apply to document&rdquo; stores the enriched document so the next deploy ships it; &ldquo;Apply and deploy&rdquo; also runs that deploy now.</p>
          )}
          <p className="ui-hint mt-2">Applying refuses to write when a process type has no matching compose service, so a half-enforced document is never stored silently.</p>
        </Card>
      ) : null}

      {observations.length > 0 ? (
        <Card icon={<HeartPulse size={18} />} title="Recent health observations" description="Newest first, from this server's probe log.">
          <ul className="space-y-2">
            {observations.slice(0, 15).map((observation) => (
              <li className="flex flex-wrap items-center gap-3 rounded-lg bg-white/[0.03] px-3 py-2 text-sm" key={observation.id}>
                <StatusPill tone={observation.healthy ? "success" : "danger"}>{observation.healthy ? "Pass" : "Fail"}</StatusPill>
                <span className="font-mono text-slate-200">{observation.processType}</span>
                <span className="min-w-0 flex-1 truncate text-slate-400" title={observation.detail}>{observation.detail || "No detail"}</span>
                <span className="text-xs text-slate-500">{new Date(observation.observedAt).toLocaleString()}</span>
              </li>
            ))}
          </ul>
        </Card>
      ) : null}

      <ConfirmDialog
        closeAction={() => { if (!deleteMutation.isPending) setDeleteTarget(null); }}
        confirmAction={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget); }}
        confirmLabel="Remove configuration"
        destructive
        description={`The stored configuration for "${deleteTarget ?? ""}" will be deleted. The application's compose document keeps whatever was rendered into it until you apply it again.`}
        loading={deleteMutation.isPending}
        open={Boolean(deleteTarget)}
        title={`Remove ${deleteTarget ?? ""}?`}
      />
    </div>
  );
}
