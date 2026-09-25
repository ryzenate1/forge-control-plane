"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, Plus, Trash2, RotateCcw, FlaskConical, Layers, Server } from "lucide-react";
import {
  listDatabaseServices,
  provisionDatabaseService,
  deleteDatabaseService,
  restartDatabaseService,
  testConnection,
  listServiceTemplates,
  createServiceTemplate,
  type DatabaseService,
  type ServiceTemplate,
} from "@/lib/api/database-services";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, Pill, AdminSelect } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { statusTone } from "@/lib/api/status";

const selectCls = "h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-[var(--brand)]/70 focus:ring-2 focus:ring-[var(--brand)]/15";

export function DatabaseServicesView() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [showProvision, setShowProvision] = useState(false);
  const [showTemplate, setShowTemplate] = useState(false);
  const [showTest, setShowTest] = useState(false);
  const [templateFilter, setTemplateFilter] = useState("");

  const servicesQ = useQuery({ queryKey: ["database-services"], queryFn: listDatabaseServices });
  const templatesQ = useQuery({ queryKey: ["service-templates"], queryFn: listServiceTemplates });

  const services = servicesQ.data ?? [];
  const templates = templatesQ.data ?? [];
  const filteredTemplates = templateFilter ? templates.filter((t) => t.type.toLowerCase().includes(templateFilter.toLowerCase()) || t.version.includes(templateFilter)) : templates;

  const restartMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await restartDatabaseService(id);
      if (!result.ok) throw new Error("The server reported the service restart did not complete.");
      return result;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-services"] }); toast({ tone: "success", title: "Service restart initiated — POST /admin/database-services/:id/restart" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Restart failed", message: e.message }),
  });
  const deleteMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await deleteDatabaseService(id);
      if (!result.ok) throw new Error("The server reported the database service was not deleted.");
      return result;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-services"] }); toast({ tone: "success", title: "Service deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Database Services"
        sub="Provisioned database services (PostgreSQL/MySQL/Redis/etc) — provision, restart, test connection, templates"
        action={
          <div className="flex flex-wrap gap-2">
            <Btn tone="ghost" onClick={() => setShowTest(true)}><FlaskConical size={14} /> Test Connection</Btn>
            <Btn tone="ghost" onClick={() => setShowTemplate(true)}><Layers size={14} /> New Template</Btn>
            <Btn onClick={() => setShowProvision(true)}><Plus size={14} /> Provision Service</Btn>
          </div>
        }
      />

      <Card className="overflow-hidden">
        <CardHeader title="Service Templates" icon={Layers} action={<Input placeholder="Filter type/version..." value={templateFilter} onChange={setTemplateFilter} />} />
        {templatesQ.isLoading ? (
          <div className="py-6 text-center text-sm text-slate-400">Loading templates…</div>
        ) : templatesQ.isError ? (
          <div className="p-4 text-sm text-red-300">Failed to load templates: {(templatesQ.error as Error).message} <Btn size="sm" tone="ghost" onClick={() => void templatesQ.refetch()}>Retry</Btn></div>
        ) : filteredTemplates.length === 0 ? (
          <EmptyState icon={Layers} title="No templates" message="No service templates yet. Create one to define default images/ports. GET /admin/database-service-templates" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead><tr className="border-b border-white/[0.06] text-left text-xs uppercase tracking-wider text-slate-400"><th className="px-4 py-2">Type</th><th className="px-4 py-2">Version</th><th className="px-4 py-2">Image</th><th className="px-4 py-2">Port</th><th className="px-4 py-2">Min Mem</th></tr></thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filteredTemplates.map((t) => (
                  <tr key={t.id} className="hover:bg-white/[0.02]"><td className="px-4 py-2"><Pill tone="blue">{t.type}</Pill></td><td className="px-4 py-2 font-mono text-xs text-slate-300">{t.version}</td><td className="px-4 py-2 font-mono text-xs text-slate-400">{t.dockerImage}</td><td className="px-4 py-2 font-mono text-xs text-slate-400">{t.defaultPort}</td><td className="px-4 py-2 text-xs text-slate-400">{t.minMemoryMb} MB</td></tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="px-4 py-2 text-[11px] text-slate-500">Wires <code className="font-mono">listServiceTemplates</code> &amp; <code className="font-mono">createServiceTemplate</code> — POST /admin/database-service-templates</p>
      </Card>

      <Card className="overflow-hidden">
        <CardHeader title="Database Services" icon={Database} />
        {servicesQ.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-400">Loading services…</div>
        ) : servicesQ.isError ? (
          <div className="p-4 text-sm text-red-300">Failed to load: {(servicesQ.error as Error).message} <Btn size="sm" tone="ghost" onClick={() => void servicesQ.refetch()}>Retry</Btn></div>
        ) : services.length === 0 ? (
          <EmptyState icon={Database} title="No services" message="No database services provisioned. Use Provision Service — POST /admin/database-services" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead><tr className="border-b border-white/[0.06] text-left text-xs uppercase tracking-wider text-slate-400"><th className="px-4 py-2">Name</th><th className="px-4 py-2">Type</th><th className="px-4 py-2">Status</th><th className="px-4 py-2">Host:Port</th><th className="px-4 py-2">Memory</th><th className="px-4 py-2" /></tr></thead>
              <tbody className="divide-y divide-white/[0.04]">
                {services.map((svc) => (
                  <tr key={svc.id} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-2 font-medium text-slate-200">{svc.name || svc.id.slice(0, 8)}</td>
                    <td className="px-4 py-2"><Pill tone="blue">{svc.type} {svc.version}</Pill></td>
                    <td className="px-4 py-2"><Pill tone={statusTone(svc.status)}>{svc.status}</Pill></td>
                    <td className="px-4 py-2 font-mono text-xs text-slate-400">{svc.host}:{svc.port}</td>
                    <td className="px-4 py-2 text-xs text-slate-400">{svc.memoryMb} MB</td>
                    <td className="px-4 py-2">
                      <div className="flex items-center justify-end gap-1">
                        <button disabled={restartMut.isPending} onClick={() => restartMut.mutate(svc.id)} className="grid h-9 w-9 place-items-center rounded text-slate-400 hover:bg-white/[0.06] hover:text-amber-200 disabled:opacity-40" title="Restart — POST /admin/database-services/:id/restart" type="button"><RotateCcw size={14} /></button>
                        <button disabled={deleteMut.isPending} onClick={() => deleteMut.mutate(svc.id)} className="grid h-9 w-9 place-items-center rounded text-slate-400 hover:bg-white/[0.06] hover:text-red-200 disabled:opacity-40" title="Delete" type="button"><Trash2 size={14} /></button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="px-4 py-2 text-[11px] text-slate-500">Wires <code className="font-mono">provisionDatabaseService</code>, <code className="font-mono">restartDatabaseService</code>, <code className="font-mono">testConnection</code></p>
      </Card>

      {showProvision && <ProvisionModal onClose={() => setShowProvision(false)} onDone={() => { setShowProvision(false); qc.invalidateQueries({ queryKey: ["database-services"] }); }} />}
      {showTemplate && <TemplateModal onClose={() => setShowTemplate(false)} onDone={() => { setShowTemplate(false); qc.invalidateQueries({ queryKey: ["service-templates"] }); }} />}
      {showTest && <TestConnectionModal onClose={() => setShowTest(false)} />}
    </div>
  );
}

function ProvisionModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const { toast } = useToast();
  const [name, setName] = useState("");
  const [type, setType] = useState("postgresql");
  const [version, setVersion] = useState("16");
  const [memoryMb, setMemoryMb] = useState(256);
  const templatesQ = useQuery({ queryKey: ["service-templates"], queryFn: listServiceTemplates });

  const mut = useMutation({
    mutationFn: () => provisionDatabaseService({ name: name || `db-${Date.now()}`, type, version, memoryMb }),
    onSuccess: () => { toast({ tone: "success", title: "Database service provisioned — POST /admin/database-services" }); onDone(); },
    onError: (e: Error) => toast({ tone: "error", title: "Provision failed", message: e.message }),
  });

  return (
    <Modal title="Provision Database Service — POST /admin/database-services" onClose={onClose} wide>
      <div className="space-y-4">
        <Input label="Name (optional, auto if blank)" value={name} onChange={setName} placeholder="my-db-service" />
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Type</label>
            <select className={selectCls} value={type} onChange={(e) => { setType(e.target.value); const t = templatesQ.data?.filter((x) => x.type===e.target.value); if (t && t.length) setVersion(t[t.length-1].version); }}>
              <option value="postgresql">postgresql</option>
              <option value="mysql">mysql</option>
              <option value="mariadb">mariadb</option>
              <option value="redis">redis</option>
              <option value="mongodb">mongodb</option>
            </select>
          </div>
          <Input label="Version" value={version} onChange={setVersion} placeholder="16" />
        </div>
        <div>
          <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Memory MB</label>
          <input type="number" className={selectCls} value={memoryMb} onChange={(e) => setMemoryMb(Number(e.target.value))} min={64} step={64} />
        </div>
        {templatesQ.data && templatesQ.data.length > 0 && (
          <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-400">Templates selector — pick to autofill version/image</p>
            <div className="flex flex-wrap gap-2">
              {templatesQ.data.map((t) => (
                <button key={t.id} type="button" onClick={() => { setType(t.type); setVersion(t.version); }} className={`rounded-full border px-3 py-1 text-xs ${type===t.type && version===t.version ? "border-[var(--brand)] bg-[var(--brand)]/20 text-white" : "border-white/10 bg-white/[0.04] text-slate-300 hover:bg-white/[0.06]"}`}>{t.type}:{t.version} → {t.dockerImage}</button>
              ))}
            </div>
          </div>
        )}
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => mut.mutate()} disabled={mut.isPending || !type || !version} confirmLabel={mut.isPending ? "Provisioning…" : "Provision"} />
    </Modal>
  );
}

function TemplateModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const { toast } = useToast();
  const [type, setType] = useState("postgresql");
  const [version, setVersion] = useState("16");
  const [dockerImage, setDockerImage] = useState("postgres:16-alpine");
  const [defaultPort, setDefaultPort] = useState(5432);
  const [defaultDatabase, setDefaultDatabase] = useState("postgres");
  const [minMemoryMb, setMinMemoryMb] = useState(256);

  const mut = useMutation({
    mutationFn: () => createServiceTemplate({ type, version, dockerImage, defaultPort, defaultDatabase, minMemoryMb }),
    onSuccess: () => { toast({ tone: "success", title: "Template created — POST /admin/database-service-templates" }); onDone(); },
    onError: (e: Error) => toast({ tone: "error", title: "Create template failed", message: e.message }),
  });

  return (
    <Modal title="Create Service Template — POST /admin/database-service-templates" onClose={onClose} wide>
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Type</label><select className={selectCls} value={type} onChange={(e) => setType(e.target.value)}><option value="postgresql">postgresql</option><option value="mysql">mysql</option><option value="mariadb">mariadb</option><option value="redis">redis</option><option value="mongodb">mongodb</option></select></div>
          <Input label="Version" value={version} onChange={setVersion} placeholder="16" />
        </div>
        <Input label="Docker Image" value={dockerImage} onChange={setDockerImage} placeholder="postgres:16-alpine" />
        <div className="grid gap-4 sm:grid-cols-3">
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Default Port</label><input type="number" className={selectCls} value={defaultPort} onChange={(e) => setDefaultPort(Number(e.target.value))} /></div>
          <Input label="Default DB" value={defaultDatabase} onChange={setDefaultDatabase} placeholder="postgres" />
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Min Memory MB</label><input type="number" className={selectCls} value={minMemoryMb} onChange={(e) => setMinMemoryMb(Number(e.target.value))} /></div>
        </div>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => mut.mutate()} disabled={mut.isPending || !type || !version || !dockerImage} confirmLabel={mut.isPending ? "Creating…" : "Create Template"} />
    </Modal>
  );
}

function TestConnectionModal({ onClose }: { onClose: () => void }) {
  const { toast } = useToast();
  const [host, setHost] = useState("127.0.0.1");
  const [port, setPort] = useState("5432");
  const [engine, setEngine] = useState("postgresql");
  const [username, setUsername] = useState("gamepanel");
  const [password, setPassword] = useState("");
  const [databaseName, setDatabaseName] = useState("postgres");

  const mut = useMutation({
    mutationFn: async () => {
      const result = await testConnection({ host, port: Number(port), engine, username, password, databaseName });
      if (!result.ok) throw new Error(result.message || "The server reported the connection test did not succeed.");
      return result;
    },
    onSuccess: (res) => toast({ tone: "success", title: res.message || "Connection successful — POST /admin/database-services/test-connection" }),
    onError: (e: Error) => toast({ tone: "error", title: "Test failed", message: e.message }),
  });

  return (
    <Modal title="Test Connection — POST /admin/database-services/test-connection" onClose={onClose} wide>
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label="Host" value={host} onChange={setHost} placeholder="127.0.0.1" />
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Port</label><input className={selectCls} value={port} onChange={(e) => setPort(e.target.value)} placeholder="5432" /></div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Engine</label><select className={selectCls} value={engine} onChange={(e) => setEngine(e.target.value)}><option value="postgresql">postgresql</option><option value="mysql">mysql</option><option value="mariadb">mariadb</option><option value="redis">redis</option><option value="mongodb">mongodb</option></select></div>
          <Input label="Username" value={username} onChange={setUsername} />
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label="Password" value={password} onChange={setPassword} type="password" placeholder="••••••••" />
          <Input label="Database" value={databaseName} onChange={setDatabaseName} placeholder="postgres" />
        </div>
        <p className="text-xs text-slate-400">Tests <code className="font-mono">POST /admin/database-services/test-connection</code> — host/engine/username required, port defaults via defaultPortForEngine.</p>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => mut.mutate()} disabled={mut.isPending || !host || !engine || !username} confirmLabel={mut.isPending ? "Testing…" : "Test Connection"} />
    </Modal>
  );
}
