"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Shield, ShieldCheck, ShieldX, RefreshCw, Trash2, Plus, AlertTriangle, CheckCircle, XCircle } from "lucide-react";
import { fetchJSON, postJSON } from "@/lib/api";
import { Card, CardHeader, EmptyState, StatsRow, Pill } from "@/components/admin/admin-ui";
import { useState } from "react";

type MTLSCert = {
  id: string;
  certType: "ca" | "server" | "client";
  commonName: string;
  organization: string;
  serialNumber: string;
  expiresAt: string;
  revokedAt: string | null;
  nodeId: string | null;
  createdAt: string;
};

type MTLSCertListResponse = { data: MTLSCert[] };
type MTLSCertResponse = { data: MTLSCert };
type MTLSStatusResponse = { data: { caConfigured: boolean; serverCertCount: number; clientCertCount: number; activeCerts: number; revokedCount: number; caExpiresAt?: string } };
type MTLSMigrationStatusResponse = { data: { caConfigured: boolean; nodesWithCerts: number; totalNodes: number; migrationEnabled: boolean; phase: string } };

export default function AdminMTLSPage() {
  const queryClient = useQueryClient();
  const [showGenerateCA, setShowGenerateCA] = useState(false);
  const [caOrg, setCAOrg] = useState("GamePanel");
  const [caCN, setCACN] = useState("GamePanel mTLS CA");

  const { data: statusData, isLoading: statusLoading, error: statusError } = useQuery({
    queryKey: ["mtls-status"],
    queryFn: () => fetchJSON<MTLSStatusResponse>("/mtls/status"),
    refetchInterval: 10000,
  });

  const { data: migrationData, isLoading: migrationLoading } = useQuery({
    queryKey: ["mtls-migration-status"],
    queryFn: () => fetchJSON<MTLSMigrationStatusResponse>("/mtls/migration/status"),
    refetchInterval: 15000,
  });

  const { data: certsData, isLoading: certsLoading } = useQuery({
    queryKey: ["mtls-certs"],
    queryFn: () => fetchJSON<MTLSCertListResponse>("/mtls/certificates?limit=100"),
  });

  const { data: caCertsData } = useQuery({
    queryKey: ["mtls-certs-ca"],
    queryFn: () => fetchJSON<MTLSCertListResponse>("/mtls/certificates?type=ca"),
  });

  const generateCAMutation = useMutation({
    mutationFn: () => postJSON<MTLSCertResponse>("/mtls/certificates/generate-ca", { organization: caOrg, commonName: caCN }),
    onSuccess: () => {
      setShowGenerateCA(false);
      queryClient.invalidateQueries({ queryKey: ["mtls-status"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs-ca"] });
    },
  });

  const revokeMutation = useMutation({
    mutationFn: (id: string) => postJSON<{ ok: boolean }>(`/mtls/certificates/${id}/revoke`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mtls-status"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs-ca"] });
    },
  });

  const migrationMutation = useMutation({
    mutationFn: () => postJSON<{ ok: boolean }>("/mtls/migration/run"),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mtls-migration-status"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-status"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs"] });
    },
  });

  const status = statusData?.data;
  const migration = migrationData?.data;
  const certs = certsData?.data || [];
  const caCerts = caCertsData?.data || [];

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-100">mTLS Certificate Management</h1>
          <p className="text-sm text-slate-400 mt-1">
            Mutual TLS authentication between the control plane and node agents
          </p>
        </div>
        <button
          onClick={() => setShowGenerateCA(true)}
          className="flex items-center gap-2 rounded-lg bg-[#dc2626] px-4 py-2 text-sm font-bold text-white hover:bg-[#b91c1c] transition"
          type="button"
        >
          <Plus className="h-4 w-4" />
          Generate CA
        </button>
      </div>

      {showGenerateCA && (
        <Card>
          <CardHeader title="Generate New CA Certificate" />
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1">Organization</label>
              <input
                className="w-full rounded-lg border border-white/[0.08] bg-white/[0.04] px-3 py-2 text-sm text-slate-200"
                value={caOrg}
                onChange={(e) => setCAOrg(e.target.value)}
                placeholder="GamePanel"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1">Common Name</label>
              <input
                className="w-full rounded-lg border border-white/[0.08] bg-white/[0.04] px-3 py-2 text-sm text-slate-200"
                value={caCN}
                onChange={(e) => setCACN(e.target.value)}
                placeholder="GamePanel mTLS CA"
              />
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => generateCAMutation.mutate()}
                disabled={generateCAMutation.isPending}
                className="rounded-lg bg-[#dc2626] px-4 py-2 text-sm font-bold text-white hover:bg-[#b91c1c] disabled:opacity-60 transition"
                type="button"
              >
                {generateCAMutation.isPending ? "Generating..." : "Generate"}
              </button>
              <button
                onClick={() => setShowGenerateCA(false)}
                className="rounded-lg border border-white/[0.08] px-4 py-2 text-sm text-slate-300 hover:bg-white/[0.04] transition"
                type="button"
              >
                Cancel
              </button>
            </div>
            {generateCAMutation.isError && (
              <p className="text-sm text-red-400">Error: {(generateCAMutation.error as Error)?.message}</p>
            )}
          </div>
        </Card>
      )}

      {statusLoading ? (
        <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-6 text-center text-sm text-slate-400">Loading status...</div>
      ) : statusError ? (
        <div className="rounded-lg border border-red-700/30 bg-red-900/10 p-6 text-center text-sm text-red-300">Failed to load mTLS status</div>
      ) : status ? (
        <StatsRow
          items={[
            { label: "CA Configured", value: status.caConfigured ? "Yes" : "No", icon: status.caConfigured ? ShieldCheck : ShieldX, tone: status.caConfigured ? "green" : "red" },
            { label: "Server Certs", value: status.serverCertCount ?? 0, icon: Shield },
            { label: "Client Certs", value: status.clientCertCount ?? 0, icon: Shield },
            { label: "Active Certs", value: status.activeCerts ?? 0, icon: CheckCircle, tone: "green" },
            { label: "Revoked", value: status.revokedCount ?? 0, icon: XCircle, tone: "red" },
          ]}
        />
      ) : null}

      {status?.caExpiresAt && (
        <Card>
          <CardHeader title="CA Certificate Expiry" />
          <div className="flex items-center gap-3">
            <AlertTriangle className="h-5 w-5 text-amber-400" />
            <span className="text-sm text-slate-300">
              CA expires at {new Date(status.caExpiresAt).toLocaleDateString()} ({Math.round((new Date(status.caExpiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24))} days)
            </span>
          </div>
        </Card>
      )}

      {caCerts.length > 0 && (
        <Card>
          <CardHeader title="CA Certificates" />
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500">
                <th className="pb-2 font-medium">Common Name</th>
                <th className="pb-2 font-medium">Organization</th>
                <th className="pb-2 font-medium">Serial</th>
                <th className="pb-2 font-medium">Expires</th>
                <th className="pb-2 font-medium">Status</th>
                <th className="pb-2 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {caCerts.map((cert) => (
                <tr key={cert.id} className="border-b border-white/[0.04] text-slate-300">
                  <td className="py-3">{cert.commonName}</td>
                  <td className="py-3">{cert.organization}</td>
                  <td className="py-3 font-mono text-xs">{cert.serialNumber?.slice(0, 16)}...</td>
                  <td className="py-3">{new Date(cert.expiresAt).toLocaleDateString()}</td>
                  <td className="py-3">
                    {cert.revokedAt ? (
                      <Pill tone="red">Revoked</Pill>
                    ) : new Date(cert.expiresAt) > new Date() ? (
                      <Pill tone="green">Active</Pill>
                    ) : (
                      <Pill tone="red">Expired</Pill>
                    )}
                  </td>
                  <td className="py-3">
                    <button
                      onClick={() => {
                        if (window.confirm("Revoke this CA certificate?")) {
                          revokeMutation.mutate(cert.id);
                        }
                      }}
                      disabled={!!cert.revokedAt}
                      className="text-red-400 hover:text-red-300 disabled:opacity-40 transition"
                      title="Revoke"
                      type="button"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}

      <Card>
        <CardHeader title={`Node Certificates (${certs.length})`} />
        {certs.length === 0 ? (
          <EmptyState icon={Shield} title="No certificates" message="Generate a CA and node certificates to enable mTLS." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500">
                <th className="pb-2 font-medium">Type</th>
                <th className="pb-2 font-medium">Common Name</th>
                <th className="pb-2 font-medium">Node</th>
                <th className="pb-2 font-medium">Expires</th>
                <th className="pb-2 font-medium">Status</th>
                <th className="pb-2 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {certs.map((cert) => (
                <tr key={cert.id} className="border-b border-white/[0.04] text-slate-300">
                  <td className="py-3">
                    <Pill tone={cert.certType === "ca" ? "yellow" : cert.certType === "server" ? "blue" : "neutral"}>
                      {cert.certType}
                    </Pill>
                  </td>
                  <td className="py-3">{cert.commonName}</td>
                  <td className="py-3 font-mono text-xs">{cert.nodeId ? cert.nodeId.slice(0, 8) : "-"}</td>
                  <td className="py-3">{new Date(cert.expiresAt).toLocaleDateString()}</td>
                  <td className="py-3">
                    {cert.revokedAt ? (
                      <Pill tone="red">Revoked</Pill>
                    ) : new Date(cert.expiresAt) > new Date() ? (
                      <Pill tone="green">Active</Pill>
                    ) : (
                      <Pill tone="red">Expired</Pill>
                    )}
                  </td>
                  <td className="py-3 flex gap-2">
                    {!cert.revokedAt && (
                      <button
                        onClick={() => {
                          if (window.confirm("Revoke this certificate?")) {
                            revokeMutation.mutate(cert.id);
                          }
                        }}
                        className="text-red-400 hover:text-red-300 transition"
                        title="Revoke"
                        type="button"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <Card>
        <CardHeader title="Token-to-mTLS Migration" />
        {migrationLoading ? (
          <div className="text-sm text-slate-400">Loading migration status...</div>
        ) : migration ? (
          <div className="space-y-4">
            <div className="grid grid-cols-3 gap-4">
              <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-4 text-center">
                <div className="text-2xl font-bold text-slate-200">{migration.totalNodes}</div>
                <div className="text-xs text-slate-400">Total Nodes</div>
              </div>
              <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-4 text-center">
                <div className="text-2xl font-bold text-green-400">{migration.nodesWithCerts}</div>
                <div className="text-xs text-slate-400">Nodes with Certs</div>
              </div>
              <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-4 text-center">
                <div className="text-2xl font-bold text-slate-200">
                  {migration.phase === "complete" ? "Complete" : migration.phase === "partial" ? "In Progress" : migration.phase === "ca_ready" ? "CA Ready" : "Not Started"}
                </div>
                <div className="text-xs text-slate-400">Phase</div>
              </div>
            </div>

            {migration.phase !== "complete" && (
              <button
                onClick={() => migrationMutation.mutate()}
                disabled={migrationMutation.isPending}
                className="flex items-center gap-2 rounded-lg bg-[#dc2626] px-4 py-2 text-sm font-bold text-white hover:bg-[#b91c1c] disabled:opacity-60 transition"
                type="button"
              >
                <RefreshCw className={`h-4 w-4 ${migrationMutation.isPending ? "animate-spin" : ""}`} />
                {migrationMutation.isPending ? "Migrating..." : "Run Migration"}
              </button>
            )}

            {migration.phase === "complete" && (
              <div className="flex items-center gap-2 text-sm text-green-400">
                <CheckCircle className="h-4 w-4" />
                All nodes have mTLS certificates. You can now enable mTLS enforcement.
              </div>
            )}
          </div>
        ) : (
          <EmptyState icon={RefreshCw} title="Migration not available" message="Migration service is not configured." />
        )}
      </Card>
    </div>
  );
}
