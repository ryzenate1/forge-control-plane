"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { type ApiServer, fetchCurrentUser, fetchServers, logout } from "@/lib/api";
import { useServerStore } from "@/stores/use-server-store";
import { Cpu, HardDrive, LogOut, MemoryStick, Server, User } from "lucide-react";
import { useBranding } from "@/components/branding";
import { Pagination, ResourceBar, SearchInput, StatusPill, Switch } from "@/components/ui/primitives";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";
import { ThemeToggle } from "@/components/ui/theme-toggle";

/* -------------------------------------------------------------------------- */
/*  Status pill with suspended/transferring/installing                        */
/* -------------------------------------------------------------------------- */

function ServerStatus({ server }: { server: ApiServer }) {
  if (server.suspended) {
    return (
      <StatusPill tone="danger">Suspended</StatusPill>
    );
  }
  if (server.transferring) {
    return (
      <StatusPill pulse tone="info">Transferring</StatusPill>
    );
  }
  if (server.status === "running") {
    return (
      <StatusPill tone="success">Running</StatusPill>
    );
  }
  if (server.status === "installing") {
    return (
      <StatusPill pulse tone="warning">Installing</StatusPill>
    );
  }
  return (
    <StatusPill>{server.status || "Offline"}</StatusPill>
  );
}

/* -------------------------------------------------------------------------- */
/*  Admin "show all servers" toggle                                           */
/* -------------------------------------------------------------------------- */

/* -------------------------------------------------------------------------- */
/*  Main page                                                                  */
/* -------------------------------------------------------------------------- */

