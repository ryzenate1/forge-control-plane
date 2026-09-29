"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, GitBranch, RefreshCw, Rocket, Server, ShieldAlert, Square, Trash2, Workflow, Layers } from "lucide-react";
import {
  drainNomadNode,
  fetchNomadAllocations,
  fetchNomadDeployments,
  fetchNomadJobs,
  fetchNomadNodes,
  stopNomadJob,
  submitNomadJob,
  type NomadAllocation,
  type NomadDeployment,
  type NomadJob,
  type NomadNode,
} from "@/lib/api/nomad";
import { nomadStatusTone } from "@/lib/api/status";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageLayout,
  AdminTabs,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
  Textarea,
} from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { errorMessage, formatDate } from "@/lib/utils";

const TABS = [
  { id: "jobs", label: "Jobs", icon: Workflow },
  { id: "allocations", label: "Allocations", icon: Activity },
  { id: "nodes", label: "Nodes", icon: Server },
  { id: "deployments", label: "Deployments", icon: GitBranch },
] as const;

type Tab = (typeof TABS)[number]["id"];

/**
 * The Nomad routes answer in product names ("Forge Orchestration service
 * unavailable", ErrNotConfigured) that appear nowhere else in this UI. Map the
 * two shapes an operator can act on; pass anything else through with its reason.
 */
function explainNomadError(error: unknown): string {
  const raw = errorMessage(error, "The Nomad request failed.");
  if (/not configured/i.test(raw)) return "Nomad is not configured for this panel (no control-plane endpoint or token), so there is nothing to read or submit.";
  if (/service unavailable/i.test(raw)) return "Nomad is not configured for this panel, so this action is unavailable.";
  return raw.replace(/Forge Orchestration/g, "Nomad");
}

function isNotConfigured(error: unknown): boolean {
  const raw = errorMessage(error, "");
  return /not configured|service unavailable/i.test(raw);
}

const NOT_REPORTED = "Not reported";

