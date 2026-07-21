"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, Plus, RotateCcw, Archive, Eye } from "lucide-react";
import { Btn, Card, CardHeader, EmptyState, Pill } from "@/components/admin/admin-ui";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";
import { useToast } from "@/components/ui/toast";
import { useParams } from "next/navigation";
import {
  type DatabaseService,
  type DatabaseServiceBackup,
  listServerDatabaseServices,
  createServiceBackup,
  listServiceCredentials,
  revokeServiceCredential,
  linkDatabaseServiceToServer,
  unlinkDatabaseServiceFromServer,
  listDatabaseServices,
} from "@/lib/api/database-services";

const statusTone: Record<string, "green" | "red" | "yellow" | "neutral"> = {
  running: "green",
  stopped: "red",
  failed: "red",
  provisioning: "yellow",
  deleting: "yellow",
};

const engineLabels: Record<string, string> = {
  postgresql: "PostgreSQL",
  mysql: "MySQL",
  mariadb: "MariaDB",
  redis: "Redis",
  mongodb: "MongoDB",
};

export default function ServerDatabasePage() {
  return (
    <ServerConsoleLayout activeTab="databases">
      {(server) => <ServerDatabaseView serverId={server.id} />}
    </ServerConsoleLayout>
  );
}

function ServerDatabaseView({ serverId }: { serverId: string }) {
  const qc = useQueryClient();
  const { toast } = useToast();

  const svcsQuery = useQuery({
    queryKey: ["server-database-services", serverId],
    queryFn: () => listServerDatabaseServices(serverId),
  });
  const svcs = svcsQuery.data ?? [];

  const allSvcsQuery = useQuery({
    queryKey: ["all-database-services"],
    queryFn: listDatabaseServices,
    enabled: false,
  });

  const [showLink, setShowLink] = useState(false);
  const [showDetail, setShowDetail] = useState<string | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["server-database-services", serverId] });
    qc.invalidateQueries({ queryKey: ["database-services"] });
  };

  const backupMut = useMutation({
    mutationFn: (svcId: string) => createServiceBackup(svcId),
    onSuccess: () => toast({ tone: "success", title: "Backup created" }),
    onError: (e: Error) => toast({ tone: "error", title: "Backup failed", message: e.message }),
  });

  const unlinkMut = useMutation({
    mutationFn: (svcId: string) => unlinkDatabaseServiceFromServer(serverId, svcId),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Service unlinked" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Unlink failed", message: e.message }),
  });

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-100">Linked Databases</h1>
          <p className="mt-1 text-sm text-slate-400">
            Managed database services linked to this server
          </p>
        </div>
        <Btn onClick={() => { allSvcsQuery.refetch(); setShowLink(true); }}>
          <Plus size={14} /> Link Service
        </Btn>
      </div>

      <Card>
        <CardHeader title="Database Services" icon={Database} />
        {svcsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading</div>
        ) : svcs.length === 0 ? (
          <EmptyState icon={Database} message="No database services linked to this server." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">Type</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Memory</th>
                <th className="px-4 py-3">Connection</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {svcs.map((svc) => (
                <tr key={svc.id} className="hover:bg-white/[0.02]">
                  <td className="px-4 py-3 font-medium text-slate-200">{svc.name || svc.id.slice(0, 8)}</td>
                  <td className="px-4 py-3"><Pill tone="blue">{engineLabels[svc.type] || svc.type} {svc.version}</Pill></td>
                  <td className="px-4 py-3"><Pill tone={statusTone[svc.status] || "neutral"}>{svc.status}</Pill></td>
                  <td className="px-4 py-3 text-xs text-slate-400">{svc.memoryMb}MB</td>
                  <td className="px-4 py-3">
                    {svc.connectionString ? (
                      <Btn size="sm" tone="ghost" onClick={() => setShowDetail(svc.id)}><Eye size={13} /> View</Btn>
                    ) : (
                      <span className="text-xs text-slate-500">-</span>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-1">
                      <button
                        className="grid h-8 w-8 place-items-center rounded text-slate-400 hover:bg-white/[0.06] hover:text-amber-200 disabled:opacity-40"
                        disabled={backupMut.isPending}
                        onClick={() => backupMut.mutate(svc.id)}
                        title="Backup"
                        type="button"
                      >
                        <Archive size={14} />
                      </button>
                      <button
                        className="grid h-8 w-8 place-items-center rounded text-slate-400 hover:bg-white/[0.06] hover:text-red-200 disabled:opacity-40"
                        disabled={unlinkMut.isPending}
                        onClick={() => { if (window.confirm("Unlink this database service?")) unlinkMut.mutate(svc.id); }}
                        title="Unlink"
                        type="button"
                      >
                        <RotateCcw size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {showLink && (
        <LinkServiceModal
          serverId={serverId}
          linkedIds={svcs.map((s) => s.id)}
          onClose={() => setShowLink(false)}
          onLinked={invalidate}
        />
      )}

      {showDetail && (
        <ConnectionDetailModal
          serviceId={showDetail}
          onClose={() => setShowDetail(null)}
        />
      )}
    </div>
  );
}

function LinkServiceModal({ serverId, linkedIds, onClose, onLinked }: {
  serverId: string;
  linkedIds: string[];
  onClose: () => void;
  onLinked: () => void;
}) {
  const { toast } = useToast();
  const [selectedId, setSelectedId] = useState("");

  const availableQuery = useQuery({
    queryKey: ["all-database-services"],
    queryFn: listDatabaseServices,
  });

  const available = (availableQuery.data ?? []).filter((s) => !linkedIds.includes(s.id));

  const linkMut = useMutation({
    mutationFn: () => linkDatabaseServiceToServer(serverId, selectedId),
    onSuccess: () => { onLinked(); onClose(); toast({ tone: "success", title: "Service linked" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Link failed", message: e.message }),
  });

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div className="w-full max-w-md rounded-xl bg-[#1e2536] p-6 shadow-2xl" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-4 text-lg font-semibold text-slate-100">Link Database Service</h2>
        {available.length === 0 ? (
          <p className="text-sm text-slate-400">No unlinked database services available.</p>
        ) : (
          <div className="space-y-2">
            {available.map((s) => (
              <label key={s.id} className={`flex cursor-pointer items-center gap-3 rounded-lg border p-3 text-sm transition-colors ${selectedId === s.id ? "border-blue-500/50 bg-blue-900/20" : "border-white/10 bg-[#161b28] hover:bg-white/[0.04]"}`}>
                <input
                  type="radio"
                  name="service"
                  value={s.id}
                  checked={selectedId === s.id}
                  onChange={() => setSelectedId(s.id)}
                  className="accent-blue-500"
                />
                <div>
                  <span className="text-slate-200">{s.name || s.id.slice(0, 8)}</span>
                  <span className="ml-2 text-xs text-slate-500">{engineLabels[s.type] || s.type} {s.version}</span>
                </div>
              </label>
            ))}
          </div>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <Btn size="sm" tone="ghost" onClick={onClose}>Cancel</Btn>
          <Btn size="sm" disabled={!selectedId || linkMut.isPending} onClick={() => linkMut.mutate()}>
            {linkMut.isPending ? "Linking..." : "Link"}
          </Btn>
        </div>
      </div>
    </div>
  );
}

function ConnectionDetailModal({ serviceId, onClose }: { serviceId: string; onClose: () => void }) {
  const svcQuery = useQuery({
    queryKey: ["database-service", serviceId],
    queryFn: async () => {
      const { getDatabaseService } = await import("@/lib/api/database-services");
      return getDatabaseService(serviceId);
    },
  });

  const svc = svcQuery.data;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div className="w-full max-w-lg rounded-xl bg-[#1e2536] p-6 shadow-2xl" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-4 text-lg font-semibold text-slate-100">Connection Details</h2>
        {!svc ? (
          <p className="text-sm text-slate-400">Loading...</p>
        ) : (
          <div className="space-y-3 text-sm">
            <div>
              <span className="text-slate-500">Connection String:</span>
              <pre className="mt-1 rounded bg-black/40 p-3 font-mono text-xs text-emerald-300 break-all select-all">{svc.connectionString}</pre>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div><span className="text-slate-500">Host:</span><br /><span className="font-mono text-xs text-slate-200">{svc.host}:{svc.port}</span></div>
              <div><span className="text-slate-500">Database:</span><br /><span className="font-mono text-xs text-slate-200">{svc.databaseName}</span></div>
              <div><span className="text-slate-500">Username:</span><br /><span className="font-mono text-xs text-slate-200">{svc.username}</span></div>
              <div><span className="text-slate-500">Engine:</span><br /><span className="text-slate-200">{engineLabels[svc.type] || svc.type} {svc.version}</span></div>
            </div>
          </div>
        )}
        <div className="mt-4 flex justify-end">
          <Btn size="sm" tone="ghost" onClick={onClose}>Close</Btn>
        </div>
      </div>
    </div>
  );
}