export default function ServersPage() {
  const router = useRouter();
  const { currentUser } = useServerStore();
  const { companyName } = useBranding();

  const userQuery = useQuery({
    queryKey: ["current-user"],
    queryFn: fetchCurrentUser,
    retry: 1,
    staleTime: 30_000,
  });

  useEffect(() => {
    if (userQuery.data === null) router.replace("/?reason=session-expired&next=%2Fservers");
  }, [router, userQuery.data]);

  const isAdmin = currentUser?.role === "admin" || userQuery.data?.role === "admin";
  const [showAdmin, setShowAdmin] = useState(false);

  const serversQuery = useQuery({
    queryKey: ["servers", showAdmin && isAdmin ? "admin" : "user"],
    queryFn: fetchServers,
    enabled: Boolean(userQuery.data),
    refetchInterval: (query) => query.state.error ? false : 15_000,
    retry: 1,
  });
  const { data, isLoading, isError } = serversQuery;

  const handleLogout = async () => {
    try {
      await logout();
    } finally {
      router.push("/");
    }
  };

  const sessionPending = userQuery.isPending || userQuery.data === undefined;
  const servers = useMemo(() => (Array.isArray(data) ? data : []), [data]);
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const pageSize = 12;
  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return needle
      ? servers.filter((server) =>
          [server.name, server.description, server.node, server.allocation].some(
            (value) => value?.toLowerCase().includes(needle),
          ),
        )
      : servers;
  }, [search, servers]);
  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize));
  const visibleServers = filtered.slice(
    (Math.min(page, pageCount) - 1) * pageSize,
    Math.min(page, pageCount) * pageSize,
  );
  useEffect(() => {
    setPage(1);
  }, [search, showAdmin]);

  if (sessionPending || userQuery.data === null) {
    return <div className="grid min-h-screen place-items-center bg-surface-base text-slate-100"><LoadingSpinner label="Verifying session" /></div>;
  }

  if (userQuery.isError) {
    return <div className="grid min-h-screen place-items-center bg-surface-base px-4 text-center text-slate-100"><div><p className="text-lg font-semibold">Session verification is unavailable</p><p className="mt-2 text-sm text-neutral-secondary">Please retry once the API is reachable.</p></div></div>;
  }

  return (
    <div className="min-h-screen bg-surface-base text-slate-100">
      {/* Header */}
      <header className="flex h-14 items-center justify-between border-b border-white/[0.06] bg-surface-secondary px-4 sm:px-6">
        <div className="text-lg font-bold text-slate-100">{companyName}</div>
        <div className="flex items-center gap-1">
          <ThemeToggle />
          <Link
            className="flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-slate-400 transition-colors hover:bg-surface-elevated hover:text-white"
            href="/account"
          >
            <User className="h-4 w-4" />
            Account
          </Link>
          <button
            onClick={handleLogout}
            className="flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-slate-400 hover:text-white hover:bg-surface-elevated transition-colors"
            type="button"
          >
            <LogOut className="w-4 h-4" />
            Sign Out
          </button>
        </div>
      </header>

      {/* Content */}
      <main className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
        <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-2xl font-bold text-slate-100">
              {showAdmin && isAdmin ? "All Servers" : "My Servers"}
            </h1>
            <p className="mt-1 text-sm text-neutral-secondary">
              Manage and monitor your game servers. Status refreshes every 15 seconds.
            </p>
          </div>
          <div className="flex items-center gap-4">
            {isAdmin ? (
              <Switch checked={showAdmin} label={showAdmin ? "Showing others' servers" : "Showing your servers"} onCheckedChange={setShowAdmin} />
            ) : null}
            <SearchInput className="w-full sm:max-w-sm" label="Search servers" onChange={(event) => setSearch(event.target.value)} placeholder="Search name, node, or address" value={search} />
          </div>
        </div>

        {isLoading && (
          <div className="flex items-center justify-center py-20">
            <LoadingSpinner label="Loading servers" />
          </div>
        )}

        {isError && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-300">
            Failed to load servers. Please refresh or try again.
          </div>
        )}

        {!isLoading && !isError && filtered.length === 0 && (
          <div className="flex flex-col items-center justify-center rounded-xl border border-white/[0.06] bg-surface-card py-20 text-center">
            <Server className="mb-4 h-12 w-12 text-slate-600" />
            <p className="text-base font-semibold text-slate-300">
              {search ? "No matching servers" : "No servers found"}
            </p>
            <p className="mt-1 text-sm text-neutral-secondary">
              {search
                ? "Try a different name, node, or address."
                : "Contact your administrator to provision a server."}
            </p>
          </div>
        )}

        {!isLoading && filtered.length > 0 && (
          <>
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {visibleServers.map((server) => (
                <Link
                  key={server.id}
                  href={`/server/${server.id}`}
                  className="group flex flex-col rounded-xl border border-white/[0.06] bg-surface-card p-5 transition hover:border-white/[0.12] hover:bg-surface-elevated"
                >
                  {/* Status bar indicator */}
                  <div className="pointer-events-none absolute right-0 top-0 bottom-0 w-1 rounded-r-xl opacity-50 group-hover:opacity-75 transition-opacity" style={{
                    backgroundColor: server.suspended ? "#f43f5e" : server.status === "running" ? "#10b981" : server.status === "installing" ? "#f59e0b" : "#64748b"
                  }} />

                  <div className="flex items-start justify-between gap-3 mb-3">
                    <div className="min-w-0">
                      <h2 className="truncate text-base font-bold text-slate-100">
                        {server.name}
                      </h2>
                      {server.description && (
                        <p className="mt-0.5 line-clamp-1 text-xs text-neutral-secondary">
                          {server.description}
                        </p>
                      )}
                    </div>
                    <ServerStatus server={server} />
                  </div>

                  <div className="mt-auto space-y-1.5 text-xs text-neutral-secondary">
                    {server.node && (
                      <div className="flex items-center gap-1.5">
                        <span className="font-medium text-slate-400">Node:</span>
                        <span>{server.node}</span>
                      </div>
                    )}
                    {server.allocation && (
                      <div className="flex items-center gap-1.5">
                        <span className="font-medium text-slate-400">Address:</span>
                        <span className="font-mono">{server.allocation}</span>
                      </div>
                    )}
                  </div>

                  {/* Resource usage bars */}
                  {!server.suspended && server.status !== "installing" && !server.transferring && (
                    <div className="mt-3 space-y-1 border-t border-white/[0.06] pt-3">
                      <ResourceBar icon={Cpu} label="CPU" current={server.cpuShares ?? server.cpuLimit} limit={server.cpuLimit ?? server.cpuShares} unit="%" />
                      <ResourceBar icon={MemoryStick} label="Memory" current={server.memoryMb} limit={server.memoryMb} unit=" MB" />
                      <ResourceBar icon={HardDrive} label="Disk" current={server.diskMb} limit={server.diskMb} unit=" MB" />
                    </div>
                  )}
                </Link>
              ))}
            </div>
            <Pagination label="Server list pages" onPageChange={setPage} page={page} pageCount={pageCount} />
          </>
        )}
      </main>
    </div>
  );
}