export default function NomadAdminPage() {
  const [tab, setTab] = useState<Tab>("jobs");
  const [submitOpen, setSubmitOpen] = useState(false);
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirmAction, renderConfirm] = useConfirm();

  const jobsQ = useQuery({ queryKey: ["nomad-jobs"], queryFn: () => fetchNomadJobs(), enabled: tab === "jobs" });
  const allocsQ = useQuery({ queryKey: ["nomad-allocations"], queryFn: () => fetchNomadAllocations(), enabled: tab === "allocations" });
  const nodesQ = useQuery({ queryKey: ["nomad-nodes"], queryFn: () => fetchNomadNodes(), enabled: tab === "nodes" });
  const deploymentsQ = useQuery({ queryKey: ["nomad-deployments"], queryFn: () => fetchNomadDeployments(), enabled: tab === "deployments" });

  const activeQuery = tab === "jobs" ? jobsQ : tab === "allocations" ? allocsQ : tab === "nodes" ? nodesQ : deploymentsQ;

  // The control plane answers 503 / ErrNotConfigured when Nomad is not wired up.
  // That is a capability, not an empty fleet, so the write actions are disabled
  // with a reason instead of inviting a guaranteed failure.
  const notConfigured = useMemo(
    () => [jobsQ, allocsQ, nodesQ, deploymentsQ].some((query) => query.isError && isNotConfigured(query.error)),
    [jobsQ, allocsQ, nodesQ, deploymentsQ],
  );

  const refresh = () => {
    for (const key of ["nomad-jobs", "nomad-allocations", "nomad-nodes", "nomad-deployments"]) {
      void qc.invalidateQueries({ queryKey: [key] });
    }
  };

  const onError = (action: string) => (err: unknown) =>
    toast({ tone: "error", title: `Failed to ${action}`, message: explainNomadError(err) });

  const stopMut = useMutation({
    mutationFn: (vars: { id: string; purge: boolean }) => stopNomadJob(vars.id, vars.purge),
    onSuccess: (_r, vars) => {
      toast({
        tone: "success",
        title: vars.purge ? "Job stopped and purged" : "Job stopped",
        message: vars.purge
          ? `${vars.id} is gone from Nomad, including its evaluation history — it cannot be restarted from this page.`
          : `${vars.id} stopped; its definition stays in Nomad and can be started again.`,
      });
      void qc.invalidateQueries({ queryKey: ["nomad-jobs"] });
    },
    onError: onError("stop job"),
  });

  const drainMut = useMutation({
    mutationFn: (vars: { drain: boolean; id: string }) => drainNomadNode(vars.id, vars.drain),
    onSuccess: (data) => {
      toast({ tone: "success", title: data.drain ? "Node draining" : "Node marked eligible", message: data.node });
      void qc.invalidateQueries({ queryKey: ["nomad-nodes"] });
    },
    onError: onError("update node drain"),
  });

  const handleStopJob = async (job: NomadJob, purge: boolean) => {
    const label = job.Name || job.ID;
    const ok = await confirmAction({
      confirmLabel: purge ? "Stop and purge" : "Stop job",
      danger: purge,
      description: purge
        ? `${label} will be stopped and then purged from Nomad: the job definition and its evaluation history are removed, and this page cannot recreate it. Allocations stop according to the job's own update stanza.`
        : `${label} will be stopped on this cluster. Its job definition stays in Nomad, so it can be started again; allocations drain according to the job's own update stanza.`,
      title: purge ? `Stop and purge ${label}?` : `Stop ${label}?`,
    });
    if (ok) stopMut.mutate({ id: job.ID, purge });
  };

  const handleDrain = async (node: NomadNode) => {
    const label = node.Name || node.ID;
    const draining = Boolean(node.Drain);
    const ok = await confirmAction({
      confirmLabel: draining ? "Mark eligible" : "Drain node",
      danger: !draining,
      description: draining
        ? `${label} becomes eligible for new placements again. Existing stopped allocations do not come back automatically.`
        : `${label} will be marked ineligible and Nomad will migrate its allocations away — this changes fleet placement immediately, the same effect as /admin/drain.`,
      title: draining ? `Mark ${label} eligible?` : `Drain ${label}?`,
    });
    if (ok) drainMut.mutate({ drain: !draining, id: node.ID });
  };

  return (
    <AdminPageLayout>
      {renderConfirm()}
      <SectionHeader
        info={{
          description: "Jobs, allocations, client nodes and deployments read from the configured Nomad control plane.",
          eyebrow: "Architecture & Semantics",
          sections: [
            {
              content:
                "Stop leaves the job definition in Nomad so it can be started again; Stop and purge removes the definition and its evaluation history, which this page cannot undo. Both ask first. Draining a client node marks it ineligible and migrates its allocations away.",
              icon: Layers,
              title: "Jobs, stops and drains",
            },
            {
              content:
                "Nomad is an optional control plane. When it is not configured the API answers 503 and this page says so: submit, stop and drain actions stay disabled with the reason rather than producing a failure toasts.",
              icon: ShieldAlert,
              title: "Capability",
            },
            {
              content:
                "Job and node fields the control plane did not report render as \"Not reported\" rather than being filled in with a plausible default such as the \"service\" job type.",
              icon: Activity,
              title: "Unreported fields",
            },
          ],
          title: "Nomad workloads",
          triggerLabel: "About Nomad",
        }}
        status={<FreshnessBadge state={sourceState(activeQuery)} />}
        action={
          <div className="flex flex-wrap items-center gap-2">
            <Btn onClick={refresh} size="sm" tone="ghost"><RefreshCw size={14} /> Refresh</Btn>
            <Btn
              disabled={notConfigured}
              onClick={() => setSubmitOpen(true)}
              size="sm"
              tone="primary"
              title={notConfigured ? "Nomad is not configured for this panel" : "Submit a Nomad job document"}
            >
              <Rocket size={14} /> Submit job
            </Btn>
          </div>
        }
      />

      {notConfigured ? (
        <AdminErrorState
          message="Nomad is not configured for this panel, so no job, allocation, node or deployment data can be read and none of the actions below will succeed."
          retry={refresh}
        />
      ) : null}

      <AdminTabs active={tab} label="Nomad sections" onChange={(id) => setTab(id as Tab)} tabs={TABS.map((entry) => ({ id: entry.id, label: entry.label, icon: entry.icon }))} />

      <div aria-label={`${TABS.find((entry) => entry.id === tab)?.label ?? "Nomad"} section`} role="tabpanel">
        {tab === "jobs" && (
          <Card>
            <CardHeader icon={Workflow} title="Jobs" />
            {jobsQ.isPending ? <AdminLoadingState label="Loading jobs…" /> : jobsQ.isError ? <div className="p-4"><AdminErrorState message={explainNomadError(jobsQ.error)} retry={() => void jobsQ.refetch()} /></div> : (jobsQ.data ?? []).length === 0 ? (
              <EmptyState icon={Workflow} message="The Nomad control plane reported no jobs." title="No jobs" />
            ) : (
              <div className="overflow-x-auto">
                <table aria-label="Nomad jobs" className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-line text-left text-xs uppercase tracking-wider text-text-subtle">
                      <th className="px-4 py-2.5 font-medium">Name</th>
                      <th className="px-4 py-2.5 font-medium">ID</th>
                      <th className="px-4 py-2.5 font-medium">Status</th>
                      <th className="px-4 py-2.5 font-medium">Type</th>
                      <th className="px-4 py-2.5 font-medium">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {(jobsQ.data as NomadJob[]).map((job) => {
                      const rowBusy = stopMut.isPending && stopMut.variables?.id === job.ID;
                      return (
                        <tr className="hover:bg-overlay-subtle" key={job.ID}>
                          <td className="px-4 py-3 text-text">{job.Name || NOT_REPORTED}</td>
                          <td className="px-4 py-3 font-mono text-xs text-text-subtle">{job.ID}</td>
                          <td className="px-4 py-3"><Pill tone={nomadStatusTone(job.Status)}>{job.Status || NOT_REPORTED}</Pill></td>
                          <td className="px-4 py-3 text-xs text-text-subtle">{job.Type || `Type ${NOT_REPORTED}`}</td>
                          <td className="px-4 py-3">
                            <div className="flex flex-wrap gap-1">
                              <Btn
                                ariaLabel={`Stop job ${job.Name || job.ID}`}
                                disabled={notConfigured || job.Status === "stopped" || rowBusy}
                                loading={rowBusy && stopMut.variables?.purge === false}
                                onClick={() => void handleStopJob(job, false)}
                                size="sm"
                                tone="warning"
                                title="Stop the job; the definition stays in Nomad"
                              >
                                <Square size={12} /> Stop
                              </Btn>
                              <Btn
                                ariaLabel={`Stop and purge job ${job.Name || job.ID}`}
                                disabled={notConfigured || rowBusy}
                                loading={rowBusy && stopMut.variables?.purge === true}
                                onClick={() => void handleStopJob(job, true)}
                                size="sm"
                                tone="danger"
                                title="Stop and purge: removes the job definition and evaluation history"
                              >
                                <Trash2 size={12} /> Purge
                              </Btn>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        )}

        {tab === "allocations" && (
          <Card>
            <CardHeader icon={Activity} title="Allocations" />
            {allocsQ.isPending ? <AdminLoadingState label="Loading allocations…" /> : allocsQ.isError ? <div className="p-4"><AdminErrorState message={explainNomadError(allocsQ.error)} retry={() => void allocsQ.refetch()} /></div> : (allocsQ.data ?? []).length === 0 ? (
              <EmptyState icon={Activity} message="The Nomad control plane reported no allocations." title="No allocations" />
            ) : (
              <div className="overflow-x-auto">
                <table aria-label="Nomad allocations" className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-line text-left text-xs uppercase tracking-wider text-text-subtle">
                      <th className="px-4 py-2.5 font-medium">ID</th>
                      <th className="px-4 py-2.5 font-medium">Job</th>
                      <th className="px-4 py-2.5 font-medium">Task group</th>
                      <th className="px-4 py-2.5 font-medium">Client status</th>
                      <th className="px-4 py-2.5 font-medium">Node</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {(allocsQ.data as NomadAllocation[]).map((allocation) => (
                      <tr className="hover:bg-overlay-subtle" key={allocation.ID}>
                        <td className="px-4 py-3 font-mono text-xs text-text">{allocation.ID.slice(0, 8)}</td>
                        <td className="px-4 py-3 font-mono text-xs text-text-subtle">{allocation.JobID || NOT_REPORTED}</td>
                        <td className="px-4 py-3 text-xs text-text-subtle">{allocation.TaskGroup || NOT_REPORTED}</td>
                        <td className="px-4 py-3"><Pill tone={nomadStatusTone(allocation.ClientStatus)}>{allocation.ClientStatus || NOT_REPORTED}</Pill></td>
                        <td className="px-4 py-3 font-mono text-xs text-text-subtle">{allocation.NodeID ? allocation.NodeID.slice(0, 8) : NOT_REPORTED}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        )}

        {tab === "nodes" && (
          <Card>
            <CardHeader icon={Server} title="Nodes" />
            {nodesQ.isPending ? <AdminLoadingState label="Loading nodes…" /> : nodesQ.isError ? <div className="p-4"><AdminErrorState message={explainNomadError(nodesQ.error)} retry={() => void nodesQ.refetch()} /></div> : (nodesQ.data ?? []).length === 0 ? (
              <EmptyState icon={Server} message="The Nomad control plane reported no client nodes." title="No client nodes" />
            ) : (
              <div className="overflow-x-auto">
                <table aria-label="Nomad client nodes" className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-line text-left text-xs uppercase tracking-wider text-text-subtle">
                      <th className="px-4 py-2.5 font-medium">Name</th>
                      <th className="px-4 py-2.5 font-medium">Node ID</th>
                      <th className="px-4 py-2.5 font-medium">Address</th>
                      <th className="px-4 py-2.5 font-medium">Status</th>
                      <th className="px-4 py-2.5 font-medium">Eligibility</th>
                      <th className="px-4 py-2.5 font-medium">Version</th>
                      <th className="px-4 py-2.5 font-medium">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {(nodesQ.data as NomadNode[]).map((node) => {
                      const rowBusy = drainMut.isPending && drainMut.variables?.id === node.ID;
                      const draining = Boolean(node.Drain);
                      return (
                        <tr className="hover:bg-overlay-subtle" key={node.ID}>
                          <td className="px-4 py-3 text-text">{node.Name || `Name ${NOT_REPORTED}`}</td>
                          <td className="px-4 py-3 font-mono text-xs text-text-subtle">{node.ID.slice(0, 12)}…</td>
                          <td className="px-4 py-3 font-mono text-xs text-text-subtle">{node.HTTPAddr || NOT_REPORTED}</td>
                          <td className="px-4 py-3"><Pill tone={nomadStatusTone(node.Status)}>{node.Status || NOT_REPORTED}</Pill></td>
                          <td className="px-4 py-3">
                            <div className="flex flex-wrap gap-1.5">
                              {/* Drain and eligibility are two facts: a node can be ineligible
                                  without this page having drained it, so the button never infers. */}
                              <Pill tone={draining ? "warn" : "neutral"}>{draining ? "draining" : "not draining"}</Pill>
                              <Pill tone="neutral">{node.Eligibility || `Eligibility ${NOT_REPORTED}`}</Pill>
                            </div>
                          </td>
                          <td className="px-4 py-3 text-xs text-text-subtle">{node.Version || NOT_REPORTED}</td>
                          <td className="px-4 py-3">
                            <Btn
                              ariaLabel={draining ? `Mark ${node.Name || node.ID} eligible` : `Drain ${node.Name || node.ID}`}
                              disabled={notConfigured || rowBusy}
                              loading={rowBusy}
                              onClick={() => void handleDrain(node)}
                              size="sm"
                              tone={draining ? "ghost" : "warning"}
                              title={draining ? "Return this node to the placement set" : "Mark ineligible and migrate allocations away"}
                            >
                              {draining ? "Mark eligible" : "Drain"}
                            </Btn>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        )}

        {tab === "deployments" && (
          <Card>
            <CardHeader icon={GitBranch} title="Deployments" />
            {deploymentsQ.isPending ? <AdminLoadingState label="Loading deployments…" /> : deploymentsQ.isError ? <div className="p-4"><AdminErrorState message={explainNomadError(deploymentsQ.error)} retry={() => void deploymentsQ.refetch()} /></div> : (deploymentsQ.data ?? []).length === 0 ? (
              <EmptyState icon={GitBranch} message="The Nomad control plane reported no deployments." title="No deployments" />
            ) : (
              <div className="overflow-x-auto">
                <table aria-label="Nomad deployments" className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-line text-left text-xs uppercase tracking-wider text-text-subtle">
                      <th className="px-4 py-2.5 font-medium">ID</th>
                      <th className="px-4 py-2.5 font-medium">Job</th>
                      <th className="px-4 py-2.5 font-medium">Status</th>
                      <th className="px-4 py-2.5 font-medium">Desired</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {(deploymentsQ.data as NomadDeployment[]).map((deployment) => (
                      <tr className="hover:bg-overlay-subtle" key={deployment.ID}>
                        <td className="px-4 py-3 font-mono text-xs text-text">{deployment.ID.slice(0, 8)}</td>
                        <td className="px-4 py-3 font-mono text-xs text-text-subtle">{deployment.JobID || NOT_REPORTED}</td>
                        <td className="px-4 py-3">
                          <div className="flex flex-wrap gap-1.5">
                            <Pill tone={nomadStatusTone(deployment.Status)}>{deployment.Status || NOT_REPORTED}</Pill>
                            {/* A paused or canary rollout is the state an operator most needs to
                                see during a deploy; both were dropped entirely. */}
                            {deployment.Pause ? <Pill tone="warn">paused</Pill> : null}
                            {deployment.Canary ? <Pill tone="info">canary</Pill> : null}
                          </div>
                          {deployment.StatusDescription ? (
                            <p className="mt-1 max-w-96 text-xs text-text-muted">{deployment.StatusDescription}</p>
                          ) : null}
                        </td>
                        <td className="px-4 py-3 text-xs text-text-subtle">{deployment.DesiredStatus || NOT_REPORTED}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        )}
      </div>

      {submitOpen && (
        <SubmitJobModal
          onClose={() => setSubmitOpen(false)}
          onSubmitted={(id) => {
            toast({ tone: "success", title: "Job submitted", message: `${id} registered with Nomad.` });
            void qc.invalidateQueries({ queryKey: ["nomad-jobs"] });
            setSubmitOpen(false);
          }}
          onError={onError("submit job")}
        />
      )}
    </AdminPageLayout>
  );
}

function SubmitJobModal({ onClose, onSubmitted, onError }: { onClose: () => void; onSubmitted: (id: string) => void; onError: (err: Error) => void }) {
  const [spec, setSpec] = useState("");
  const [invalid, setInvalid] = useState("");

  const submitMut = useMutation({
    mutationFn: async () => {
      // Nomad's HTTP API consumes a JSON job document. The panel forwards the
      // body verbatim (handlers_nomad.go), so non-JSON used to be wrapped as
      // `{ hcl: "…" }` and rejected by Nomad for a reason that looked like a
      // job problem. The shape is checked here instead.
      const text = spec.trim();
      let payload: Record<string, unknown>;
      try {
        payload = JSON.parse(text) as Record<string, unknown>;
      } catch {
        throw new Error("A JSON job document is required. Render HCL to JSON first with `nomad job run -output` or a JSON converter.");
      }
      if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
        throw new Error("The job document must be a JSON object at the top level.");
      }
      return submitNomadJob(payload);
    },
    onSuccess: (res) => {
      const id = res?.job?.ID;
      if (!id) {
        onError(new Error("Nomad accepted the request but returned no job id, so the job may not have been created. Check the Jobs tab."));
        return;
      }
      onSubmitted(id);
    },
    onError: (err: Error) => onError(err),
  });

  return (
    <Modal description="Paste a Nomad job document. The control-plane HTTP API consumes JSON only; HCL must be rendered to JSON first." onClose={onClose} title="Submit Nomad job">
      <div className="space-y-4">
        <Textarea label="Job specification (JSON)" onChange={(value) => { setSpec(value); setInvalid(""); }} placeholder={'{\n  "Job": {\n    "ID": "example",\n    "Type": "service",\n    "Datacenters": ["dc1"],\n    "TaskGroups": []\n  }\n}'} rows={14} value={spec} />
        {invalid ? <AdminErrorState message={invalid} /> : null}
        <p className="text-xs leading-5 text-text-subtle">
          The panel forwards this document to Nomad unchanged and does not parse HCL. Run{" "}
          <code className="font-mono">nomad job run -output</code> to render HCL into a job descriptor first.
        </p>
        <ModalFooter
          confirmLabel="Submit"
          disabled={submitMut.isPending || !spec.trim()}
          onCancel={onClose}
          onConfirm={() => {
            try {
              JSON.parse(spec.trim());
              setInvalid("");
            } catch {
              setInvalid("That is not valid JSON, so it was not sent. Nomad only accepts a JSON job document.");
              return;
            }
            void submitMut.mutate();
          }}
        />
      </div>
    </Modal>
  );
}
