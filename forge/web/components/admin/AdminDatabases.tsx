"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle2, Database, Plus, RefreshCw, Server, Trash2 } from "lucide-react";
import { type ApiDatabaseHost, type CreateDatabaseHostInput, createDatabaseHost, deleteDatabaseHost, fetchDatabaseHosts, fetchNodes, fetchOrphanRemediations, resolveDatabaseOrphanRemediation, resolveServerOrphanRemediation, testDatabaseHostConnection, updateDatabaseHost } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "./admin-ui";

const selectStyle = "h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15";

type FieldErrors = {
  name?: string;
  host?: string;
  port?: string;
  username?: string;
  password?: string;
  tlsMode?: string;
  maxDatabases?: string;
};

function validate(name: string, host: string, port: string, username: string, password: string, requirePassword: boolean, tlsMode: string, tlsServerName: string, maxDatabases: string): FieldErrors {
  const errors: FieldErrors = {};
  if (!name.trim()) errors.name = "Display name is required";
  if (!host.trim()) errors.host = "Host is required";
  if (!port.trim() || isNaN(Number(port)) || Number(port) < 1 || Number(port) > 65535) errors.port = "Port must be between 1 and 65535";
  if (!username.trim()) errors.username = "Username is required";
  if (requirePassword && !password) errors.password = "Password is required to test or create a database host";
  if (maxDatabases.trim() && (!Number.isInteger(Number(maxDatabases)) || Number(maxDatabases) <= 0)) errors.maxDatabases = "Max databases must be a positive whole number";
  return errors;
}

