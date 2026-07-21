"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity, AlertCircle, Box, Container, Cpu, Database, Network, RefreshCw, Server,
} from "lucide-react";
import {
  ApiError, fetchEndpoint, fetchEndpointDiagnostics, fetchEndpointInventory,
  fetchEndpointNodes, fetchEndpointHealthHistory, fetchEndpointAccessPolicies,
} from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, PermissionDeniedState, Pill, SectionHeader } from "./admin-ui";

const STATUS_COLORS: Record<string, string> = {
  online: "border-emerald-500/30 bg-emerald-900/30 text-emerald-300",
  degraded: "border-amber-500/30 bg-amber-900/30 text-amber-300",
  offline: "border-red-500/30 bg-red-900/30 text-red-300",
  unknown: "border-slate-500/30 bg-slate-900/30 text-slate-300",
  provisioning: "border-cyan-500/30 bg-cyan-900/30 text-cyan-300",
};

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

  if (epQuery.isLoading) return <div className="py-10 text-center text-sm text-slate-500">Loading</div>;
  if (epQuery.isError) {
    if (epQuery.error instanceof ApiError && epQuery.error.status === 403) {
      return (
        <div className="p-4">
          <PermissionDeniedState />
          <Btn className="mt-4" onClick={() => router.push("/admin/endpoints")}>Back to endpoints</Btn>
        </div>
      );
    }
    return (
      <div className="p-4">
        <div className="flex items-center gap-2 rounded-md border border-red-500/20 bg-red-500/5 p-3 text-sm text-red-400">
          <AlertCircle size={16} /> Endpoint not found
        </div>
        <Btn className="mt-4" onClick={() => router.push("/admin/endpoints")}>Back to endpoints</Btn>
      </div>
    );
  }

  const ep = epQuery.data!;
  const diag = diagQuery.data;
  const inv = invQuery.data;
  const nodes = nodesQuery.data ?? [];
  const health = healthQuery.data ?? [];
  const policies = policiesQuery.data ?? [];

  return (
    <div className="space-y-6">
      <SectionHeader
        title={ep.name}
        sub={ep.description || `${ep.endpointType} · ${ep.connectionMode}`}
        action={<><Btn onClick={refresh}><RefreshCw size={14} /> Refresh</Btn><Btn onClick={() => router.push("/admin/endpoints")}>Back</Btn></>}
      />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card><CardHeader title="Status" icon={Activity} />
          <div className="p-4">
            <Pill className={STATUS_COLORS[ep.status] ?? ""}>{ep.status}</Pill>
            <div className="mt-2 text-xs text-slate-500">
              {ep.reachable ? "Reachable" : "Unreachable"}
            </div>
            {ep.version && <div className="mt-1 text-xs text-slate-500">v{ep.version}</div>}
          </div>
        </Card>

        <Card><CardHeader title="Type" icon={Box} />
          <div className="p-4">
            <div className="text-sm font-medium text-slate-200 capitalize">{ep.endpointType}</div>
            <div className="mt-1 text-xs text-slate-500 capitalize">{ep.connectionMode} connection</div>
            {ep.edgeId && <div className="mt-1 text-xs text-slate-500">Edge ID: {ep.edgeId}</div>}
          </div>
        </Card>

        <Card><CardHeader title="Nodes" icon={Server} />
          <div className="p-4">
            <div className="text-2xl font-semibold text-slate-100">{nodes.length}</div>
            <div className="mt-1 text-xs text-slate-500">attached nodes</div>
          </div>
        </Card>

        <Card><CardHeader title="Containers" icon={Container} />
          <div className="p-4">
            <div className="text-2xl font-semibold text-slate-100">{inv?.totalServers ?? "-"}</div>
            <div className="mt-1 text-xs text-slate-500">servers running</div>
          </div>
        </Card>
      </div>

      {/* Inventory Summary */}
      {inv && (
        <Card>
          <CardHeader title="Inventory Summary" icon={Database} />
          <div className="grid grid-cols-2 gap-4 p-4 sm:grid-cols-4">
            <div>
              <div className="text-sm text-slate-400">Servers</div>
              <div className="text-xl font-semibold text-slate-100">{inv.totalServers}</div>
            </div>
            <div>
              <div className="text-sm text-slate-400">Memory Used</div>
              <div className="text-xl font-semibold text-slate-100">{formatMB(inv.usedMemoryMb)}</div>
              <div className="text-xs text-slate-500">of {formatMB(inv.totalMemoryMb)}</div>
            </div>
            <div>
              <div className="text-sm text-slate-400">Disk Used</div>
              <div className="text-xl font-semibold text-slate-100">{formatMB(inv.usedDiskMb)}</div>
              <div className="text-xs text-slate-500">of {formatMB(inv.totalDiskMb)}</div>
            </div>
            <div>
              <div className="text-sm text-slate-400">Containers / Images / Volumes</div>
              <div className="text-xl font-semibold text-slate-100">
                {inv.totalContainers ?? "-"} / {inv.totalImages ?? "-"} / {inv.totalVolumes ?? "-"}
              </div>
            </div>
          </div>
        </Card>
      )}

      {/* Diagnostics */}
      {diag && (
        <Card>
          <CardHeader title="Diagnostics" icon={Cpu} />
          <div className="p-4">
            <div className="mb-3 flex items-center gap-3 text-sm">
              <span className={diag.reachable ? "text-emerald-400" : "text-red-400"}>
                {diag.reachable ? "Reachable" : "Unreachable"}
              </span>
              {diag.version && <span className="text-slate-500">v{diag.version}</span>}
              <span className="text-slate-500">Checked: {new Date(diag.checkedAt).toLocaleString()}</span>
            </div>

            {diag.nodes.length > 0 && (
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-slate-700/50 text-left text-xs uppercase tracking-wider text-slate-500">
                    <th className="px-3 py-2 font-medium">Node</th>
                    <th className="px-3 py-2 font-medium">Status</th>
                    <th className="px-3 py-2 font-medium">Servers</th>
                    <th className="px-3 py-2 font-medium">Allocated Mem</th>
                    <th className="px-3 py-2 font-medium">Allocated CPU</th>
                    <th className="px-3 py-2 font-medium">Allocated Disk</th>
                  </tr>
                </thead>
                <tbody>
                  {diag.nodes.map((n: { nodeId: string; name: string; status: string; serverCount: number; allocatedMemMb: number; allocatedCpu: number; allocatedDiskMb: number }) => (
                    <tr key={n.nodeId} className="border-b border-slate-800/50">
                      <td className="px-3 py-2 font-medium text-slate-200">{n.name}</td>
                      <td className="px-3 py-2"><Pill className={STATUS_COLORS[n.status] ?? ""}>{n.status}</Pill></td>
                      <td className="px-3 py-2 text-slate-300">{n.serverCount}</td>
                      <td className="px-3 py-2 text-slate-300">{formatMB(n.allocatedMemMb)}</td>
                      <td className="px-3 py-2 text-slate-300">{n.allocatedCpu}</td>
                      <td className="px-3 py-2 text-slate-300">{formatMB(n.allocatedDiskMb)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </Card>
      )}

      {/* Attached Nodes */}
      <Card>
        <CardHeader title="Attached Nodes" icon={Server} />
        {nodes.length === 0 ? (
          <EmptyState icon={Server} message="No nodes attached to this endpoint." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-700/50 text-left text-xs uppercase tracking-wider text-slate-500">
                <th className="px-4 py-3 font-medium">Node</th>
                <th className="px-4 py-3 font-medium">Status</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map((n) => (
                <tr key={n.id} className="border-b border-slate-800/50">
                  <td className="px-4 py-3 font-medium text-slate-200">{n.nodeName}</td>
                  <td className="px-4 py-3"><Pill className={STATUS_COLORS[n.nodeStatus] ?? ""}>{n.nodeStatus}</Pill></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {/* Access Policies */}
      <Card>
        <CardHeader title="Access Policies" icon={Network} />
        {policies.length === 0 ? (
          <EmptyState icon={Network} message="No access policies configured." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-700/50 text-left text-xs uppercase tracking-wider text-slate-500">
                <th className="px-4 py-3 font-medium">Principal Type</th>
                <th className="px-4 py-3 font-medium">Principal ID</th>
                <th className="px-4 py-3 font-medium">Role</th>
              </tr>
            </thead>
            <tbody>
              {policies.map((p) => (
                <tr key={p.id} className="border-b border-slate-800/50">
                  <td className="px-4 py-3 text-slate-300">{p.principalType}</td>
                  <td className="px-4 py-3 font-mono text-xs text-slate-300">{p.principalId}</td>
                  <td className="px-4 py-3"><Pill tone={p.role === "owner" ? "green" : p.role === "admin" ? "blue" : "neutral"}>{p.role}</Pill></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {/* Health History */}
      <Card>
        <CardHeader title="Health History" icon={Activity} />
        {health.length === 0 ? (
          <EmptyState icon={Activity} message="No health records yet." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-700/50 text-left text-xs uppercase tracking-wider text-slate-500">
                <th className="px-4 py-3 font-medium">Time</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Score</th>
                <th className="px-4 py-3 font-medium">Containers</th>
                <th className="px-4 py-3 font-medium">Error</th>
              </tr>
            </thead>
            <tbody>
              {health.map((r) => (
                <tr key={r.id} className="border-b border-slate-800/50">
                  <td className="px-4 py-3 text-xs text-slate-400">{new Date(r.observedAt).toLocaleString()}</td>
                  <td className="px-4 py-3"><Pill className={STATUS_COLORS[r.status] ?? ""}>{r.status}</Pill></td>
                  <td className="px-4 py-3 text-slate-300">{(r.healthScore * 100).toFixed(0)}%</td>
                  <td className="px-4 py-3 text-slate-300">{r.containers}</td>
                  <td className="px-4 py-3 text-xs text-red-400">{r.error || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  );
}

function formatMB(mb: number): string {
  if (mb >= 1024) return (mb / 1024).toFixed(1) + " GB";
  return mb + " MB";
}
