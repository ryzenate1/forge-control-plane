"use client";
import { useNodesQuery } from "@/lib/admin/telemetry";

import { useEffect, useState, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Box, CheckCircle2, Database, LayoutGrid, List, Network, Plus, RefreshCw, Search, Server, Trash2 } from "lucide-react";
import { type ApiDatabaseHost, type CreateDatabaseHostInput, createDatabaseHost, deleteDatabaseHost, fetchDatabaseHosts, fetchOrphanRemediations, resolveDatabaseOrphanRemediation, resolveServerOrphanRemediation, testDatabaseHostConnection, updateDatabaseHost } from "@/lib/api";
import { toast } from "@/components/ui/sonner";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminSelect, AdminFormSection, AdminLoadingState, cn } from "./admin-ui";
import { DbStatCards, type DbStat } from "../database/databases-overview";
import { Pagination } from "@/components/ui/primitives";

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

function hostEngineLabel(engine?: string): string {
  if (!engine) return "—";
  const map: Record<string, string> = { postgresql: "PostgreSQL", mysql: "MySQL", mariadb: "MariaDB" };
  return map[engine.toLowerCase()] ?? engine;
}

export function AdminDatabases({ embedded = false }: { embedded?: boolean }) {
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const hostsQuery = useQuery({ queryKey: ["database-hosts"], queryFn: fetchDatabaseHosts });
  const hosts = useMemo(() => Array.isArray(hostsQuery.data) ? hostsQuery.data : [], [hostsQuery.data]);
  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => Array.isArray(nodesQuery.data) ? nodesQuery.data : [], [nodesQuery.data]);
  const [remediationStatus, setRemediationStatus] = useState<"pending" | "resolved">("pending");
  const remediationsQuery = useQuery({ queryKey: ["orphan-remediations", remediationStatus], queryFn: () => fetchOrphanRemediations(remediationStatus), retry: false });
  const serverRemediations = useMemo(() => Array.isArray(remediationsQuery.data?.serverRemediations) ? remediationsQuery.data.serverRemediations : [], [remediationsQuery.data]);
  const databaseRemediations = useMemo(() => Array.isArray(remediationsQuery.data?.databaseRemediations) ? remediationsQuery.data.databaseRemediations : [], [remediationsQuery.data]);
  const resolveDatabaseRemediationMut = useMutation({
    mutationFn: resolveDatabaseOrphanRemediation,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["orphan-remediations"] }); toast.success("Database orphan remediation resolved"); },
    onError: (error: Error) => toast.error(error.message || "Could not resolve database remediation"),
  });
  const resolveServerRemediationMut = useMutation({
    mutationFn: resolveServerOrphanRemediation,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["orphan-remediations"] }); toast.success("Server orphan remediation resolved"); },
    onError: (error: Error) => toast.error(error.message || "Could not resolve server remediation"),
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
  const [hostsPage, setHostsPage] = useState(1);
  const HOSTS_PAGE_SIZE = 10;
  const [hostSearch, setHostSearch] = useState("");
  const [hostEngineFilter, setHostEngineFilter] = useState("all");
  const [hostView, setHostView] = useState<"table" | "cards">("table");
  const [hostSort, setHostSort] = useState("name-asc");

  const hostSelectCls = "h-10 cursor-pointer appearance-none rounded-lg border border-white/[0.08] bg-black/20 pl-3 pr-8 text-xs text-slate-200 outline-none";
  const hostSelectWrap = "relative flex flex-col justify-center rounded-lg border border-white/[0.08] bg-black/20 px-3 py-1";

  const hostEngines = useMemo(() => [...new Set(hosts.map((h) => h.engine))].sort(), [hosts]);
  const hostStats: DbStat[] = useMemo(() => {
    const loading = hostsQuery.isLoading;
    const engines = new Set(hosts.map((h) => h.engine));
    const totalDatabases = hosts.reduce((sum, h) => sum + (h.databases ?? h.maxDatabases ?? 0), 0);
    const nodes = new Set(hosts.map((h) => h.nodeId ?? h.nodeName).filter((v): v is string => Boolean(v)));
    return [
      { key: "total", label: "Total Hosts", icon: Server, tile: "border-white/[0.08] bg-white/[0.03] text-slate-300", value: loading ? "…" : hosts.length },
      { key: "engines", label: "Engines", icon: Database, tile: "border-sky-500/25 bg-sky-500/10 text-sky-300", value: loading ? "…" : engines.size },
      { key: "databases", label: "Databases", icon: Box, tile: "border-amber-500/25 bg-amber-500/10 text-amber-300", value: loading ? "…" : totalDatabases },
      { key: "nodes", label: "Nodes", icon: Network, tile: "border-emerald-500/25 bg-emerald-500/10 text-emerald-300", value: loading ? "…" : nodes.size },
    ];
  }, [hosts, hostsQuery.isLoading]);
  const filteredHosts = useMemo(() => {
    const term = hostSearch.trim().toLowerCase();
    return hosts.filter((h) => {
      if (hostEngineFilter !== "all" && h.engine !== hostEngineFilter) return false;
      if (!term) return true;
      return `${h.name} ${h.host} ${h.engine}`.toLowerCase().includes(term);
    });
  }, [hosts, hostSearch, hostEngineFilter]);
  const sortedHosts = useMemo(() => {
    const list = [...filteredHosts];
    switch (hostSort) {
      case "name-desc": return list.sort((a, b) => b.name.localeCompare(a.name));
      case "engine": return list.sort((a, b) => a.engine.localeCompare(b.engine) || a.name.localeCompare(b.name));
      default: return list.sort((a, b) => a.name.localeCompare(b.name));
    }
  }, [filteredHosts, hostSort]);
  const hostPageCount = Math.max(1, Math.ceil(sortedHosts.length / HOSTS_PAGE_SIZE));
  const safeHostsPage = Math.min(Math.max(hostsPage, 1), hostPageCount);
  const visibleHosts = sortedHosts.slice((safeHostsPage - 1) * HOSTS_PAGE_SIZE, safeHostsPage * HOSTS_PAGE_SIZE);
  const hostsRangeFrom = sortedHosts.length === 0 ? 0 : (safeHostsPage - 1) * HOSTS_PAGE_SIZE + 1;
  const hostsRangeTo = Math.min(safeHostsPage * HOSTS_PAGE_SIZE, sortedHosts.length);

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
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-hosts"] }); setModal(null); toast.success("Database host created"); },
    onError: (e: Error) => { console.error("Create database host error:", e); toast.error(e.message || "Failed to create database host"); },
  });
  const updateMut = useMutation({
    mutationFn: (hostId: string) => updateDatabaseHost(hostId, databaseHostInput),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-hosts"] }); setModal(null); toast.success("Database host updated"); },
    onError: (e: Error) => { console.error("Failed to update database host:", e); toast.error(e.message || "Failed to update database host"); },
  });

  const deleteMut = useMutation({
    mutationFn: deleteDatabaseHost,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-hosts"] }); toast.success("Database host deleted"); },
    onError: (e: Error) => toast.error(e.message || "Failed to delete database host"),
  });
  const testMut = useMutation({
    mutationFn: (input: CreateDatabaseHostInput | string) => typeof input === "string" ? testDatabaseHostConnection(input) : testDatabaseHostConnection(input),
    onSuccess: (result) => {
      setTestedConfiguration(configurationKey);
      toast.success(result.message ?? "The database host is reachable.");
    },
    onError: (e: Error) => toast.error(e.message || "Connection test failed"),
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
    <div className="space-y-6">
      {!embedded && (
        <SectionHeader
          title="Database Hosts"
          sub="Connect external MySQL and PostgreSQL hosts used to provision workload databases."
          action={<Btn tone="primary" onClick={openCreate}><Plus size={14} /> New Host</Btn>}
        />
      )}
      {embedded && (
        <div className="flex flex-wrap items-start justify-between gap-3 rounded-xl border border-line bg-overlay-subtle px-4 py-3">
          <div>
            <h2 className="text-sm font-semibold text-text">Database Hosts</h2>
            <p className="mt-1 text-xs leading-5 text-text-subtle">External MySQL and PostgreSQL connections. Credentials are encrypted and TLS verification is enabled by default.</p>
          </div>
          <Btn size="sm" onClick={openCreate}><Plus size={14} /> New Host</Btn>
        </div>
      )}

      <DbStatCards stats={hostStats} />

      <Card className="overflow-hidden">
        <CardHeader title="Configured hosts" icon={Database} />
        {hostsQuery.isLoading ? (
          <AdminLoadingState label="Loading database hosts…" />
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
          <>
            <div className="space-y-4 px-5 pb-5 pt-4">
              <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
                <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2">
                  <Search size={13} className="shrink-0 text-slate-500" />
                  <input
                    type="text"
                    value={hostSearch}
                    onChange={(e) => { setHostSearch(e.target.value); setHostsPage(1); }}
                    placeholder="Search hosts by name, host, engine…"
                    aria-label="Search database hosts"
                    className="w-full bg-transparent text-xs text-slate-200 outline-none placeholder:text-slate-600"
                  />
                </label>
                <div className="flex flex-wrap items-center gap-2">
                  <label className={hostSelectWrap}>
                    <span className="text-[10px] leading-3 text-slate-500">Engine</span>
                    <select aria-label="Filter by engine" value={hostEngineFilter} onChange={(e) => { setHostEngineFilter(e.target.value); setHostsPage(1); }} className={hostSelectCls + " h-6 border-0 bg-transparent pl-0 text-xs"}>
                      <option value="all">All</option>
                      {hostEngines.map((e) => <option key={e} value={e}>{e}</option>)}
                    </select>
                  </label>
                  <div className="flex gap-1 rounded-lg border border-white/[0.08] bg-black/20 p-1" role="group" aria-label="View mode">
                    <button type="button" aria-label="Table view" aria-pressed={hostView === "table"} onClick={() => setHostView("table")} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", hostView === "table" ? "border border-red-500/50 bg-red-500/10 text-red-200" : "text-slate-500 hover:text-slate-300")}>
                      <List size={14} /> Table
                    </button>
                    <button type="button" aria-label="Cards view" aria-pressed={hostView === "cards"} onClick={() => setHostView("cards")} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", hostView === "cards" ? "border border-red-500/50 bg-red-500/10 text-red-200" : "text-slate-500 hover:text-slate-300")}>
                      <LayoutGrid size={14} /> Cards
                    </button>
                  </div>
                </div>
              </div>

              <div className="flex flex-wrap items-center justify-between gap-3">
                <h2 className="text-sm font-bold text-slate-100">Hosts ({sortedHosts.length})</h2>
                <label className="flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-1.5 text-xs text-slate-300">
                  <span className="text-[11px] text-slate-500">Sort by</span>
                  <select aria-label="Sort database hosts" value={hostSort} onChange={(e) => setHostSort(e.target.value)} className="cursor-pointer appearance-none bg-transparent pr-1 outline-none">
                    <option value="name-asc">Name (A → Z)</option>
                    <option value="name-desc">Name (Z → A)</option>
                    <option value="engine">Engine</option>
                  </select>
                </label>
              </div>

              {sortedHosts.length === 0 ? (
                <EmptyState icon={Database} title="No matches" message="No database hosts match these filters." />
              ) : hostView === "cards" ? (
                <>
                  <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                    {visibleHosts.map((host) => (
                      <div key={host.id} className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition hover:border-white/20">
                        <div className="flex items-start gap-3">
                          <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-white/[0.08] bg-white/[0.03] text-slate-300">
                            <Database size={18} />
                          </span>
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-sm font-bold text-slate-100" title={host.name}>{host.name}</p>
                            <p className="font-mono text-[10px] text-slate-500">{host.id.slice(0, 8)}</p>
                          </div>
                        </div>
                        <div className="mt-3 space-y-1 border-t border-white/[0.06] pt-3 font-mono text-[11px] text-slate-400">
                          <p className="truncate">{hostEngineLabel(host.engine)} · {host.host}:{host.port}</p>
                          <p className="truncate">{host.databases != null ? `${host.databases} dbs` : "—"} · {host.nodeName ?? "—"}</p>
                        </div>
                        <div className="mt-3 flex items-center justify-end gap-1 border-t border-white/[0.06] pt-3">
                          <Btn size="sm" tone="ghost" onClick={() => testMut.mutate(host.id)} disabled={testMut.isPending}>{testMut.isPending && testMut.variables === host.id ? "Testing..." : "Test"}</Btn>
                          <Btn size="sm" tone="ghost" onClick={() => openEdit(host)}>Edit</Btn>
                          <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete database host "${host.name}"?`, description: `Databases provisioned through ${host.host}:${host.port} may be left in place; the panel host entry will be removed. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(host.id); })(); }} disabled={deleteMut.isPending}><Trash2 size={12} /></Btn>
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-slate-400">
                    <span>Showing {hostsRangeFrom}–{hostsRangeTo} of {sortedHosts.length} hosts</span>
                  </div>
                  {hostPageCount > 1 ? (
                    <Pagination page={safeHostsPage} pageCount={hostPageCount} onPageChange={setHostsPage} label="Database hosts pagination" />
                  ) : null}
                </>
              ) : (
                <>
                  <div className="overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] shadow-sm">
                  <div className="overflow-x-auto">
                    <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                    <th className="px-4 py-3 font-medium">Name</th>
                    <th className="px-2 py-3 font-medium">Engine</th>
                    <th className="px-2 py-3 font-medium">Status</th>
                    <th className="px-2 py-3 font-medium">Host : Port</th>
                    <th className="px-2 py-3 font-medium">Resources</th>
                    <th className="px-2 py-3 font-medium">Node</th>
                    <th className="px-2 py-3 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {visibleHosts.map((host) => (
                    <tr key={host.id} className="transition hover:bg-white/[0.02]">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2.5">
                          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-400">
                            <Database size={16} className="h-4 w-4" />
                          </span>
                          <span className="min-w-0">
                            <span className="block max-w-44 truncate text-xs font-bold text-slate-100" title={host.name}>{host.name}</span>
                            <span className="block font-mono text-[10px] text-slate-500">{host.id.slice(0, 8)}</span>
                          </span>
                        </div>
                      </td>
                      <td className="px-2 py-3">
                        <span className="block text-xs text-slate-200">{hostEngineLabel(host.engine)}</span>
                        <span className="block font-mono text-[10px] text-slate-500">{host.username}</span>
                      </td>
                      <td className="px-2 py-3"><span className="text-slate-500">—</span></td>
                      <td className="px-2 py-3 font-mono text-[11px] text-slate-300">{host.host}:{host.port}</td>
                      <td className="px-2 py-3 text-[11px] text-slate-400">{host.databases != null ? `${host.databases} dbs` : "—"}</td>
                      <td className="px-2 py-3 text-[11px] text-slate-400">{host.nodeName ?? "—"}</td>
                      <td className="px-2 py-3">
                        <div className="flex items-center justify-end gap-1">
                          <Btn size="sm" tone="ghost" onClick={() => testMut.mutate(host.id)} disabled={testMut.isPending}>{testMut.isPending && testMut.variables === host.id ? "Testing..." : "Test"}</Btn>
                          <Btn size="sm" tone="ghost" onClick={() => openEdit(host)}>Edit</Btn>
                          <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete database host "${host.name}"?`, description: `Databases provisioned through ${host.host}:${host.port} may be left in place; the panel host entry will be removed. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(host.id); })(); }} disabled={deleteMut.isPending}><Trash2 size={12} /></Btn>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
                    </table>
                  </div>
                  <div className="flex flex-wrap items-center justify-between gap-3 border-t border-white/[0.06] px-4 py-3 text-xs text-slate-400">
                    <span>Showing {hostsRangeFrom}–{hostsRangeTo} of {sortedHosts.length} hosts</span>
                  </div>
                </div>
                  {hostPageCount > 1 ? (
                    <Pagination page={safeHostsPage} pageCount={hostPageCount} onPageChange={setHostsPage} label="Database hosts pagination" />
                  ) : null}
                </>
              )}
            </div>
          </>
        )}
      </Card>

      <Card className="overflow-hidden">
        <CardHeader title="Orphan remediation" icon={AlertCircle} />
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] bg-[var(--surface)] px-5 py-4 text-sm text-slate-300">
          <p>Force-deleted server and database resources that could not be removed remotely are tracked here for administrator follow-up.</p>
          <div className="flex items-center gap-2">
            <AdminSelect label="" value={remediationStatus} onChange={(v) => setRemediationStatus(v as "pending" | "resolved")} options={[{ value: "pending", label: "Pending" }, { value: "resolved", label: "Resolved" }]} />
            <Btn size="sm" tone="ghost" onClick={() => void remediationsQuery.refetch()} disabled={remediationsQuery.isFetching}>
              <RefreshCw size={13} /> {remediationsQuery.isFetching ? "Refreshing..." : "Refresh"}
            </Btn>
          </div>
        </div>

        {remediationsQuery.isLoading ? (
          <div className="px-5 pb-5 pt-4"><AdminLoadingState label="Loading remediation tasks…" /></div>
        ) : remediationsQuery.isError ? (
          <div className="px-5 pb-5 pt-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load orphan remediation tasks: {remediationsQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void remediationsQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : (
          <div>
            <div className="flex items-center gap-2 border-b border-[var(--line)] bg-[color-mix(in_srgb,var(--surface-raised)_50%,transparent)] px-5 py-3 text-xs font-semibold uppercase tracking-widest text-slate-400">
              <Server size={14} /> Server resources <Pill>{serverRemediations.length}</Pill>
            </div>
            {serverRemediations.length === 0 ? (
              <div className="px-5 py-4 text-sm text-slate-300">No {remediationStatus} server orphan remediation tasks.</div>
            ) : (
              <div className="divide-y divide-[var(--line)]">
                {serverRemediations.map((remediation) => {
                  const isResolving = resolveServerRemediationMut.isPending && resolveServerRemediationMut.variables === remediation.id;
                  return (
                    <div className="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center lg:justify-between" key={remediation.id}>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-slate-200">Server {remediation.serverId}</span>
                          <Pill tone={remediation.status === "pending" ? "yellow" : "green"}>{remediation.status}</Pill>
                        </div>
                        <p className="mt-1 break-all font-mono text-xs text-slate-400">Node: {remediation.nodeUrl}</p>
                        <p className="mt-2 break-words text-xs text-red-200">{remediation.daemonError}</p>
                        <p className="mt-2 text-xs text-slate-400">Reported {new Date(remediation.createdAt).toLocaleString()}</p>
                      </div>
                      {remediation.status === "pending" ? (
                        <Btn size="sm" tone="ghost" disabled={resolveServerRemediationMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Mark server ${remediation.serverId} as resolved?`, description: "Only do this after confirming its remote resource has been cleaned up.", confirmLabel: "Mark resolved" })) resolveServerRemediationMut.mutate(remediation.id); })(); }}>{isResolving ? "Resolving..." : "Mark resolved"}</Btn>
                      ) : <span className="text-xs text-slate-400">Resolved {remediation.resolvedAt ? new Date(remediation.resolvedAt).toLocaleString() : ""}</span>}
                    </div>
                  );
                })}
              </div>
            )}

            <div className="flex items-center gap-2 border-y border-[var(--line)] bg-[color-mix(in_srgb,var(--surface-raised)_50%,transparent)] px-5 py-3 text-xs font-semibold uppercase tracking-widest text-slate-400">
              <Database size={14} /> Database resources <Pill>{databaseRemediations.length}</Pill>
            </div>
            {databaseRemediations.length === 0 ? (
              <div className="px-5 py-4 text-sm text-slate-300">No {remediationStatus} database orphan remediation tasks.</div>
            ) : (
              <div className="divide-y divide-[var(--line)]">
                {databaseRemediations.map((remediation) => {
                  const isResolving = resolveDatabaseRemediationMut.isPending && resolveDatabaseRemediationMut.variables === remediation.id;
                  return (
                    <div className="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center lg:justify-between" key={remediation.id}>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-slate-200">{remediation.database}</span>
                          <Pill tone={remediation.status === "pending" ? "yellow" : "green"}>{remediation.status}</Pill>
                        </div>
                        <p className="mt-1 break-all font-mono text-xs text-slate-400">{remediation.engine} · {remediation.host}:{remediation.port} · {remediation.username}@{remediation.remote}</p>
                        <p className="mt-2 break-words text-xs text-red-200">{remediation.reason}</p>
                        <p className="mt-2 text-xs text-slate-400">Reported {new Date(remediation.createdAt).toLocaleString()}</p>
                      </div>
                      {remediation.status === "pending" ? (
                        <Btn size="sm" tone="ghost" disabled={resolveDatabaseRemediationMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Mark ${remediation.database} as resolved?`, description: "Only do this after confirming its remote resource has been cleaned up.", confirmLabel: "Mark resolved" })) resolveDatabaseRemediationMut.mutate(remediation.id); })(); }}>{isResolving ? "Resolving..." : "Mark resolved"}</Btn>
                      ) : <span className="text-xs text-slate-400">Resolved {remediation.resolvedAt ? new Date(remediation.resolvedAt).toLocaleString() : ""}</span>}
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
          <div className="space-y-4">
          <AdminFormSection title="Connection">
            <div className="grid gap-5 sm:grid-cols-2">
              <div>
                <Input label="Display name" value={hName} onChange={setHName} placeholder="Local PostgreSQL" />
                {fieldErrors.name ? <p className="mt-1 text-sm text-red-300">{fieldErrors.name}</p> : null}
              </div>
              <AdminSelect label="Engine" value={hEngine} onChange={setHEngine} options={[{ value: "postgresql", label: "PostgreSQL" }, { value: "mysql", label: "MySQL" }]} />
              <div>
                <Input label="Host" value={hHost} onChange={setHHost} placeholder="db.internal.example" mono />
                {fieldErrors.host ? <p className="mt-1 text-sm text-red-300">{fieldErrors.host}</p> : <p className="mt-1 text-xs text-text-subtle">Use a hostname reachable from the Forge control plane. Inside a container, 127.0.0.1 refers to that container.</p>}
              </div>
              <div>
                <Input label="Port" value={hPort} onChange={setHPort} type="number" placeholder="5432" />
                {fieldErrors.port ? <p className="mt-1 text-sm text-red-300">{fieldErrors.port}</p> : null}
              </div>
              <div>
                <Input label="Username" value={hUser} onChange={setHUser} placeholder="gamepanel" mono />
                {fieldErrors.username ? <p className="mt-1 text-sm text-red-300">{fieldErrors.username}</p> : null}
              </div>
              <div>
                <Input label={modal === "create" ? "Password" : "Password (blank keeps current)"} value={hPass} onChange={setHPass} type="password" placeholder="" autoComplete="new-password" />
                {fieldErrors.password ? <p className="mt-1 text-sm text-red-300">{fieldErrors.password}</p> : null}
              </div>
              <AdminSelect label="Linked node (optional)" value={hNode} onChange={setHNode} placeholder="None" options={Array.isArray(nodes) ? nodes.map((n) => ({ value: n.id, label: n.name })) : []} />
              <div>
                <Input label="Max databases (blank = unlimited)" value={hMax} onChange={setHMax} type="number" placeholder="unlimited" />
                {fieldErrors.maxDatabases ? <p className="mt-1 text-sm text-red-300">{fieldErrors.maxDatabases}</p> : null}
              </div>
              <AdminSelect label="TLS Mode" value={hTLSMode} onChange={setHTLSMode} options={[{ value: "disable", label: "Disable" }, { value: "required", label: "Require" }, { value: "verify-ca", label: "Verify CA" }, { value: "verify-full", label: "Verify Full" }]} />
                {fieldErrors.tlsMode ? <p className="mt-1 text-sm text-red-300">{fieldErrors.tlsMode}</p> : <p className="mt-1 text-xs text-slate-400">Verify Full validates the server certificate and name. A custom CA is optional.</p>}
              <Input label="TLS Server Name (SNI, optional)" value={hTLSServerName} onChange={setHTLSServerName} mono />
            </div>
          </AdminFormSection>
          <AdminFormSection title="TLS">
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-300">TLS CA certificate (write-only)</label>
              <textarea className="h-28 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3.5 py-2 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-400 hover:border-white/20 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)] font-mono text-xs" value={hTLSCA} onChange={(e) => setHTLSCA(e.target.value)} placeholder={modal === "create" ? "Optional PEM certificate" : "Leave blank to keep current certificate"}/>
              <p className="mt-1 text-xs text-text-subtle">Certificates and passwords are stored securely and are not displayed after submission.</p>
            </div>
          </AdminFormSection>
          {createMut.isError ? (
            <div className="flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
              <AlertCircle size={14} className="mt-0.5 shrink-0" />
              <span>{createMut.error?.message || "An unexpected error occurred."}</span>
            </div>
          ) : null}
          {updateMut.isError ? (
            <div className="flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
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
          <div className="flex items-center justify-between gap-3 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-4">
            <p className="text-xs text-slate-400">Test the current settings successfully before saving.</p>
            <Btn tone="success" type="button" onClick={handleTest} disabled={testMut.isPending}>
              {testMut.isPending ? "Testing..." : "Test Connection"}
            </Btn>
          </div>
          </div>
          <ModalFooter
            onCancel={() => setModal(null)}
            onConfirm={handleConfirm}
            disabled={!hName.trim() || !hHost.trim() || !hPort.trim() || !hUser.trim() || (modal === "create" && !hPass) || !hasSuccessfulTest || isPending}
            confirmLabel={isPending ? "Saving..." : (modal === "create" ? "Create" : "Save")}
          />
        </Modal>
      ) : null}
      {renderConfirm()}
    </div>
  );
}
