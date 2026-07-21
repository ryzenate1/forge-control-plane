"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Database, Plus, Trash2, RotateCcw, Archive, Eye, Link2, Unlink,
} from "lucide-react";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import {
  type DatabaseService,
  type DatabaseServiceBackup,
  type DatabaseServiceCredential,
  type ServiceTemplate,
  type ProvisionDBServiceRequest,
  listDatabaseServices,
  provisionDatabaseService,
  deleteDatabaseService,
  restartDatabaseService,
  listServiceBackups,
  createServiceBackup,
  restoreServiceBackup,
  listServiceCredentials,
  createServiceCredential,
  revokeServiceCredential,
  listServiceTemplates,
  createServiceTemplate,
  getServiceLogs,
  getDatabaseService,
  linkDatabaseServiceToServer,
  unlinkDatabaseServiceFromServer,
} from "@/lib/api/database-services";

type Tab = "services" | "templates";

const engineLabels: Record<string, string> = {
  postgresql: "PostgreSQL",
  mysql: "MySQL",
  mariadb: "MariaDB",
  redis: "Redis",
  mongodb: "MongoDB",
};

const statusTone: Record<string, "green" | "red" | "yellow" | "neutral"> = {
  running: "green",
  stopped: "red",
  failed: "red",
  provisioning: "yellow",
  deleting: "yellow",
};