export function AdminDatabases() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const hostsQuery = useQuery({ queryKey: ["database-hosts"], queryFn: fetchDatabaseHosts });
  const hosts = hostsQuery.data ?? [];
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = nodesQuery.data ?? [];
  const [remediationStatus, setRemediationStatus] = useState<"pending" | "resolved">("pending");
  const remediationsQuery = useQuery({ queryKey: ["orphan-remediations", remediationStatus], queryFn: () => fetchOrphanRemediations(remediationStatus), retry: false });
  const serverRemediations = remediationsQuery.data?.serverRemediations ?? [];
  const databaseRemediations = remediationsQuery.data?.databaseRemediations ?? [];
  const resolveDatabaseRemediationMut = useMutation({
    mutationFn: resolveDatabaseOrphanRemediation,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["orphan-remediations"] }); toast({ tone: "success", title: "Database orphan remediation resolved" }); },
    onError: (error: Error) => toast({ tone: "error", title: "Could not resolve database remediation", message: error.message }),
  });
  const resolveServerRemediationMut = useMutation({
    mutationFn: resolveServerOrphanRemediation,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["orphan-remediations"] }); toast({ tone: "success", title: "Server orphan remediation resolved" }); },
    onError: (error: Error) => toast({ tone: "error", title: "Could not resolve server remediation", message: error.message }),
  });

  const [modal, setModal] = useState<null | "create" | ApiDatabaseHost>(null);
  const [hName, setHName] = useState("");
  const [hHost, setHHost] = useState("127.0.0.1");
  const [hPort, setHPort] = useState("5432");
  const [hUser, setHUser] = useState("gamepanel");
  const [hPass, setHPass] = useState("");
  const [hEngine, setHEngine] = useState("postgresql");
  const [hNode, setHNode] = useState("");
  const [hMax, setHMax] = useState("");
  const [hTLSMode, setHTLSMode] = useState("verify-full");
  const [hTLSServerName, setHTLSServerName] = useState("");
  const [hTLSCA, setHTLSCA] = useState("");
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [testedConfiguration, setTestedConfiguration] = useState<string | null>(null);

  const databaseHostInput = {
    name: hName.trim(), host: hHost.trim(), port: Number(hPort),
    username: hUser.trim(), password: hPass,
    engine: hEngine, nodeId: hNode || undefined,
    tlsMode: hTLSMode, tlsServerName: hTLSServerName.trim(),
    ...(hTLSCA.trim() ? { tlsCa: hTLSCA } : {}),
    maxDatabases: hMax.trim() ? Number(hMax) : undefined,
  };
  const configurationKey = JSON.stringify(databaseHostInput);

  const createMut = useMutation({
    mutationFn: () => createDatabaseHost(databaseHostInput),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-hosts"] }); setModal(null); toast({ tone: "success", title: "Database host created" }); },
    onError: (e: Error) => { console.error("Create database host error:", e); toast({ tone: "error", title: "Failed to create database host", message: e.message || "Unknown error" }); },
  });
  const updateMut = useMutation({
    mutationFn: (hostId: string) => updateDatabaseHost(hostId, databaseHostInput),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-hosts"] }); setModal(null); toast({ tone: "success", title: "Database host updated" }); },
    onError: (e: Error) => { console.error("Failed to update database host:", e); toast({ tone: "error", title: "Failed to update database host", message: e.message }); },
  });

  const deleteMut = useMutation({
    mutationFn: deleteDatabaseHost,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-hosts"] }); toast({ tone: "success", title: "Database host deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to delete database host", message: e.message }),
  });
  const testMut = useMutation({
    mutationFn: (input: CreateDatabaseHostInput | string) => typeof input === "string" ? testDatabaseHostConnection(input) : testDatabaseHostConnection(input),
    onSuccess: (result) => {
      setTestedConfiguration(configurationKey);
      toast({ tone: "success", title: "Connection test passed", message: result.message ?? "The database host is reachable." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Connection test failed", message: e.message }),
  });

  useEffect(() => {
    setTestedConfiguration(null);
  }, [configurationKey]);

  const openCreate = () => {
    setHName("");
    setHHost("127.0.0.1");
    setHPort("5432");
    setHUser("gamepanel");
    setHPass("");
    setHEngine("postgresql");
    setHNode("");
    setHMax("");
    setHTLSMode("verify-full"); setHTLSServerName(""); setHTLSCA("");
    setFieldErrors({});
    setTestedConfiguration(null);
    setModal("create");
  };

  const openEdit = (host: ApiDatabaseHost) => {
    setHName(host.name);
    setHHost(host.host);
    setHPort(String(host.port));
    setHUser(host.username);
    setHPass("");
    setHEngine(host.engine);
    setHNode(host.nodeId ?? "");
    setHMax(host.maxDatabases === undefined ? "" : String(host.maxDatabases));
    setHTLSMode(host.tlsMode || "verify-full"); setHTLSServerName(host.tlsServerName ?? ""); setHTLSCA("");
    setFieldErrors({});
    setTestedConfiguration(null);
    setModal(host);
  };

  const handleTest = () => {
    const testingSavedHost = modal !== "create" && !hPass;
    const errors = validate(hName, hHost, hPort, hUser, hPass, !testingSavedHost, hTLSMode, hTLSServerName, hMax);
    setFieldErrors(errors);
    if (Object.keys(errors).length === 0) testMut.mutate(testingSavedHost ? (modal as ApiDatabaseHost).id : databaseHostInput);
  };

  const handleConfirm = () => {
    const errors = validate(hName, hHost, hPort, hUser, hPass, modal === "create", hTLSMode, hTLSServerName, hMax);
    setFieldErrors(errors);
    if (Object.keys(errors).length > 0) return;
    if (modal === "create") createMut.mutate();
    else updateMut.mutate((modal as ApiDatabaseHost).id);
  };

  const isPending = createMut.isPending || updateMut.isPending;
  const hasSuccessfulTest = testedConfiguration === configurationKey;

  return (
    <div>
      <SectionHeader
        title="Database hosts"
        sub="External MySQL/PostgreSQL provisioning hosts. This is separate from the panel metadata PostgreSQL reported in Monitoring."
        action={<Btn onClick={openCreate}><Plus size={14} /> New Host</Btn>}
      />

      <Card className="overflow-hidden">
        <CardHeader title="Configured hosts" icon={Database} />
        {hostsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading</div>
        ) : hostsQuery.isError ? (
          <div className="p-5">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load database hosts: {hostsQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void hostsQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : hosts.length === 0 ? (
          <EmptyState icon={Database} message="No database hosts. Add one so servers can create databases." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                  <th className="px-4 py-3 font-semibold">Name</th>
                  <th className="px-4 py-3 font-semibold">Host : Port</th>
                  <th className="px-4 py-3 font-semibold">Engine</th>
                  <th className="px-4 py-3 font-semibold">User</th>
                  <th className="px-4 py-3 font-semibold">DBs</th>
                  <th className="px-4 py-3 font-semibold">Node</th>
                  <th className="px-4 py-3 font-semibold" />
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {hosts.map((host) => (
                  <tr key={host.id} className="transition-colors hover:bg-white/[0.02]">
                    <td className="px-4 py-3 font-medium text-slate-200">{host.name}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">{host.host}:{host.port}</td>
                    <td className="px-4 py-3"><Pill tone="blue">{host.engine}</Pill></td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">{host.username}</td>
                    <td className="px-4 py-3"><Pill>{host.databases}</Pill></td>
                    <td className="px-4 py-3 text-xs text-slate-500">{host.nodeName ?? "-"}</td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1">
                        <Btn size="sm" tone="ghost" onClick={() => testMut.mutate(host.id)} disabled={testMut.isPending}>{testMut.isPending && testMut.variables === host.id ? "Testing..." : "Test"}</Btn>
                        <Btn size="sm" tone="ghost" onClick={() => openEdit(host)}>Edit</Btn>
                        <Btn size="sm" tone="danger" onClick={() => deleteMut.mutate(host.id)} disabled={deleteMut.isPending}><Trash2 size={12} /></Btn>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card className="overflow-hidden">
        <CardHeader title="Orphan remediation" icon={AlertCircle} />
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/[0.06] bg-white/[0.02] px-5 py-4 text-sm text-slate-400">
          <p>Force-deleted server and database resources that could not be removed remotely are tracked here for administrator follow-up.</p>
          <div className="flex items-center gap-2">
            <select className={selectStyle + " h-9 text-xs"} aria-label="Orphan remediation status" value={remediationStatus} onChange={(event) => setRemediationStatus(event.target.value as "pending" | "resolved")}>
              <option value="pending">Pending</option>
              <option value="resolved">Resolved</option>
            </select>
            <Btn size="sm" tone="ghost" onClick={() => void remediationsQuery.refetch()} disabled={remediationsQuery.isFetching}>
              <RefreshCw size={13} /> {remediationsQuery.isFetching ? "Refreshing..." : "Refresh"}
            </Btn>
          </div>
        </div>

        {remediationsQuery.isLoading ? (
          <div className="px-5 pb-5 pt-4 text-sm text-slate-500">Loading remediation tasks…</div>
        ) : remediationsQuery.isError ? (
          <div className="px-5 pb-5 pt-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load orphan remediation tasks: {remediationsQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void remediationsQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : (
          <div>
            <div className="flex items-center gap-2 border-b border-white/[0.06] bg-[#161b28]/50 px-5 py-3 text-xs font-semibold uppercase tracking-widest text-slate-400">
              <Server size={14} /> Server resources <Pill>{serverRemediations.length}</Pill>
            </div>
            {serverRemediations.length === 0 ? (
              <div className="px-5 py-4 text-sm text-slate-500">No {remediationStatus} server orphan remediation tasks.</div>
            ) : (
              <div className="divide-y divide-white/[0.06]">
                {serverRemediations.map((remediation) => {
                  const isResolving = resolveServerRemediationMut.isPending && resolveServerRemediationMut.variables === remediation.id;
                  return (
                    <div className="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center lg:justify-between" key={remediation.id}>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-slate-200">Server {remediation.serverId}</span>
                          <Pill tone={remediation.status === "pending" ? "yellow" : "green"}>{remediation.status}</Pill>
                        </div>
                        <p className="mt-1 break-all font-mono text-xs text-slate-500">Node: {remediation.nodeUrl}</p>
                        <p className="mt-2 break-words text-xs text-red-200">{remediation.daemonError}</p>
                        <p className="mt-2 text-xs text-slate-500">Reported {new Date(remediation.createdAt).toLocaleString()}</p>
                      </div>
                      {remediation.status === "pending" ? (
                        <Btn size="sm" tone="ghost" disabled={resolveServerRemediationMut.isPending} onClick={() => {
                          if (window.confirm(`Mark server ${remediation.serverId} as resolved after confirming its remote resource has been cleaned up?`)) resolveServerRemediationMut.mutate(remediation.id);
                        }}>{isResolving ? "Resolving..." : "Mark resolved"}</Btn>
                      ) : <span className="text-xs text-slate-500">Resolved {remediation.resolvedAt ? new Date(remediation.resolvedAt).toLocaleString() : ""}</span>}
                    </div>
                  );
                })}
              </div>
            )}

            <div className="flex items-center gap-2 border-y border-white/[0.06] bg-[#161b28]/50 px-5 py-3 text-xs font-semibold uppercase tracking-widest text-slate-400">
              <Database size={14} /> Database resources <Pill>{databaseRemediations.length}</Pill>
            </div>
            {databaseRemediations.length === 0 ? (
              <div className="px-5 py-4 text-sm text-slate-500">No {remediationStatus} database orphan remediation tasks.</div>
            ) : (
              <div className="divide-y divide-white/[0.06]">
                {databaseRemediations.map((remediation) => {
                  const isResolving = resolveDatabaseRemediationMut.isPending && resolveDatabaseRemediationMut.variables === remediation.id;
                  return (
                    <div className="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center lg:justify-between" key={remediation.id}>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-slate-200">{remediation.database}</span>
                          <Pill tone={remediation.status === "pending" ? "yellow" : "green"}>{remediation.status}</Pill>
                        </div>
                        <p className="mt-1 break-all font-mono text-xs text-slate-500">{remediation.engine} · {remediation.host}:{remediation.port} · {remediation.username}@{remediation.remote}</p>
                        <p className="mt-2 break-words text-xs text-red-200">{remediation.reason}</p>
                        <p className="mt-2 text-xs text-slate-500">Reported {new Date(remediation.createdAt).toLocaleString()}</p>
                      </div>
                      {remediation.status === "pending" ? (
                        <Btn size="sm" tone="ghost" disabled={resolveDatabaseRemediationMut.isPending} onClick={() => {
                          if (window.confirm(`Mark ${remediation.database} as resolved after confirming its remote resource has been cleaned up?`)) resolveDatabaseRemediationMut.mutate(remediation.id);
                        }}>{isResolving ? "Resolving..." : "Mark resolved"}</Btn>
                      ) : <span className="text-xs text-slate-500">Resolved {remediation.resolvedAt ? new Date(remediation.resolvedAt).toLocaleString() : ""}</span>}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </Card>

      {modal ? (
        <Modal title={modal === "create" ? "Add Database Host" : "Edit Database Host"} onClose={() => setModal(null)} className="max-w-3xl">
          <div className="space-y-5">
            <div className="grid gap-5 sm:grid-cols-2">
              <div>
                <Input label="Display name" value={hName} onChange={setHName} placeholder="Local PostgreSQL" />
                {fieldErrors.name ? <p className="mt-1 text-xs text-red-400">{fieldErrors.name}</p> : null}
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-slate-300">Engine</label>
                <select className={selectStyle} value={hEngine} onChange={(e) => setHEngine(e.target.value)}>
                  <option value="postgresql">PostgreSQL</option>
                  <option value="mysql">MySQL</option>
                </select>
              </div>
              <div>
                <Input label="Host" value={hHost} onChange={setHHost} placeholder="db.internal.example" mono />
                {fieldErrors.host ? <p className="mt-1 text-xs text-red-400">{fieldErrors.host}</p> : <p className="mt-1 text-xs text-slate-500">Resolved by the panel API. In a container, 127.0.0.1 is the API container, not automatically the panel database.</p>}
              </div>
              <div>
                <Input label="Port" value={hPort} onChange={setHPort} type="number" placeholder="5432" />
                {fieldErrors.port ? <p className="mt-1 text-xs text-red-400">{fieldErrors.port}</p> : null}
              </div>
              <div>
                <Input label="Username" value={hUser} onChange={setHUser} placeholder="gamepanel" mono />
                {fieldErrors.username ? <p className="mt-1 text-xs text-red-400">{fieldErrors.username}</p> : null}
              </div>
              <div>
                <Input label={modal === "create" ? "Password" : "Password (blank keeps current)"} value={hPass} onChange={setHPass} type="password" placeholder="" />
                {fieldErrors.password ? <p className="mt-1 text-xs text-red-400">{fieldErrors.password}</p> : null}
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-slate-300">Linked node (optional)</label>
                <select className={selectStyle} value={hNode} onChange={(e) => setHNode(e.target.value)}>
                  <option value="">None</option>
                  {nodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
                </select>
              </div>
              <div>
                <Input label="Max databases (blank = unlimited)" value={hMax} onChange={setHMax} type="number" placeholder="unlimited" />
                {fieldErrors.maxDatabases ? <p className="mt-1 text-xs text-red-400">{fieldErrors.maxDatabases}</p> : null}
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-slate-300">TLS Mode</label>
                <select className={selectStyle} value={hTLSMode} onChange={(e) => setHTLSMode(e.target.value)}>
                  <option value="disable">Disable</option>
                  <option value="required">Require</option>
                  <option value="verify-ca">Verify CA</option>
                  <option value="verify-full">Verify Full</option>
                </select>
                {fieldErrors.tlsMode ? <p className="mt-1 text-xs text-red-400">{fieldErrors.tlsMode}</p> : <p className="mt-1 text-xs text-slate-500">Verify Full validates the server certificate and name. A custom CA is optional.</p>}
              </div>
              <Input label="TLS Server Name (SNI, optional)" value={hTLSServerName} onChange={setHTLSServerName} mono />
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-300">TLS CA certificate (write-only)</label>
              <textarea className="h-28 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 py-2 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15 font-mono text-xs" value={hTLSCA} onChange={(e) => setHTLSCA(e.target.value)} placeholder={modal === "create" ? "Optional PEM certificate" : "Leave blank to keep current certificate"}/>
              <p className="mt-1 text-xs text-slate-500">Certificates and passwords are redacted by the API and never displayed after submission.</p>
            </div>
          </div>
          {createMut.isError ? (
            <div className="mt-5 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
              <AlertCircle size={14} className="mt-0.5 shrink-0" />
              <span>{createMut.error?.message || "An unexpected error occurred."}</span>
            </div>
          ) : null}
          {updateMut.isError ? (
            <div className="mt-5 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
              <AlertCircle size={14} className="mt-0.5 shrink-0" />
              <span>{updateMut.error?.message || "An unexpected error occurred."}</span>
            </div>
          ) : null}
          {createMut.isSuccess || updateMut.isSuccess ? (
            <div className="mt-5 flex items-start gap-2 rounded-lg border border-emerald-500/20 bg-emerald-950/10 p-3 text-xs text-emerald-200">
              <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
              <span>Database host {modal === "create" ? "created" : "updated"} successfully.</span>
            </div>
          ) : null}
          <div className="mt-5 flex items-center justify-between gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] p-4">
            <p className="text-xs text-slate-500">Test the current settings successfully before saving.</p>
            <Btn tone="success" type="button" onClick={handleTest} disabled={testMut.isPending}>
              {testMut.isPending ? "Testing..." : "Test Connection"}
            </Btn>
          </div>
          <ModalFooter
            onCancel={() => setModal(null)}
            onConfirm={handleConfirm}
            disabled={!hName.trim() || !hHost.trim() || !hPort.trim() || !hUser.trim() || (modal === "create" && !hPass) || !hasSuccessfulTest || isPending}
            confirmLabel={isPending ? "Saving..." : (modal === "create" ? "Create" : "Save")}
          />
        </Modal>
      ) : null}
    </div>
  );
}
