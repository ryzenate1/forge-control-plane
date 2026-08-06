"use client";
import { isAvailable, isPartial, reportedTotal, useNodesQuery } from "@/lib/admin/telemetry";

import { useEffect, useState, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle2, Cpu, Database, LayoutGrid, List, Network, Plus, RefreshCw, Search, Server, Trash2 } from "lucide-react";
import { type ApiDatabaseHost, type CreateDatabaseHostInput, createDatabaseHost, deleteDatabaseHost, fetchDatabaseHosts, fetchOrphanRemediations, resolveDatabaseOrphanRemediation, resolveServerOrphanRemediation, testDatabaseHostConnection, updateDatabaseHost } from "@/lib/api";
import { toast } from "@/components/ui/sonner";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, AdminSelect, AdminFormSection, AdminErrorState, AdminLoadingState, cn } from "./admin-ui";
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

export function AdminDatabases() {
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

  const hostSelectCls = "ui-input w-full cursor-pointer sm:w-44";

  const hostEngines = useMemo(() => [...new Set(hosts.map((h) => h.engine))].sort(), [hosts]);
  /**
   * Host tiles. `tile` class strings are gone with `DbStatCards`' per-tile palette
   * (the tiles are one neutral treatment now), and the database total no longer
   * folds an unreported host into `0`: `reportedTotal` sums only the hosts that
   * actually gave a number and says how many did, so a partial read is legible as
   * partial rather than as a smaller measured total.
   */
  const hostStats: DbStat[] = useMemo(() => {
    const ready = isAvailable(hostsQuery);
    const engines = new Set(hosts.map((h) => h.engine));
    const databaseTotal = reportedTotal(hosts, (h) => h.databases ?? h.maxDatabases);
    const linkedNodes = new Set(hosts.map((h) => h.nodeId ?? h.nodeName).filter((v): v is string => Boolean(v)));
    const unknown = <span className="text-text-muted" title="The hosts query has not reported">—</span>;
    return [
      { key: "total", label: "Total Hosts", icon: Server, hint: "Database hosts registered in this panel", value: ready ? hosts.length : unknown },
      { key: "engines", label: "Engines", icon: Cpu, hint: "Distinct database engines across those hosts", value: ready ? engines.size : unknown },
      {
        key: "databases",
        label: "Databases",
        icon: Database,
        hint: "Sum of what the hosts reported",
        value: ready
          ? databaseTotal.total === 0
            ? "—"
            : isPartial(databaseTotal)
              ? (
                <span title={`Only ${databaseTotal.reported} of ${databaseTotal.total} hosts reported a database count`}>
                  {databaseTotal.value?.toLocaleString()}
                  <span className="ml-1 text-xs font-normal text-warn">partial</span>
                </span>
              )
              : databaseTotal.value?.toLocaleString()
          : unknown,
      },
      { key: "nodes", label: "Nodes", icon: Network, hint: "Beacons linked to at least one host", value: ready ? linkedNodes.size : unknown },
    ];
  }, [hosts, hostsQuery]);
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
  const hostHasFilters = Boolean(hostSearch.trim()) || hostEngineFilter !== "all";
  /** Range line that names the filtered denominator instead of folding it into the total. */
  const hostsRangeLabel = sortedHosts.length === 0
    ? "Nothing to show"
    : `Showing ${hostsRangeFrom}–${hostsRangeTo} of ${hostHasFilters ? `${sortedHosts.length} matched of ${hosts.length} hosts (filtered)` : `${sortedHosts.length} hosts`}`;

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
    <div className="space-y-5">
      {/* Sub-heading, not a second page title. This component renders inside the
          Databases route, whose `<h1>` comes from `SectionHeader` there; a tab that
          headed itself with `SectionHeader` put a second `<h1>` on the route and made
          the visible page title change with every tab. Heading order is now
          h1 (Databases) → h2 (Database Hosts). */}
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line pb-3">
        <div className="min-w-0">
          <h2 className="t-title">Database Hosts</h2>
          <p className="mt-0.5 max-w-prose text-meta text-text-subtle">
            External MySQL and PostgreSQL hosts that provision per-workload databases. A host can be
            linked to a Beacon so provisioning happens on that machine.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Btn ariaLabel="Refresh database hosts" disabled={hostsQuery.isFetching} onClick={() => void hostsQuery.refetch()} tone="ghost">
            <RefreshCw size={14} /> Refresh
          </Btn>
          <Btn onClick={openCreate} tone="primary"><Plus size={14} /> New Host</Btn>
        </div>
      </div>
      <p className="max-w-prose rounded-lg border border-line bg-overlay-subtle px-4 py-2 text-xs leading-5 text-text-subtle">
        A host here is a connection this panel stores and hands to workloads — it is not a machine
        Beacon runs on. Credentials are stored encrypted and TLS verification defaults to
        <code className="ui-code-inline">verify-full</code>. Test a configuration before saving it: a host
        that has never been tested successfully can still be created, but provisioning against it
        will fail.
      </p>

      <DbStatCards stats={hostStats} />

      <Card className="overflow-hidden">
        <CardHeader title="Configured hosts" icon={Database} />
        {hostsQuery.isPending ? (
          <AdminLoadingState label="Loading database hosts…" />
        ) : hostsQuery.isError ? (
          <div className="p-5">
            <AdminErrorState
              message={`Could not load database hosts: ${hostsQuery.error.message}`}
              retry={() => void hostsQuery.refetch()}
            />
          </div>
        ) : hosts.length === 0 ? (
          <EmptyState icon={Database} message="No database hosts registered. Add one so servers can provision databases on it." title="No database hosts" />
        ) : (
          <>
            <div className="space-y-4 px-5 pb-5 pt-4">
              <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
                <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-2 focus-within:border-line-strong">
                  <Search aria-hidden="true" size={13} className="shrink-0 text-text-muted" />
                  <input
                    aria-label="Search database hosts"
                    className="w-full bg-transparent text-xs text-text outline-none placeholder:text-text-muted"
                    onChange={(e) => { setHostSearch(e.target.value); setHostsPage(1); }}
                    placeholder="Search hosts by name, host, engine…"
                    type="search"
                    value={hostSearch}
                  />
                </label>
                <div className="flex flex-wrap items-center gap-2">
                  <select aria-label="Filter hosts by engine" className={hostSelectCls} onChange={(e) => { setHostEngineFilter(e.target.value); setHostsPage(1); }} value={hostEngineFilter}>
                    <option value="all">Engine: all</option>
                    {hostEngines.map((e) => <option key={e} value={e}>{e}</option>)}
                  </select>
                  <div aria-label="View mode" className="flex gap-1 rounded-lg border border-line bg-overlay-subtle p-1" role="group">
                    <button aria-pressed={hostView === "table"} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", hostView === "table" ? "border border-brand-line bg-brand-subtle text-text" : "border border-transparent text-text-muted hover:text-text")} onClick={() => setHostView("table")} type="button">
                      <List aria-hidden="true" size={14} /> Table
                    </button>
                    <button aria-pressed={hostView === "cards"} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", hostView === "cards" ? "border border-brand-line bg-brand-subtle text-text" : "border border-transparent text-text-muted hover:text-text")} onClick={() => setHostView("cards")} type="button">
                      <LayoutGrid aria-hidden="true" size={14} /> Cards
                    </button>
                  </div>
                </div>
              </div>

              <div className="flex flex-wrap items-center justify-between gap-3">
                {/* The filtered count is not the total: say which is which, the way
                    the Servers list does, rather than labelling a subset "Hosts (4)". */}
                <h3 className="t-title">
                  Hosts <span className="font-normal text-text-subtle">({hostHasFilters ? `${sortedHosts.length} of ${hosts.length} (filtered)` : `${hosts.length} host${hosts.length === 1 ? "" : "s"}`})</span>
                </h3>
                <label className="flex items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-1.5 text-xs text-text">
                  <span className="text-text-subtle">Sort by</span>
                  <select aria-label="Sort database hosts" className="cursor-pointer appearance-none bg-transparent pr-1 outline-none" onChange={(e) => setHostSort(e.target.value)} value={hostSort}>
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
                      <div key={host.id} className="rounded-xl border border-line bg-overlay-subtle p-4 shadow-sm transition hover:border-line-strong">
                        <div className="flex items-start gap-3">
                          <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-line bg-overlay-subtle text-text-subtle">
                            <Database size={18} />
                          </span>
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-sm font-bold text-text" title={host.name}>{host.name}</p>
                            <p className="font-mono text-meta text-text-muted">{host.id.slice(0, 8)}</p>
                          </div>
                        </div>
                        <div className="mt-3 space-y-1 border-t border-line pt-3 font-mono text-meta text-text-subtle">
                          <p className="truncate">{hostEngineLabel(host.engine)} · {host.host}:{host.port}</p>
                          <p className="truncate">{host.databases != null ? `${host.databases} dbs` : "—"} · {host.nodeName ?? "—"}</p>
                        </div>
                        <div className="mt-3 flex items-center justify-end gap-1 border-t border-line pt-3">
                          <Btn size="sm" tone="ghost" onClick={() => testMut.mutate(host.id)} disabled={testMut.isPending}>{testMut.isPending && testMut.variables === host.id ? "Testing..." : "Test"}</Btn>
                          <Btn size="sm" tone="ghost" onClick={() => openEdit(host)}>Edit</Btn>
                          <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete database host "${host.name}"?`, description: `Databases provisioned through ${host.host}:${host.port} may be left in place; the panel host entry will be removed. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(host.id); })(); }} disabled={deleteMut.isPending}><Trash2 size={12} /></Btn>
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-text-subtle">
                    <span>{hostsRangeLabel}</span>
                  </div>
                  {hostPageCount > 1 ? (
                    <Pagination page={safeHostsPage} pageCount={hostPageCount} onPageChange={setHostsPage} label="Database hosts pagination" />
                  ) : null}
                </>
              ) : (
                <>
                  <div className="overflow-hidden rounded-xl border border-line bg-overlay-subtle shadow-sm">
                  <div className="overflow-x-auto">
                    <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-line text-left text-eyebrow uppercase tracking-[0.12em] text-text-muted">
                    <th className="px-4 py-3 font-medium">Name</th>
                    <th className="px-2 py-3 font-medium">Engine</th>
                    <th className="px-2 py-3 font-medium">Status</th>
                    <th className="px-2 py-3 font-medium">Host : Port</th>
                    <th className="px-2 py-3 font-medium">Resources</th>
                    <th className="px-2 py-3 font-medium">Node</th>
                    <th className="px-2 py-3 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {visibleHosts.map((host) => (
                    <tr key={host.id} className="transition hover:bg-overlay-subtle">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2.5">
                          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-line bg-overlay-subtle text-text-subtle">
                            <Database size={16} className="h-4 w-4" />
                          </span>
                          <span className="min-w-0">
                            <span className="block max-w-44 truncate text-xs font-bold text-text" title={host.name}>{host.name}</span>
                            <span className="block font-mono text-meta text-text-muted">{host.id.slice(0, 8)}</span>
                          </span>
                        </div>
                      </td>
                      <td className="px-2 py-3">
                        <span className="block text-xs text-text">{hostEngineLabel(host.engine)}</span>
                        <span className="block font-mono text-meta text-text-muted">{host.username}</span>
                      </td>
                      <td className="px-2 py-3"><span className="text-text-muted">—</span></td>
                      <td className="px-2 py-3 font-mono text-meta text-text-subtle">{host.host}:{host.port}</td>
                      <td className="px-2 py-3 text-meta text-text-subtle">{host.databases != null ? `${host.databases} dbs` : "—"}</td>
                      <td className="px-2 py-3 text-meta text-text-subtle">{host.nodeName ?? "—"}</td>
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
                  <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-4 py-3 text-xs text-text-subtle">
                    <span>{hostsRangeLabel}</span>
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
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] bg-overlay-subtle px-5 py-4 text-sm text-text-subtle">
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
            <div className="flex items-start justify-between gap-4 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
              <span>Could not load orphan remediation tasks: {remediationsQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void remediationsQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : (
          <div>
            <div className="flex items-center gap-2 border-b border-[var(--line)] bg-[color-mix(in_srgb,var(--surface-raised)_50%,transparent)] px-5 py-3 text-xs font-semibold uppercase tracking-widest text-text-subtle">
              <Server size={14} /> Server resources <Pill>{serverRemediations.length}</Pill>
            </div>
            {serverRemediations.length === 0 ? (
              <div className="px-5 py-4 text-sm text-text-subtle">No {remediationStatus} server orphan remediation tasks.</div>
            ) : (
              <div className="divide-y divide-[var(--line)]">
                {serverRemediations.map((remediation) => {
                  const isResolving = resolveServerRemediationMut.isPending && resolveServerRemediationMut.variables === remediation.id;
                  return (
                    <div className="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center lg:justify-between" key={remediation.id}>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-text">Server {remediation.serverId}</span>
                          <Pill tone={remediation.status === "pending" ? "yellow" : "green"}>{remediation.status}</Pill>
                        </div>
                        <p className="mt-1 break-all font-mono text-xs text-text-subtle">Node: {remediation.nodeUrl}</p>
                        <p className="mt-2 break-words text-xs text-danger">{remediation.daemonError}</p>
                        <p className="mt-2 text-xs text-text-subtle">Reported {new Date(remediation.createdAt).toLocaleString()}</p>
                      </div>
                      {remediation.status === "pending" ? (
                        <Btn size="sm" tone="ghost" disabled={resolveServerRemediationMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Mark server ${remediation.serverId} as resolved?`, description: "Only do this after confirming its remote resource has been cleaned up.", confirmLabel: "Mark resolved" })) resolveServerRemediationMut.mutate(remediation.id); })(); }}>{isResolving ? "Resolving..." : "Mark resolved"}</Btn>
                      ) : <span className="text-xs text-text-subtle">Resolved {remediation.resolvedAt ? new Date(remediation.resolvedAt).toLocaleString() : ""}</span>}
                    </div>
                  );
                })}
              </div>
            )}

            <div className="flex items-center gap-2 border-y border-[var(--line)] bg-[color-mix(in_srgb,var(--surface-raised)_50%,transparent)] px-5 py-3 text-xs font-semibold uppercase tracking-widest text-text-subtle">
              <Database size={14} /> Database resources <Pill>{databaseRemediations.length}</Pill>
            </div>
            {databaseRemediations.length === 0 ? (
              <div className="px-5 py-4 text-sm text-text-subtle">No {remediationStatus} database orphan remediation tasks.</div>
            ) : (
              <div className="divide-y divide-[var(--line)]">
                {databaseRemediations.map((remediation) => {
                  const isResolving = resolveDatabaseRemediationMut.isPending && resolveDatabaseRemediationMut.variables === remediation.id;
                  return (
                    <div className="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center lg:justify-between" key={remediation.id}>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-text">{remediation.database}</span>
                          <Pill tone={remediation.status === "pending" ? "yellow" : "green"}>{remediation.status}</Pill>
                        </div>
                        <p className="mt-1 break-all font-mono text-xs text-text-subtle">{remediation.engine} · {remediation.host}:{remediation.port} · {remediation.username}@{remediation.remote}</p>
                        <p className="mt-2 break-words text-xs text-danger">{remediation.reason}</p>
                        <p className="mt-2 text-xs text-text-subtle">Reported {new Date(remediation.createdAt).toLocaleString()}</p>
                      </div>
                      {remediation.status === "pending" ? (
                        <Btn size="sm" tone="ghost" disabled={resolveDatabaseRemediationMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Mark ${remediation.database} as resolved?`, description: "Only do this after confirming its remote resource has been cleaned up.", confirmLabel: "Mark resolved" })) resolveDatabaseRemediationMut.mutate(remediation.id); })(); }}>{isResolving ? "Resolving..." : "Mark resolved"}</Btn>
                      ) : <span className="text-xs text-text-subtle">Resolved {remediation.resolvedAt ? new Date(remediation.resolvedAt).toLocaleString() : ""}</span>}
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
                {fieldErrors.name ? <p className="mt-1 text-sm text-danger">{fieldErrors.name}</p> : null}
              </div>
              <AdminSelect label="Engine" value={hEngine} onChange={setHEngine} options={[{ value: "postgresql", label: "PostgreSQL" }, { value: "mysql", label: "MySQL" }]} />
              <div>
                <Input label="Host" value={hHost} onChange={setHHost} placeholder="db.internal.example" mono />
                {fieldErrors.host ? <p className="mt-1 text-sm text-danger">{fieldErrors.host}</p> : <p className="mt-1 text-xs text-text-subtle">Resolved by the panel API. In a container, 127.0.0.1 is the API container, not automatically the panel database.</p>}
              </div>
              <div>
                <Input label="Port" value={hPort} onChange={setHPort} type="number" placeholder="5432" />
                {fieldErrors.port ? <p className="mt-1 text-sm text-danger">{fieldErrors.port}</p> : null}
              </div>
              <div>
                <Input label="Username" value={hUser} onChange={setHUser} placeholder="gamepanel" mono />
                {fieldErrors.username ? <p className="mt-1 text-sm text-danger">{fieldErrors.username}</p> : null}
              </div>
              <div>
                <Input label={modal === "create" ? "Password" : "Password (blank keeps current)"} value={hPass} onChange={setHPass} type="password" placeholder="" autoComplete="new-password" />
                {fieldErrors.password ? <p className="mt-1 text-sm text-danger">{fieldErrors.password}</p> : null}
              </div>
              <AdminSelect label="Linked node (optional)" value={hNode} onChange={setHNode} placeholder="None" options={Array.isArray(nodes) ? nodes.map((n) => ({ value: n.id, label: n.name })) : []} />
              <div>
                <Input label="Max databases (blank = unlimited)" value={hMax} onChange={setHMax} type="number" placeholder="unlimited" />
                {fieldErrors.maxDatabases ? <p className="mt-1 text-sm text-danger">{fieldErrors.maxDatabases}</p> : null}
              </div>
              <AdminSelect label="TLS Mode" value={hTLSMode} onChange={setHTLSMode} options={[{ value: "disable", label: "Disable" }, { value: "required", label: "Require" }, { value: "verify-ca", label: "Verify CA" }, { value: "verify-full", label: "Verify Full" }]} />
                {fieldErrors.tlsMode ? <p className="mt-1 text-sm text-danger">{fieldErrors.tlsMode}</p> : <p className="mt-1 text-xs text-text-subtle">Verify Full validates the server certificate and name. A custom CA is optional.</p>}
              <Input label="TLS Server Name (SNI, optional)" value={hTLSServerName} onChange={setHTLSServerName} mono />
            </div>
          </AdminFormSection>
          <AdminFormSection title="TLS">
            <div>
              <label className="mb-1.5 block text-sm font-medium text-text-subtle">TLS CA certificate (write-only)</label>
              <textarea className="h-28 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3.5 py-2 text-sm text-text shadow-inner shadow-black/10 outline-none transition placeholder:text-text-subtle hover:border-line-strong focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)] font-mono text-xs" value={hTLSCA} onChange={(e) => setHTLSCA(e.target.value)} placeholder={modal === "create" ? "Optional PEM certificate" : "Leave blank to keep current certificate"}/>
              <p className="mt-1 text-xs text-text-subtle">Certificates and passwords are redacted by the API and never displayed after submission.</p>
            </div>
          </AdminFormSection>
          {createMut.isError ? (
            <div className="flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
              <AlertCircle size={14} className="mt-0.5 shrink-0" />
              <span>{createMut.error?.message || "An unexpected error occurred."}</span>
            </div>
          ) : null}
          {updateMut.isError ? (
            <div className="flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
              <AlertCircle size={14} className="mt-0.5 shrink-0" />
              <span>{updateMut.error?.message || "An unexpected error occurred."}</span>
            </div>
          ) : null}
          {createMut.isSuccess || updateMut.isSuccess ? (
            <div className="mt-5 flex items-start gap-2 rounded-lg border border-ok-line bg-ok-subtle p-3 text-xs text-ok">
              <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
              <span>Database host {modal === "create" ? "created" : "updated"} successfully.</span>
            </div>
          ) : null}
          <div className="flex items-center justify-between gap-3 rounded-lg border border-[var(--line)] bg-overlay-subtle p-4">
            <p className="text-xs text-text-subtle">Test the current settings successfully before saving.</p>
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