export default function AdminDatabaseServicesPage() {
  const [activeTab, setActiveTab] = useState<Tab>("services");
  const [showProvision, setShowProvision] = useState(false);
  const [showTemplate, setShowTemplate] = useState(false);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [showCreds, setShowCreds] = useState<string | null>(null);

  return (
    <div>
      <div className="mb-6 flex items-center gap-1 rounded-lg bg-[#161b28] p-1 w-fit">
        {([
          { key: "services" as Tab, label: "Database Services" },
          { key: "templates" as Tab, label: "Service Templates" },
        ]).map((tab) => (
          <button
            key={tab.key}
            className={`rounded-md px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === tab.key
                ? "bg-[#1e2536] text-slate-100 shadow-sm"
                : "text-slate-400 hover:text-slate-200"
            }`}
            onClick={() => setActiveTab(tab.key)}
            type="button"
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === "services" ? (
        <ServicesTab
          onProvision={() => setShowProvision(true)}
          onDetail={(id) => setDetailId(id)}
          showProvision={showProvision}
          onCloseProvision={() => setShowProvision(false)}
          detailId={detailId}
          onCloseDetail={() => setDetailId(null)}
          showCreds={showCreds}
          onShowCreds={(id) => setShowCreds(id)}
          onCloseCreds={() => setShowCreds(null)}
        />
      ) : (
        <TemplatesTab
          showCreate={showTemplate}
          onShowCreate={() => setShowTemplate(true)}
          onClose={() => setShowTemplate(false)}
        />
      )}
    </div>
  );
}

function ServicesTab({
  onProvision, onDetail, showProvision, onCloseProvision,
  detailId, onCloseDetail, showCreds, onShowCreds, onCloseCreds,
}: {
  onProvision: () => void;
  onDetail: (id: string) => void;
  showProvision: boolean;
  onCloseProvision: () => void;
  detailId: string | null;
  onCloseDetail: () => void;
  showCreds: string | null;
  onShowCreds: (id: string) => void;
  onCloseCreds: () => void;
}) {
  const qc = useQueryClient();
  const { toast } = useToast();

  const servicesQuery = useQuery({
    queryKey: ["database-services"],
    queryFn: listDatabaseServices,
  });
  const services = servicesQuery.data ?? [];

  const invalidate = () => qc.invalidateQueries({ queryKey: ["database-services"] });

  const deleteMut = useMutation({
    mutationFn: deleteDatabaseService,
    onSuccess: () => { invalidate(); onCloseDetail(); toast({ tone: "success", title: "Service deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Deletion failed", message: e.message }),
  });

  const restartMut = useMutation({
    mutationFn: restartDatabaseService,
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Service restarted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Restart failed", message: e.message }),
  });

  return (
    <div>
      <SectionHeader
        title="Database Services"
        sub="Provisioned managed database containers"
        action={<Btn onClick={onProvision}><Plus size={14} /> Provision</Btn>}
      />

      <Card>
        <CardHeader title="Services" icon={Database} />
        {servicesQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading</div>
        ) : servicesQuery.isError ? (
          <div className="p-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load services: {servicesQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void servicesQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : services.length === 0 ? (
          <EmptyState icon={Database} message="No database services. Provision one to get started." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">Type</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Resources</th>
                <th className="px-4 py-3">Server</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {services.map((svc) => (
                <tr key={svc.id} className="hover:bg-white/[0.02]">
                  <td className="px-4 py-3 font-medium text-slate-200">{svc.name || svc.id.slice(0, 8)}</td>
                  <td className="px-4 py-3"><Pill tone="blue">{engineLabels[svc.type] || svc.type} {svc.version}</Pill></td>
                  <td className="px-4 py-3"><Pill tone={statusTone[svc.status] || "neutral"}>{svc.status}</Pill></td>
                  <td className="px-4 py-3 text-xs text-slate-400">{svc.memoryMb}MB</td>
                  <td className="px-4 py-3 text-xs text-slate-500">{svc.serverId ? svc.serverId.slice(0, 8) : "-"}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-1">
                      <Btn size="sm" tone="ghost" onClick={() => onDetail(svc.id)}><Eye size={13} /></Btn>
                      <Btn size="sm" tone="warning" onClick={() => restartMut.mutate(svc.id)} disabled={restartMut.isPending}><RotateCcw size={13} /></Btn>
                      <Btn size="sm" tone="danger" onClick={() => { if (window.confirm(`Delete service ${svc.name || svc.id.slice(0, 8)}?`)) deleteMut.mutate(svc.id); }} disabled={deleteMut.isPending}><Trash2 size={13} /></Btn>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {showProvision && <ProvisionModal onClose={onCloseProvision} onCreated={invalidate} />}
      {detailId && <DetailModal serviceId={detailId} onClose={onCloseDetail} onInvalidate={invalidate} />}
    </div>
  );
}

function ProvisionModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { toast } = useToast();
  const [name, setName] = useState("");
  const [type, setType] = useState("postgresql");
  const [version, setVersion] = useState("16");
  const [memoryMb, setMemoryMb] = useState("256");

  const templatesQuery = useQuery({
    queryKey: ["service-templates"],
    queryFn: listServiceTemplates,
  });

  const versionsForType = (templatesQuery.data ?? [])
    .filter((t) => t.type === type)
    .map((t) => t.version);

  const createMut = useMutation({
    mutationFn: () => provisionDatabaseService({
      name,
      type,
      version,
      memoryMb: memoryMb ? parseInt(memoryMb, 10) : 256,
    }),
    onSuccess: () => {
      onCreated();
      onClose();
      toast({ tone: "success", title: "Database service provisioning started" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Provisioning failed", message: e.message }),
  });

  return (
    <Modal title="Provision Database Service" onClose={onClose} wide>
      <div className="grid gap-4 md:grid-cols-2">
        <Input label="Name (optional)" value={name} onChange={setName} placeholder="my-database" />
        <div>
          <label className="block text-sm font-medium text-slate-300 mb-1.5">Type</label>
          <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100 text-sm" value={type} onChange={(e) => { setType(e.target.value); const vs = versionsForType; if (vs.length > 0) setVersion(vs[vs.length - 1]); }}>
            {Object.entries(engineLabels).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </select>
        </div>
        <div>
          <label className="block text-sm font-medium text-slate-300 mb-1.5">Version</label>
          <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100 text-sm" value={version} onChange={(e) => setVersion(e.target.value)}>
            {versionsForType.map((v) => <option key={v} value={v}>{v}</option>)}
          </select>
        </div>
        <Input label="Memory (MB)" value={memoryMb} onChange={setMemoryMb} type="number" placeholder="256" />
      </div>
      {createMut.isError && (
        <div className="mt-4 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
          <span>{createMut.error?.message || "An unexpected error occurred."}</span>
        </div>
      )}
      <ModalFooter
        onCancel={onClose}
        onConfirm={() => createMut.mutate()}
        disabled={createMut.isPending || !type || !version}
        confirmLabel={createMut.isPending ? "Provisioning..." : "Provision"}
      />
    </Modal>
  );
}

function DetailModal({ serviceId, onClose, onInvalidate }: { serviceId: string; onClose: () => void; onInvalidate: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();

  const svcQuery = useQuery({
    queryKey: ["database-service", serviceId],
    queryFn: () => getDatabaseService(serviceId),
  });
  const svc = svcQuery.data;

  const backupsQuery = useQuery({
    queryKey: ["service-backups", serviceId],
    queryFn: () => listServiceBackups(serviceId),
  });

  const credsQuery = useQuery({
    queryKey: ["service-credentials", serviceId],
    queryFn: () => listServiceCredentials(serviceId),
  });

  const logsQuery = useQuery({
    queryKey: ["service-logs", serviceId],
    queryFn: () => getServiceLogs(serviceId),
    enabled: false,
  });

  const backupMut = useMutation({
    mutationFn: () => createServiceBackup(serviceId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["service-backups", serviceId] }); toast({ tone: "success", title: "Backup created" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Backup failed", message: e.message }),
  });

  const restoreMut = useMutation({
    mutationFn: (backupId: string) => restoreServiceBackup(serviceId, backupId),
    onSuccess: () => toast({ tone: "success", title: "Backup restored" }),
    onError: (e: Error) => toast({ tone: "error", title: "Restore failed", message: e.message }),
  });

  const [showNewCred, setShowNewCred] = useState(false);
  const [newUser, setNewUser] = useState("");
  const [newPass, setNewPass] = useState("");
  const [newPerms, setNewPerms] = useState("read-write");

  const createCredMut = useMutation({
    mutationFn: () => createServiceCredential(serviceId, { username: newUser, password: newPass, permissions: newPerms }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["service-credentials", serviceId] }); setShowNewCred(false); setNewUser(""); setNewPass(""); toast({ tone: "success", title: "Credential created" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Credential creation failed", message: e.message }),
  });

  const revokeCredMut = useMutation({
    mutationFn: (credId: string) => revokeServiceCredential(serviceId, credId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["service-credentials", serviceId] }); toast({ tone: "success", title: "Credential revoked" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Revoke failed", message: e.message }),
  });

  if (!svc) {
    return (
      <Modal title="Service Detail" onClose={onClose} wide>
        <div className="py-4 text-center text-sm text-slate-500">Loading...</div>
      </Modal>
    );
  }

  return (
    <Modal title={`Service: ${svc.name || svc.id.slice(0, 8)}`} onClose={onClose} wide>
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4 rounded-lg bg-[#161b28] p-4 text-sm">
          <div><span className="text-slate-500">Type:</span> <span className="text-slate-200">{engineLabels[svc.type] || svc.type} {svc.version}</span></div>
          <div><span className="text-slate-500">Status:</span> <Pill tone={statusTone[svc.status] || "neutral"}>{svc.status}</Pill></div>
          {svc.connectionString && (
            <div className="col-span-2">
              <span className="text-slate-500">Connection String:</span>
              <pre className="mt-1 rounded bg-black/30 p-2 font-mono text-xs text-emerald-300 break-all">{svc.connectionString}</pre>
            </div>
          )}
          <div><span className="text-slate-500">Host:</span> <span className="font-mono text-xs text-slate-200">{svc.host}:{svc.port}</span></div>
          <div><span className="text-slate-500">Database:</span> <span className="font-mono text-xs text-slate-200">{svc.databaseName}</span></div>
          <div><span className="text-slate-500">Username:</span> <span className="font-mono text-xs text-slate-200">{svc.username}</span></div>
          <div><span className="text-slate-500">Memory:</span> <span className="text-slate-200">{svc.memoryMb}MB</span></div>
          {svc.serverId && <div className="col-span-2"><span className="text-slate-500">Linked Server:</span> <span className="font-mono text-xs text-slate-200">{svc.serverId}</span></div>}
        </div>

        <div>
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-sm font-semibold text-slate-300">Backups</h3>
            <Btn size="sm" tone="ghost" onClick={() => backupMut.mutate()} disabled={backupMut.isPending}>
              <Archive size={13} /> {backupMut.isPending ? "Creating..." : "Create Backup"}
            </Btn>
          </div>
          <div className="rounded-lg bg-[#161b28]">
            {backupsQuery.isLoading ? (
              <div className="p-3 text-xs text-slate-500">Loading...</div>
            ) : (backupsQuery.data ?? []).length === 0 ? (
              <div className="p-3 text-xs text-slate-500">No backups</div>
            ) : (
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-slate-500">
                    <th className="px-3 py-2">Status</th>
                    <th className="px-3 py-2">Size</th>
                    <th className="px-3 py-2">Created</th>
                    <th className="px-3 py-2" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(backupsQuery.data ?? []).map((b) => (
                    <tr key={b.id}>
                      <td className="px-3 py-2"><Pill tone={b.status === "completed" ? "green" : b.status === "failed" ? "red" : "yellow"}>{b.status}</Pill></td>
                      <td className="px-3 py-2 text-slate-400">{b.sizeBytes > 0 ? `${(b.sizeBytes / 1024 / 1024).toFixed(1)}MB` : "-"}</td>
                      <td className="px-3 py-2 text-slate-400">{new Date(b.createdAt).toLocaleString()}</td>
                      <td className="px-3 py-2">
                        {b.status === "completed" && (
                          <Btn size="sm" tone="ghost" disabled={restoreMut.isPending} onClick={() => { if (window.confirm("Restore this backup?")) restoreMut.mutate(b.id); }}>Restore</Btn>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>

        <div>
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-sm font-semibold text-slate-300">Credentials</h3>
            <Btn size="sm" tone="ghost" onClick={() => setShowNewCred(true)}><Plus size={13} /> New Credential</Btn>
          </div>
          <div className="rounded-lg bg-[#161b28]">
            {credsQuery.isLoading ? (
              <div className="p-3 text-xs text-slate-500">Loading...</div>
            ) : (credsQuery.data ?? []).length === 0 ? (
              <div className="p-3 text-xs text-slate-500">No credentials</div>
            ) : (
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-slate-500">
                    <th className="px-3 py-2">Username</th>
                    <th className="px-3 py-2">Database</th>
                    <th className="px-3 py-2">Permissions</th>
                    <th className="px-3 py-2">Status</th>
                    <th className="px-3 py-2" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(credsQuery.data ?? []).map((c) => (
                    <tr key={c.id}>
                      <td className="px-3 py-2 font-mono text-slate-200">{c.username}</td>
                      <td className="px-3 py-2 text-slate-400">{c.databaseName}</td>
                      <td className="px-3 py-2"><Pill>{c.permissions}</Pill></td>
                      <td className="px-3 py-2">{c.revokedAt ? <Pill tone="red">Revoked</Pill> : <Pill tone="green">Active</Pill>}</td>
                      <td className="px-3 py-2">
                        {!c.revokedAt && (
                          <Btn size="sm" tone="danger" disabled={revokeCredMut.isPending} onClick={() => revokeCredMut.mutate(c.id)}>Revoke</Btn>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>

        {showNewCred && (
          <div className="rounded-lg border border-white/10 bg-[#1e2536] p-4 space-y-3">
            <h4 className="text-sm font-semibold text-slate-300">New Credential</h4>
            <div className="grid gap-3 md:grid-cols-2">
              <Input label="Username" value={newUser} onChange={setNewUser} placeholder="db_user" />
              <Input label="Password" value={newPass} onChange={setNewPass} type="password" placeholder="password" />
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-1.5">Permissions</label>
                <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100 text-sm" value={newPerms} onChange={(e) => setNewPerms(e.target.value)}>
                  <option value="read-only">Read Only</option>
                  <option value="read-write">Read Write</option>
                  <option value="admin">Admin</option>
                </select>
              </div>
            </div>
            <div className="flex justify-end gap-2">
              <Btn size="sm" tone="ghost" onClick={() => setShowNewCred(false)}>Cancel</Btn>
              <Btn size="sm" disabled={!newUser || !newPass || createCredMut.isPending} onClick={() => createCredMut.mutate()}>
                {createCredMut.isPending ? "Creating..." : "Create"}
              </Btn>
            </div>
          </div>
        )}

        <div>
          <h3 className="mb-2 text-sm font-semibold text-slate-300">Logs</h3>
          <Btn size="sm" tone="ghost" onClick={() => logsQuery.refetch()} disabled={logsQuery.isFetching}>
            {logsQuery.isFetching ? "Loading..." : "Fetch Logs"}
          </Btn>
          {logsQuery.data && (
            <pre className="mt-2 max-h-48 overflow-y-auto rounded bg-black/30 p-3 font-mono text-xs text-slate-400">
              {logsQuery.data.logs.join("\n")}
            </pre>
          )}
        </div>
      </div>
    </Modal>
  );
}

function TemplatesTab({ showCreate, onShowCreate, onClose }: { showCreate: boolean; onShowCreate: () => void; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();

  const templatesQuery = useQuery({
    queryKey: ["service-templates"],
    queryFn: listServiceTemplates,
  });
  const templates = templatesQuery.data ?? [];

  const [newType, setNewType] = useState("postgresql");
  const [newVersion, setNewVersion] = useState("");
  const [newImage, setNewImage] = useState("");
  const [newPort, setNewPort] = useState("");
  const [newMinMem, setNewMinMem] = useState("256");

  const createMut = useMutation({
    mutationFn: () => createServiceTemplate({
      type: newType,
      version: newVersion,
      dockerImage: newImage,
      defaultPort: parseInt(newPort, 10),
      minMemoryMb: parseInt(newMinMem, 10),
    }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["service-templates"] }); onClose(); toast({ tone: "success", title: "Template created" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to create template", message: e.message }),
  });

  return (
    <div>
      <SectionHeader
        title="Service Templates"
        sub="Pre-configured database service definitions"
        action={<Btn onClick={onShowCreate}><Plus size={14} /> Add Template</Btn>}
      />

      <Card>
        <CardHeader title="Templates" icon={Database} />
        {templatesQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading</div>
        ) : templates.length === 0 ? (
          <EmptyState icon={Database} message="No service templates defined." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                <th className="px-4 py-3">Type</th>
                <th className="px-4 py-3">Version</th>
                <th className="px-4 py-3">Docker Image</th>
                <th className="px-4 py-3">Port</th>
                <th className="px-4 py-3">Min Memory</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {templates.map((t) => (
                <tr key={t.id} className="hover:bg-white/[0.02]">
                  <td className="px-4 py-3"><Pill tone="blue">{engineLabels[t.type] || t.type}</Pill></td>
                  <td className="px-4 py-3 font-mono text-xs text-slate-200">{t.version}</td>
                  <td className="px-4 py-3 font-mono text-xs text-slate-400">{t.dockerImage}</td>
                  <td className="px-4 py-3 text-xs text-slate-400">{t.defaultPort}</td>
                  <td className="px-4 py-3 text-xs text-slate-400">{t.minMemoryMb}MB</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {showCreate && (
        <Modal title="Add Service Template" onClose={onClose} wide>
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Type</label>
              <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100 text-sm" value={newType} onChange={(e) => setNewType(e.target.value)}>
                {Object.entries(engineLabels).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
              </select>
            </div>
            <Input label="Version" value={newVersion} onChange={setNewVersion} placeholder="16" />
            <Input label="Docker Image" value={newImage} onChange={setNewImage} placeholder="postgres:16" />
            <Input label="Default Port" value={newPort} onChange={setNewPort} type="number" placeholder="5432" />
            <Input label="Min Memory (MB)" value={newMinMem} onChange={setNewMinMem} type="number" placeholder="256" />
          </div>
          <ModalFooter
            onCancel={onClose}
            onConfirm={() => createMut.mutate()}
            disabled={createMut.isPending || !newVersion || !newImage || !newPort}
            confirmLabel={createMut.isPending ? "Creating..." : "Add Template"}
          />
        </Modal>
      )}
    </div>
  );
}
