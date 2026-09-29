"use client";

import { useMemo } from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity, Box, Container, Cpu, Database, Network, RefreshCw, Server,
} from "lucide-react";
import {
  ApiError, fetchEndpoint, fetchEndpointDiagnostics, fetchEndpointInventory,
  fetchEndpointNodes, fetchEndpointHealthHistory, fetchEndpointAccessPolicies,
} from "@/lib/api";
import { AdminPageLayout, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, AdminLoadingState, AdminErrorState, Btn, Card, CardHeader, EmptyState, PermissionDeniedState, Pill, SectionHeader } from "./admin-ui";
import { FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { formatDate } from "@/lib/utils";
import Link from "next/link";

// Chips come from `Pill tone`; a status this map did not know used to render as a
// borderless chip holding the raw wire word, and looked different again from the
// list page's second copy of the same map.
const STATUS_TONE: Record<string, "green" | "yellow" | "red" | "info" | "unknown"> = {
  online: "green",
  degraded: "yellow",
  offline: "red",
  provisioning: "info",
  unknown: "unknown",
};

function StatusPill({ value }: { value?: string }) {
  if (!value) return <Pill tone="unknown">Status not reported</Pill>;
  return <Pill tone={STATUS_TONE[value] ?? "unknown"}>{value}</Pill>;
}

/** Memory/disk readouts. A missing figure is not 0 MB and never renders as
 *  "undefined MB"; `formatMB` used to fall through to string concatenation. */
function formatMB(mb?: number | null): string {
  if (mb === undefined || mb === null || Number.isNaN(mb)) return "Not reported";
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
  return `${mb} MB`;
}

function countOrUnknown(value: number | undefined | null): string {
  return value === undefined || value === null ? "Not reported" : String(value);
}

function percentOrUnknown(score?: number | null): string {
  if (score === undefined || score === null) return "Not scored";
  const pct = score * 100;
  if (Number.isNaN(pct)) return "Not scored";
  return `${pct.toFixed(0)}%`;
}

export function AdminEndpointDetail() {
  const params = useParams();
  const router = useRouter();
  const qc = useQueryClient();
  const id = params.id as string;

  const epQuery = useQuery({ queryKey: ["infra-endpoint", id], queryFn: () => fetchEndpoint(id) });
  const diagQuery = useQuery({ queryKey: ["infra-endpoint-diag", id], queryFn: () => fetchEndpointDiagnostics(id) });
  const invQuery = useQuery({ queryKey: ["infra-endpoint-inv", id], queryFn: () => fetchEndpointInventory(id) });
  const nodesQuery = useQuery({ queryKey: ["infra-endpoint-nodes", id], queryFn: () => fetchEndpointNodes(id) });
  const healthQuery = useQuery({ queryKey: ["infra-endpoint-health", id], queryFn: () => fetchEndpointHealthHistory(id, 20) });
  const policiesQuery = useQuery({ queryKey: ["infra-endpoint-policies", id], queryFn: () => fetchEndpointAccessPolicies(id) });

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["infra-endpoint", id] });
    qc.invalidateQueries({ queryKey: ["infra-endpoint-diag", id] });
    qc.invalidateQueries({ queryKey: ["infra-endpoint-inv", id] });
    qc.invalidateQueries({ queryKey: ["infra-endpoint-nodes", id] });
    qc.invalidateQueries({ queryKey: ["infra-endpoint-health", id] });
    qc.invalidateQueries({ queryKey: ["infra-endpoint-policies", id] });
  };

  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const health = useMemo(() => healthQuery.data ?? [], [healthQuery.data]);
  const policies = useMemo(() => policiesQuery.data ?? [], [policiesQuery.data]);

  if (epQuery.isPending) {
    return (
      <AdminPageLayout>
        <AdminLoadingState label="Loading endpoint…" />
      </AdminPageLayout>
    );
  }

  if (epQuery.isError) {
    const forbidden = epQuery.error instanceof ApiError && epQuery.error.status === 403;
    const missing = epQuery.error instanceof ApiError && epQuery.error.status === 404;
    return (
      <AdminPageLayout>
        <SectionHeader backAction={() => router.push("/admin/endpoints")} backLabel="Endpoints" title={missing ? "Endpoint not found" : "Endpoint could not be loaded"} />
        <div className="p-4">
          {forbidden ? (
            <>
              <PermissionDeniedState message="Viewing this endpoint needs the infrastructure read scope." />
              <Btn className="mt-4" onClick={() => router.push("/admin/endpoints")}>Back to endpoints</Btn>
            </>
          ) : (
            <>
              <AdminErrorState
                message={missing
                  ? "This endpoint is not in the inventory — it may have been deleted."
                  : `Loading this endpoint failed (${epQuery.error instanceof Error ? epQuery.error.message : "request error"}). That is not the same as the endpoint being missing.`}
                retry={() => void epQuery.refetch()}
              />
              <Btn className="mt-4" onClick={() => router.push("/admin/endpoints")}>Back to endpoints</Btn>
            </>
          )}
        </div>
      </AdminPageLayout>
    );
  }

  const ep = epQuery.data!;
  const diag = diagQuery.data;
  const inv = invQuery.data;

  return (
    <AdminPageLayout>
      <SectionHeader
        backAction={() => router.push("/admin/endpoints")}
        backLabel="Endpoints"
        status={<FreshnessBadge state={sourceState(epQuery)} />}
        sub={`${ep.endpointType} · ${ep.connectionMode}`}
        title={ep.name}
        action={<><Btn onClick={refresh}><RefreshCw size={14} /> Refresh</Btn></>}
      />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card><CardHeader title="Status" icon={Activity} />
          <div className="p-4">
            <StatusPill value={ep.status} />
            <div className="mt-2 text-xs text-text-subtle">
              {ep.reachable === true ? "Reachable" : ep.reachable === false ? "Unreachable" : "Reachability not probed"}
            </div>
            <div className="mt-1 text-xs text-text-subtle">{ep.version ? `v${ep.version}` : "Version not reported"}</div>
          </div>
        </Card>

        <Card><CardHeader title="Type" icon={Box} />
          <div className="p-4">
            <div className="text-sm font-medium capitalize text-text">{ep.endpointType}</div>
            <div className="mt-1 text-xs capitalize text-text-subtle">{ep.connectionMode} connection</div>
            <div className="mt-1 text-xs text-text-subtle">{ep.url ? <span className="break-all font-mono">{ep.url}</span> : "No URL recorded"}</div>
          </div>
        </Card>

        <Card><CardHeader title="Nodes" icon={Server} />
          <div className="p-4">
            <div className="text-2xl font-semibold text-text">
              {nodesQuery.isPending ? "—" : nodesQuery.isError ? "Unavailable" : nodes.length}
            </div>
            <div className="mt-1 text-xs text-text-subtle">
              {nodesQuery.isError ? "attached nodes — count unavailable" : "attached nodes"}
            </div>
          </div>
        </Card>

        <Card><CardHeader title="Containers" icon={Container} />
          <div className="p-4">
            <div className="text-2xl font-semibold text-text">{countOrUnknown(inv?.totalContainers ?? ep.totalContainers)}</div>
            <div className="mt-1 text-xs text-text-subtle">containers reported running</div>
          </div>
        </Card>
      </div>

      {/* Inventory. `{inv && …}` used to render nothing at all when the request
          failed, so an outage was indistinguishable from no inventory. */}
      <Card>
        <CardHeader title="Inventory Summary" icon={Database} />
        {invQuery.isPending ? <div className="p-4"><AdminLoadingState label="Loading inventory…" /></div>
          : invQuery.isError ? <div className="p-4"><AdminErrorState message={invQuery.error instanceof Error ? `Inventory could not be loaded: ${invQuery.error.message}` : "Inventory could not be loaded"} retry={() => void invQuery.refetch()} /></div>
          : (
            <div className="grid grid-cols-2 gap-4 p-4 sm:grid-cols-4">
              <div>
                <div className="text-sm text-text-subtle">Servers</div>
                <div className="text-xl font-semibold text-text">{countOrUnknown(inv?.totalServers)}</div>
              </div>
              <div>
                <div className="text-sm text-text-subtle">Memory Used</div>
                <div className="text-xl font-semibold text-text">{formatMB(inv?.usedMemoryMb)}</div>
                <div className="text-xs text-text-subtle">of {formatMB(inv?.totalMemoryMb)}</div>
              </div>
              <div>
                <div className="text-sm text-text-subtle">Disk Used</div>
                <div className="text-xl font-semibold text-text">{formatMB(inv?.usedDiskMb)}</div>
                <div className="text-xs text-text-subtle">of {formatMB(inv?.totalDiskMb)}</div>
              </div>
              <div>
                <div className="text-sm text-text-subtle">Containers / Images / Volumes</div>
                <div className="text-xl font-semibold text-text">
                  {countOrUnknown(inv?.totalContainers)} / {countOrUnknown(inv?.totalImages)} / {countOrUnknown(inv?.totalVolumes)}
                </div>
              </div>
            </div>
          )}
      </Card>

      <Card>
        <CardHeader title="Diagnostics" icon={Cpu} />
        {diagQuery.isPending ? <div className="p-4"><AdminLoadingState label="Running diagnostics…" /></div>
          : diagQuery.isError ? <div className="p-4"><AdminErrorState message={diagQuery.error instanceof Error ? `Diagnostics failed: ${diagQuery.error.message}` : "Diagnostics failed"} retry={() => void diagQuery.refetch()} /></div>
          : diag ? (
            <div className="p-4">
              <div className="mb-3 flex flex-wrap items-center gap-3 text-sm">
                <Pill tone={diag.reachable === true ? "green" : diag.reachable === false ? "red" : "unknown"}>
                  {diag.reachable === true ? "Reachable" : diag.reachable === false ? "Unreachable" : "Not probed"}
                </Pill>
                <span className="text-text-subtle">{diag.version ? `v${diag.version}` : "Version not reported"}</span>
                <span className="text-text-subtle">Checked: {formatDate(diag.checkedAt, "Time not reported")}</span>
              </div>

              {Array.isArray(diag.nodes) && diag.nodes.length > 0 ? (
                <AdminTable label="Nodes seen by diagnostics">
                  <AdminTHead><AdminTh>Node</AdminTh><AdminTh>Status</AdminTh><AdminTh>Servers</AdminTh><AdminTh>Allocated Mem</AdminTh><AdminTh>Allocated CPU</AdminTh><AdminTh>Allocated Disk</AdminTh></AdminTHead>
                  <AdminTBody>
                    {diag.nodes.map((n: { nodeId: string; name: string; status: string; serverCount: number; allocatedMemMb: number; allocatedCpu: number; allocatedDiskMb: number }) => (
                      <AdminTr key={n.nodeId}>
                        <AdminTd className="font-medium text-text">{n.name}</AdminTd>
                        <AdminTd><StatusPill value={n.status} /></AdminTd>
                        <AdminTd>{countOrUnknown(n.serverCount)}</AdminTd>
                        <AdminTd>{formatMB(n.allocatedMemMb)}</AdminTd>
                        <AdminTd>{countOrUnknown(n.allocatedCpu)} {n.allocatedCpu != null ? "cores" : ""}</AdminTd>
                        <AdminTd>{formatMB(n.allocatedDiskMb)}</AdminTd>
                      </AdminTr>
                    ))}
                  </AdminTBody>
                </AdminTable>
              ) : (
                <p className="text-xs text-text-subtle">Diagnostics returned no nodes for this endpoint.</p>
              )}
            </div>
          ) : <div className="p-4"><EmptyState icon={Cpu} title="No diagnostics returned" message="The endpoint answered the diagnostics call with an empty result." /></div>}
      </Card>

      <Card>
        <CardHeader title="Attached Nodes" icon={Server} />
        {nodesQuery.isPending ? <div className="p-4"><AdminLoadingState label="Loading attached nodes…" /></div>
          : nodesQuery.isError ? <div className="p-4"><AdminErrorState message={nodesQuery.error instanceof Error ? `Attached nodes could not be loaded: ${nodesQuery.error.message}` : "Attached nodes could not be loaded"} retry={() => void nodesQuery.refetch()} /></div>
          : nodes.length === 0 ? <EmptyState icon={Server} title="No attached nodes" message="No node reports through this endpoint." />
          : (
            <AdminTable label="Nodes attached to this endpoint">
              <AdminTHead><AdminTh>Node</AdminTh><AdminTh>Status</AdminTh></AdminTHead>
              <AdminTBody>
                {nodes.map((n) => (
                  <AdminTr key={n.id}>
                    <AdminTd className="font-medium text-text">
                      <Link className="underline hover:text-text-strong" href={`/admin/nodes/${n.nodeId ?? n.id}`}>{n.nodeName}</Link>
                    </AdminTd>
                    <AdminTd><StatusPill value={n.nodeStatus} /></AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
      </Card>

      <Card>
        <CardHeader title="Access Policies" icon={Network} />
        {policiesQuery.isPending ? <div className="p-4"><AdminLoadingState label="Loading access policies…" /></div>
          : policiesQuery.isError ? <div className="p-4"><AdminErrorState message={policiesQuery.error instanceof Error ? `Access policies could not be loaded: ${policiesQuery.error.message}` : "Access policies could not be loaded"} retry={() => void policiesQuery.refetch()} /></div>
          : policies.length === 0 ? <EmptyState icon={Network} title="No access policies" message="No principal is granted access to this endpoint." />
          : (
            <AdminTable label="Endpoint access policies">
              <AdminTHead><AdminTh>Principal Type</AdminTh><AdminTh>Principal</AdminTh><AdminTh>Role</AdminTh></AdminTHead>
              <AdminTBody>
                {policies.map((p) => (
                  <AdminTr key={p.id}>
                    <AdminTd className="text-text-subtle">{p.principalType || "Not reported"}</AdminTd>
                    <AdminTd className="break-all font-mono text-xs text-text-subtle">{p.principalId || "No principal recorded"}</AdminTd>
                    <AdminTd><Pill tone={p.role === "owner" ? "green" : p.role === "admin" ? "blue" : "neutral"}>{p.role || "Not reported"}</Pill></AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
      </Card>

      <Card>
        <CardHeader title="Health History" icon={Activity} />
        {healthQuery.isPending ? <div className="p-4"><AdminLoadingState label="Loading health history…" /></div>
          : healthQuery.isError ? <div className="p-4"><AdminErrorState message={healthQuery.error instanceof Error ? `Health history could not be loaded: ${healthQuery.error.message}` : "Health history could not be loaded"} retry={() => void healthQuery.refetch()} /></div>
          : health.length === 0 ? <EmptyState icon={Activity} title="No health records" message="This endpoint has not reported a health check yet." />
          : (
            <AdminTable label="Health records, newest first">
              <AdminTHead><AdminTh>Time</AdminTh><AdminTh>Status</AdminTh><AdminTh>Score</AdminTh><AdminTh>Containers</AdminTh><AdminTh>Error</AdminTh></AdminTHead>
              <AdminTBody>
                {health.map((r) => (
                  <AdminTr key={r.id}>
                    <AdminTd className="text-xs text-text-subtle">{formatDate(r.observedAt, "Time not reported")}</AdminTd>
                    <AdminTd><StatusPill value={r.status} /></AdminTd>
                    <AdminTd>{percentOrUnknown(r.healthScore)}</AdminTd>
                    <AdminTd>{countOrUnknown(r.containers)}</AdminTd>
                    <AdminTd className="text-xs text-danger">{r.error || "None reported"}</AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
      </Card>
    </AdminPageLayout>
  );
}
